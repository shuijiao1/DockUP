package dockerx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var semverTagRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
var hexRE = regexp.MustCompile(`^[0-9a-fA-F]+$`)

type ImageVersion struct {
	Ref    string
	ID     string
	Digest string
	Tag    string
}

func (v ImageVersion) Display() string {
	id := shortID(v.ID)
	if v.Tag == "" || isBareDigestTag(v.Tag) {
		return id
	}
	if id == "" {
		return v.Tag
	}
	return fmt.Sprintf("%s (%s)", v.Tag, id)
}

func isBareDigestTag(tag string) bool {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "sha256:")
	return len(tag) >= 32 && hexRE.MatchString(tag)
}

func (v ImageVersion) SameTag(other ImageVersion) bool {
	return v.Tag != "" && v.Tag == other.Tag
}

func (c *Client) InspectImageVersion(ctx context.Context, ref string) (ImageVersion, error) {
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil, &data); err != nil {
		return ImageVersion{}, err
	}

	v := ImageVersion{Ref: ref}
	if id, _ := data["Id"].(string); id != "" {
		v.ID = id
	} else if id, _ := data["ID"].(string); id != "" {
		v.ID = id
	}

	if digests, _ := data["RepoDigests"].([]any); len(digests) > 0 {
		for _, raw := range digests {
			d := str(raw)
			if strings.Contains(d, "@sha256:") {
				v.Digest = d
				break
			}
		}
	}
	if v.Digest == "" {
		v.Digest = digestFromRef(ref)
	}

	if tag := tagFromRef(ref); tag != "" && tag != "latest" && !isBareDigestTag(tag) {
		v.Tag = tag
	}
	if v.Tag == "" {
		v.Tag = imageVersionLabel(data)
	}
	if v.Tag == "" && v.Digest != "" {
		if tag, err := c.LookupVersionTag(ctx, ref, v.Digest); err == nil {
			v.Tag = tag
		}
	}
	return v, nil
}

func imageVersionLabel(data map[string]any) string {
	cfg, _ := data["Config"].(map[string]any)
	labels, _ := cfg["Labels"].(map[string]any)
	for _, key := range []string{"org.opencontainers.image.version", "org.label-schema.version", "version"} {
		if v := strings.TrimSpace(str(labels[key])); v != "" && v != "latest" && v != "main" {
			return v
		}
	}
	return ""
}

func (c *Client) InspectImageVersionByID(ctx context.Context, imageID string) (ImageVersion, error) {
	return c.InspectImageVersionByIDWithRef(ctx, imageID, "")
}

func (c *Client) InspectImageVersionByIDWithRef(ctx context.Context, imageID, imageRef string) (ImageVersion, error) {
	imageID = strings.TrimSpace(imageID)
	if imageID == "" {
		return ImageVersion{}, fmt.Errorf("empty image id")
	}
	v, err := c.InspectImageVersion(ctx, imageID)
	if err != nil {
		return v, err
	}
	if v.ID == "" {
		v.ID = imageID
	}
	imageRef = strings.TrimSpace(imageRef)
	if v.Ref == "" {
		v.Ref = imageRef
	}
	if v.Tag == "" && imageRef != "" {
		if refVersion, err := c.InspectImageVersion(ctx, imageRef); err == nil {
			if v.Digest == "" {
				v.Digest = refVersion.Digest
			}
			if v.Tag == "" {
				v.Tag = refVersion.Tag
			}
			if v.Ref == "" {
				v.Ref = refVersion.Ref
			}
		}
	}
	return v, nil
}

func (c *Client) LookupVersionTag(ctx context.Context, ref, digest string) (string, error) {
	repo, err := dockerHubRepo(ref)
	if err != nil {
		return "", err
	}
	digest = canonicalDigest(digest)
	if digest == "" {
		return "", fmt.Errorf("empty digest")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=100", repo)
	candidates := []string{}

	for page := 0; page < 5 && url != ""; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("Docker Hub tags request failed: %s", resp.Status)
		}

		var parsed struct {
			Next    string `json:"next"`
			Results []struct {
				Name   string        `json:"name"`
				Digest string        `json:"digest"`
				Images []digestImage `json:"images"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", err
		}
		for _, tag := range parsed.Results {
			if !tagMatchesDigest(tag.Digest, tag.Images, digest) {
				continue
			}
			if tag.Name == "latest" || strings.HasSuffix(tag.Name, "-latest") {
				continue
			}
			candidates = append(candidates, tag.Name)
		}
		url = parsed.Next
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching tag found for %s", digest)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return tagScore(candidates[i]) > tagScore(candidates[j])
	})
	return candidates[0], nil
}

type digestImage struct{ Digest string }

func tagMatchesDigest(tagDigest string, images []digestImage, digest string) bool {
	if canonicalDigest(tagDigest) == digest {
		return true
	}
	for _, img := range images {
		if canonicalDigest(img.Digest) == digest {
			return true
		}
	}
	return false
}

func tagScore(tag string) int {
	score := 0
	if semverTagRE.MatchString(tag) {
		score += 100
	}
	if strings.Contains(tag, "-") {
		score -= 10
	}
	if strings.Contains(tag, "latest") {
		score -= 50
	}
	return score
}

func dockerHubRepo(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "://") || strings.Contains(ref, "@") {
		return "", fmt.Errorf("unsupported image ref %q", ref)
	}

	name := ref
	parts := strings.Split(name, "/")
	if len(parts) > 0 && strings.Contains(parts[0], ".") {
		if parts[0] != "docker.io" && parts[0] != "index.docker.io" && parts[0] != "registry-1.docker.io" {
			return "", fmt.Errorf("non Docker Hub registry is not supported for tag lookup: %s", parts[0])
		}
		parts = parts[1:]
	}
	if len(parts) == 1 {
		parts = []string{"library", parts[0]}
	}
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid image ref %q", ref)
	}
	last := parts[len(parts)-1]
	if i := strings.LastIndex(last, ":"); i >= 0 {
		last = last[:i]
		parts[len(parts)-1] = last
	}
	return strings.Join(parts, "/"), nil
}

func tagFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "@") {
		return ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon <= lastSlash {
		return ""
	}
	return ref[lastColon+1:]
}

func digestFromRef(ref string) string {
	if i := strings.LastIndex(ref, "@sha256:"); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

func canonicalDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if i := strings.LastIndex(digest, "@"); i >= 0 {
		digest = digest[i+1:]
	}
	if strings.HasPrefix(digest, "sha256:") {
		return digest
	}
	return ""
}

func shortID(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
