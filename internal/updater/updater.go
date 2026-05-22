package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/shuijiao1/DockUP/internal/config"
	"github.com/shuijiao1/DockUP/internal/dockerx"
	"github.com/shuijiao1/DockUP/internal/telegram"
)

type Updater struct {
	cfg     config.Config
	docker  *dockerx.Client
	bot     *telegram.Bot
	log     *slog.Logger
	pending map[string]pendingUpdate
	mu      sync.Mutex
}

type pendingUpdate struct {
	Token      string
	Container  dockerx.ContainerInfo
	OldVersion dockerx.ImageVersion
	NewVersion dockerx.ImageVersion
	MessageID  int64
	CreatedAt  time.Time
}

func New(cfg config.Config, docker *dockerx.Client, bot *telegram.Bot, log *slog.Logger) *Updater {
	return &Updater{cfg: cfg, docker: docker, bot: bot, log: log, pending: map[string]pendingUpdate{}}
}

func (u *Updater) Run(ctx context.Context) error {
	callbacks := make(chan telegram.Callback, 32)
	if u.bot.Enabled() {
		go func() {
			if err := u.bot.PollCallbacks(ctx, callbacks); err != nil && err != context.Canceled {
				u.log.Warn("telegram callback polling stopped", "error", err)
			}
		}()
	}

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
		case cb := <-callbacks:
			u.handleCallback(ctx, cb)
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

	for _, c := range containers {
		if err := u.checkContainer(ctx, c); err != nil {
			u.log.Warn("container check failed", "container", c.Name, "image", c.Image, "error", err)
		}
	}
	return nil
}

func (u *Updater) checkContainer(ctx context.Context, c dockerx.ContainerInfo) error {
	oldVersion, err := u.docker.InspectImageVersion(ctx, c.Image)
	if err != nil {
		return err
	}
	if oldVersion.ID == "" {
		oldVersion.ID = c.ImageID
	}

	u.log.Info("pulling image", "container", c.Name, "image", c.Image)
	if err := u.docker.PullImage(ctx, c.Image); err != nil {
		return err
	}

	newVersion, err := u.docker.InspectImageVersion(ctx, c.Image)
	if err != nil {
		return err
	}
	if normalizeID(oldVersion.ID) == normalizeID(newVersion.ID) {
		return nil
	}

	return u.notifyUpdate(ctx, c, oldVersion, newVersion)
}

func (u *Updater) notifyUpdate(ctx context.Context, c dockerx.ContainerInfo, oldVersion, newVersion dockerx.ImageVersion) error {
	if !u.bot.Enabled() {
		u.log.Warn("update available but telegram is not configured", "container", c.Name, "image", c.Image, "old", oldVersion.Display(), "new", newVersion.Display())
		return nil
	}
	u.mu.Lock()
	for _, p := range u.pending {
		if p.Container.ID == c.ID && normalizeID(p.NewVersion.ID) == normalizeID(newVersion.ID) {
			u.mu.Unlock()
			return nil
		}
	}
	token := randomToken()
	p := pendingUpdate{Token: token, Container: c, OldVersion: oldVersion, NewVersion: newVersion, CreatedAt: time.Now()}
	u.pending[token] = p
	u.mu.Unlock()

	text := formatPrompt(c, oldVersion, newVersion)
	msgID, err := u.bot.SendUpdatePrompt(ctx, text, "update:"+token, "ignore:"+token)
	if err != nil {
		return err
	}
	if msgID != 0 {
		u.mu.Lock()
		p.MessageID = msgID
		u.pending[token] = p
		u.mu.Unlock()
	}
	return nil
}

func (u *Updater) handleCallback(parent context.Context, cb telegram.Callback) {
	parts := strings.SplitN(cb.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, token := parts[0], parts[1]
	if action == "noop" {
		_ = u.bot.AnswerCallback(parent, cb.ID, "测试按钮，无操作")
		return
	}

	u.mu.Lock()
	p, ok := u.pending[token]
	if ok {
		delete(u.pending, token)
	}
	u.mu.Unlock()

	if !ok {
		_ = u.bot.AnswerCallback(parent, cb.ID, "这个更新已经处理或过期")
		return
	}
	if p.MessageID == 0 {
		p.MessageID = cb.MessageID
	}

	switch action {
	case "ignore":
		_ = u.bot.AnswerCallback(parent, cb.ID, "已忽略")
		_ = u.bot.EditMessage(parent, p.MessageID, formatIgnored(p))
	case "update":
		_ = u.bot.AnswerCallback(parent, cb.ID, "开始更新")
		_ = u.bot.EditMessage(parent, p.MessageID, formatUpdating(p))
		go u.applyUpdate(parent, p)
	}
}

func (u *Updater) applyUpdate(parent context.Context, p pendingUpdate) {
	ctx, cancel := context.WithTimeout(parent, u.cfg.Timeout)
	defer cancel()

	u.log.Info("updating container", "container", p.Container.Name, "image", p.Container.Image, "old", p.OldVersion.Display(), "new", p.NewVersion.Display())
	if p.Container.Name == "dockup" {
		helperID, err := u.docker.RunSelfUpdateHelper(ctx, p.Container.Image, p.Container.ID, p.Container.Image, u.cfg.Cleanup)
		if err != nil {
			_ = u.bot.EditMessage(parent, p.MessageID, formatFailed(p, err))
			return
		}
		_ = u.bot.EditMessage(parent, p.MessageID, formatSelfUpdateStarted(p, helperID))
		return
	}
	_, _, err := u.docker.UpdateContainer(ctx, p.Container.ID, p.Container.Image, u.cfg.Cleanup)
	if err != nil {
		_ = u.bot.EditMessage(parent, p.MessageID, formatFailed(p, err))
		return
	}
	_ = u.bot.EditMessage(parent, p.MessageID, formatSuccess(p))
}

func formatPrompt(c dockerx.ContainerInfo, oldVersion, newVersion dockerx.ImageVersion) string {
	return fmt.Sprintf("🐳 发现 Docker 镜像更新\n\n容器：%s\n镜像：%s\n旧版本：%s\n新版本：%s\n\n请选择是否更新。", c.Name, c.Image, oldVersion.Display(), newVersion.Display())
}

func formatUpdating(p pendingUpdate) string {
	return fmt.Sprintf("⏳ 正在更新 Docker 容器\n\n容器：%s\n镜像：%s\n版本：%s → %s", p.Container.Name, p.Container.Image, p.OldVersion.Display(), p.NewVersion.Display())
}

func formatSuccess(p pendingUpdate) string {
	return fmt.Sprintf("✅ Docker 容器更新成功\n\n容器：%s\n镜像：%s\n版本：%s → %s", p.Container.Name, p.Container.Image, p.OldVersion.Display(), p.NewVersion.Display())
}

func formatSelfUpdateStarted(p pendingUpdate, helperID string) string {
	return fmt.Sprintf("🔄 DockUP 自更新已交给临时容器执行\n\n容器：%s\n镜像：%s\n版本：%s → %s\n临时容器：%s\n\n如果更新成功，DockUP 会以新镜像重新启动。", p.Container.Name, p.Container.Image, p.OldVersion.Display(), p.NewVersion.Display(), shortID(helperID))
}

func formatFailed(p pendingUpdate, err error) string {
	return fmt.Sprintf("❌ Docker 容器更新失败\n\n容器：%s\n镜像：%s\n版本：%s → %s\n错误：%s", p.Container.Name, p.Container.Image, p.OldVersion.Display(), p.NewVersion.Display(), err.Error())
}

func formatIgnored(p pendingUpdate) string {
	return fmt.Sprintf("⏭️ 已忽略 Docker 镜像更新\n\n容器：%s\n镜像：%s\n版本：%s → %s", p.Container.Name, p.Container.Image, p.OldVersion.Display(), p.NewVersion.Display())
}

func randomToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
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
