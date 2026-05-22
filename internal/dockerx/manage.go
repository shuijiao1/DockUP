package dockerx

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ProjectInfo struct {
	Key        string
	Name       string
	Type       string
	WorkingDir string
	ConfigFile string
	Containers []ContainerInfo
}

type ContainerDetail struct {
	Info       ContainerInfo
	State      string
	Status     string
	Health     string
	Created    string
	Started    string
	Restarting bool
	Compose    bool
	Project    string
	Service    string
	WorkingDir string
	ConfigFile string
	Ports      string
}

func (c *Client) AllContainers(ctx context.Context) ([]ContainerInfo, error) {
	var items []listItem
	if err := c.doJSON(ctx, http.MethodGet, "/containers/json?all=true", nil, &items); err != nil {
		return nil, err
	}
	out := make([]ContainerInfo, 0, len(items))
	for _, item := range items {
		out = append(out, c.containerInfoFromListItem(ctx, item))
	}
	return out, nil
}

func (c *Client) Projects(ctx context.Context) ([]ProjectInfo, error) {
	containers, err := c.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	projects := map[string]*ProjectInfo{}
	for _, ci := range containers {
		d, err := c.ContainerDetail(ctx, ci.ID)
		if err != nil {
			continue
		}
		key := ci.ID[:12]
		name := ci.Name
		typ := "docker"
		if d.Project != "" {
			key = "compose:" + d.Project
			name = d.Project
			typ = "compose"
		}
		p := projects[key]
		if p == nil {
			p = &ProjectInfo{Key: key, Name: name, Type: typ, WorkingDir: d.WorkingDir, ConfigFile: d.ConfigFile}
			projects[key] = p
		}
		p.Containers = append(p.Containers, ci)
	}
	out := make([]ProjectInfo, 0, len(projects))
	for _, p := range projects {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "compose"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (c *Client) Project(ctx context.Context, key string) (ProjectInfo, error) {
	projects, err := c.Projects(ctx)
	if err != nil {
		return ProjectInfo{}, err
	}
	for _, p := range projects {
		if p.Key == key {
			return p, nil
		}
	}
	return ProjectInfo{}, fmt.Errorf("project not found")
}

func (c *Client) ContainerDetail(ctx context.Context, id string) (ContainerDetail, error) {
	var inspect map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &inspect); err != nil {
		return ContainerDetail{}, err
	}
	cfg, _ := inspect["Config"].(map[string]any)
	labels, _ := cfg["Labels"].(map[string]any)
	state, _ := inspect["State"].(map[string]any)
	network, _ := inspect["NetworkSettings"].(map[string]any)
	name := strings.TrimPrefix(str(inspect["Name"]), "/")
	image := str(cfg["Image"])
	if image == "" {
		image = str(inspect["Image"])
	}
	d := ContainerDetail{
		Info: ContainerInfo{
			ID:      str(inspect["Id"]),
			Name:    name,
			Image:   image,
			ImageID: str(inspect["Image"]),
		},
		State:      str(state["Status"]),
		Status:     str(state["Status"]),
		Restarting: boolVal(state["Restarting"]),
		Created:    str(inspect["Created"]),
		Started:    str(state["StartedAt"]),
		Project:    label(labels, "com.docker.compose.project"),
		Service:    label(labels, "com.docker.compose.service"),
		WorkingDir: label(labels, "com.docker.compose.project.working_dir"),
		ConfigFile: label(labels, "com.docker.compose.project.config_files"),
		Ports:      formatPorts(network["Ports"]),
	}
	d.Compose = d.Project != ""
	if health, _ := state["Health"].(map[string]any); health != nil {
		d.Health = str(health["Status"])
	}
	return d, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.postEmpty(ctx, "/containers/"+url.PathEscape(id)+"/start")
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.postEmpty(ctx, "/containers/"+url.PathEscape(id)+"/stop?t=30")
}

func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.postEmpty(ctx, "/containers/"+url.PathEscape(id)+"/restart?t=30")
}

func (c *Client) DeleteContainer(ctx context.Context, id string) error {
	return c.delete(ctx, "/containers/"+url.PathEscape(id)+"?force=true&v=false")
}

func (c *Client) containerInfoFromListItem(ctx context.Context, item listItem) ContainerInfo {
	name := item.ID[:12]
	if len(item.Names) > 0 {
		name = strings.TrimPrefix(item.Names[0], "/")
	}
	imageRef := item.Image
	var inspect map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(item.ID)+"/json", nil, &inspect); err == nil {
		if cfg, _ := inspect["Config"].(map[string]any); cfg != nil {
			if img := str(cfg["Image"]); img != "" {
				imageRef = img
			}
		}
	}
	return ContainerInfo{ID: item.ID, Name: name, Image: imageRef, ImageID: item.ImageID}
}

func (p ProjectInfo) Summary() string {
	running := 0
	for _, c := range p.Containers {
		if c.ID != "" {
			running++
		}
	}
	return fmt.Sprintf("%s · %d containers", p.Type, running)
}

func formatPorts(raw any) string {
	ports, _ := raw.(map[string]any)
	if len(ports) == 0 {
		return "-"
	}
	parts := []string{}
	for private, bindsRaw := range ports {
		binds, _ := bindsRaw.([]any)
		if len(binds) == 0 {
			parts = append(parts, private)
			continue
		}
		for _, b := range binds {
			m, _ := b.(map[string]any)
			hostIP := str(m["HostIp"])
			hostPort := str(m["HostPort"])
			if hostIP == "" || hostIP == "0.0.0.0" {
				hostIP = "*"
			}
			parts = append(parts, fmt.Sprintf("%s:%s->%s", hostIP, hostPort, private))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func label(labels map[string]any, key string) string { return str(labels[key]) }

func boolVal(v any) bool {
	b, _ := v.(bool)
	return b
}

func formatTimeShort(t string) string {
	parsed, err := time.Parse(time.RFC3339Nano, t)
	if err != nil {
		return t
	}
	return parsed.Local().Format("2006-01-02 15:04:05")
}
