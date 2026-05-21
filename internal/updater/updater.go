package updater

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shuijiao1/DockUP/internal/config"
	"github.com/shuijiao1/DockUP/internal/dockerx"
	"github.com/shuijiao1/DockUP/internal/telegram"
)

type Updater struct {
	cfg      config.Config
	docker   *dockerx.Client
	notifier *telegram.Notifier
	log      *slog.Logger
}

type result struct {
	Name   string
	Image  string
	Status string
	Detail string
}

func New(cfg config.Config, docker *dockerx.Client, notifier *telegram.Notifier, log *slog.Logger) *Updater {
	return &Updater{cfg: cfg, docker: docker, notifier: notifier, log: log}
}

func (u *Updater) Run(ctx context.Context) error {
	if u.cfg.RunOnce {
		return u.CheckOnce(ctx)
	}

	u.log.Info("DockUP started", "interval", u.cfg.CheckInterval.String())
	if err := u.CheckOnce(ctx); err != nil {
		u.log.Error("initial check failed", "error", err)
	}

	ticker := time.NewTicker(u.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := u.CheckOnce(ctx); err != nil {
				u.log.Error("scheduled check failed", "error", err)
			}
		}
	}
}

func (u *Updater) CheckOnce(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, u.cfg.Timeout)
	defer cancel()

	containers, err := u.docker.RunningContainers(ctx)
	if err != nil {
		return err
	}
	u.log.Info("checking containers", "count", len(containers))

	results := make([]result, 0)
	for _, c := range containers {
		if c.Name == "dockup" {
			results = append(results, result{Name: c.Name, Image: c.Image, Status: "跳过", Detail: "跳过自身容器"})
			continue
		}
		r := u.checkContainer(ctx, c)
		results = append(results, r)
	}

	changed := false
	failed := false
	for _, r := range results {
		if r.Status == "更新成功" || r.Status == "更新失败" {
			changed = true
		}
		if r.Status == "更新失败" {
			failed = true
		}
	}

	if changed || failed {
		msg := formatMessage(results)
		if err := u.notifier.Send(parent, msg); err != nil {
			u.log.Warn("telegram notification failed", "error", err)
		}
	}
	return nil
}

func (u *Updater) checkContainer(ctx context.Context, c dockerx.ContainerInfo) result {
	r := result{Name: c.Name, Image: c.Image, Status: "无更新"}
	oldImageID := c.ImageID
	if oldImageID == "" {
		var err error
		oldImageID, err = u.docker.InspectImageID(ctx, c.Image)
		if err != nil {
			r.Status = "检查失败"
			r.Detail = err.Error()
			return r
		}
	}

	u.log.Info("pulling image", "container", c.Name, "image", c.Image)
	if err := u.docker.PullImage(ctx, c.Image); err != nil {
		r.Status = "检查失败"
		r.Detail = err.Error()
		return r
	}

	newImageID, err := u.docker.InspectImageID(ctx, c.Image)
	if err != nil {
		r.Status = "检查失败"
		r.Detail = err.Error()
		return r
	}
	if normalizeID(oldImageID) == normalizeID(newImageID) {
		return r
	}

	u.log.Info("updating container", "container", c.Name, "image", c.Image, "old", shortID(oldImageID), "new", shortID(newImageID))
	_, _, err = u.docker.UpdateContainer(ctx, c.ID, c.Image, u.cfg.Cleanup)
	if err != nil {
		r.Status = "更新失败"
		r.Detail = err.Error()
		return r
	}
	r.Status = "更新成功"
	r.Detail = fmt.Sprintf("%s → %s", shortID(oldImageID), shortID(newImageID))
	return r
}

func formatMessage(results []result) string {
	var b strings.Builder
	b.WriteString("🐳 DockUP 自动更新报告\n\n")
	for _, r := range results {
		if r.Status != "更新成功" && r.Status != "更新失败" {
			continue
		}
		icon := "✅"
		if r.Status == "更新失败" {
			icon = "❌"
		}
		b.WriteString(icon + " " + r.Status + "\n")
		b.WriteString("容器：" + r.Name + "\n")
		b.WriteString("镜像：" + r.Image + "\n")
		if r.Detail != "" {
			b.WriteString("详情：" + r.Detail + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func normalizeID(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
}

func shortID(id string) string {
	id = normalizeID(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
