package dockerx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
	log  *slog.Logger
}

type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	ImageID string
	State   string
}

type listItem struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	State   string            `json:"State"`
	Labels  map[string]string `json:"Labels"`
}

type createResp struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

func New(log *slog.Logger) (*Client, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 0}, log: log}, nil
}

func (c *Client) Close() error { return nil }

func (c *Client) RunningContainers(ctx context.Context) ([]ContainerInfo, error) {
	var items []listItem
	if err := c.doJSON(ctx, http.MethodGet, "/containers/json?filters="+url.QueryEscape(`{"status":["running"]}`), nil, &items); err != nil {
		return nil, err
	}
	out := make([]ContainerInfo, 0, len(items))
	for _, item := range items {
		out = append(out, c.containerInfoFromListItem(ctx, item))
	}
	return out, nil
}

func (c *Client) PullImage(ctx context.Context, ref string) error {
	if !isPullable(ref) {
		return fmt.Errorf("image %q is not tag based, skip", ref)
	}
	path := "/images/create?fromImage=" + url.QueryEscape(ref)
	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func (c *Client) InspectImageID(ctx context.Context, ref string) (string, error) {
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil, &data); err != nil {
		return "", err
	}
	if id, _ := data["Id"].(string); id != "" {
		return id, nil
	}
	if id, _ := data["ID"].(string); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("image id not found for %s", ref)
}

func (c *Client) UpdateContainer(ctx context.Context, id string, imageRef string, cleanup bool) (oldID, newID string, err error) {
	var inspect map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &inspect); err != nil {
		return "", "", err
	}
	oldID = str(inspect["Id"])
	oldName := strings.TrimPrefix(str(inspect["Name"]), "/")
	oldImageID := str(inspect["Image"])

	cfg, _ := inspect["Config"].(map[string]any)
	body := map[string]any{}
	for k, v := range cfg {
		body[k] = v
	}
	if imageRef != "" {
		body["Image"] = imageRef
	}
	hostCfg, _ := inspect["HostConfig"].(map[string]any)
	networking := buildNetworkingConfig(inspect["NetworkSettings"])
	body["HostConfig"] = hostCfg
	body["NetworkingConfig"] = networking

	if err := c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/stop?t=30"); err != nil {
		return oldID, "", fmt.Errorf("stop container %s: %w", oldName, err)
	}

	backupName := fmt.Sprintf("%s-dockup-backup-%d", oldName, time.Now().Unix())
	if err := c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/rename?name="+url.QueryEscape(backupName)); err != nil {
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/start")
		return oldID, "", fmt.Errorf("rename old container: %w", err)
	}

	var created createResp
	if err := c.doJSON(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(oldName), body, &created); err != nil {
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/rename?name="+url.QueryEscape(oldName))
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/start")
		return oldID, "", fmt.Errorf("create new container: %w", err)
	}
	newID = created.ID

	if err := c.postEmpty(ctx, "/containers/"+url.PathEscape(newID)+"/start"); err != nil {
		_ = c.delete(ctx, "/containers/"+url.PathEscape(newID)+"?force=true")
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/rename?name="+url.QueryEscape(oldName))
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/start")
		return oldID, newID, fmt.Errorf("start new container failed, rolled back: %w", err)
	}

	if err := c.waitHealthy(ctx, newID, 5*time.Minute); err != nil {
		_ = c.delete(ctx, "/containers/"+url.PathEscape(newID)+"?force=true")
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/rename?name="+url.QueryEscape(oldName))
		_ = c.postEmpty(ctx, "/containers/"+url.PathEscape(oldID)+"/start")
		return oldID, newID, fmt.Errorf("healthcheck failed, rolled back: %w", err)
	}

	if err := c.delete(ctx, "/containers/"+url.PathEscape(oldID)+"?force=true"); err != nil {
		c.log.Warn("failed to remove backup container", "container", backupName, "error", err)
	}
	if cleanup && oldImageID != "" {
		if err := c.delete(ctx, "/images/"+url.PathEscape(oldImageID)+"?force=false&noprune=false"); err != nil {
			c.log.Warn("failed to remove old image", "image", oldImageID, "error", err)
		}
	}
	return oldID, newID, nil
}

func (c *Client) RunSelfUpdateHelper(ctx context.Context, helperImage, targetID, targetImage string, cleanup bool) (string, error) {
	if helperImage == "" || targetID == "" || targetImage == "" {
		return "", fmt.Errorf("helperImage, targetID and targetImage are required")
	}
	name := fmt.Sprintf("dockup-self-update-%d", time.Now().Unix())
	body := map[string]any{
		"Image": helperImage,
		"Env": []string{
			"DOCKUP_APPLY_CONTAINER_ID=" + targetID,
			"DOCKUP_APPLY_IMAGE_REF=" + targetImage,
			fmt.Sprintf("CLEANUP=%t", cleanup),
		},
		"HostConfig": map[string]any{
			"Binds":       []string{"/var/run/docker.sock:/var/run/docker.sock"},
			"AutoRemove":  true,
			"NetworkMode": "bridge",
		},
	}
	var created createResp
	if err := c.doJSON(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), body, &created); err != nil {
		return "", err
	}
	if err := c.postEmpty(ctx, "/containers/"+url.PathEscape(created.ID)+"/start"); err != nil {
		_ = c.delete(ctx, "/containers/"+url.PathEscape(created.ID)+"?force=true")
		return "", err
	}
	return created.ID, nil
}

func (c *Client) waitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var inspect map[string]any
		if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &inspect); err != nil {
			return err
		}
		state, _ := inspect["State"].(map[string]any)
		if running, _ := state["Running"].(bool); !running {
			return fmt.Errorf("container exited")
		}
		health, _ := state["Health"].(map[string]any)
		if health == nil {
			return nil
		}
		switch str(health["Status"]) {
		case "healthy":
			return nil
		case "unhealthy":
			return fmt.Errorf("container unhealthy")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("healthcheck timeout")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func buildNetworkingConfig(v any) map[string]any {
	settings, _ := v.(map[string]any)
	nets, _ := settings["Networks"].(map[string]any)
	endpoints := map[string]any{}
	for name, raw := range nets {
		ep, _ := raw.(map[string]any)
		copy := map[string]any{}
		for k, val := range ep {
			copy[k] = val
		}
		for _, k := range []string{"NetworkID", "EndpointID", "Gateway", "IPAddress", "IPPrefixLen", "IPv6Gateway", "GlobalIPv6Address", "GlobalIPv6PrefixLen", "MacAddress"} {
			delete(copy, k)
		}
		endpoints[name] = copy
	}
	return map[string]any{"EndpointsConfig": endpoints}
}

func (c *Client) postEmpty(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) delete(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return nil, fmt.Errorf("docker API %s %s failed: %s %s", method, path, resp.Status, strings.TrimSpace(string(b)))
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func isPullable(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "sha256:") || strings.Contains(ref, "@sha256:") {
		return false
	}
	return true
}
