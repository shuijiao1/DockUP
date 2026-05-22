package updater

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shuijiao1/DockUP/internal/dockerx"
	"github.com/shuijiao1/DockUP/internal/telegram"
)

func (u *Updater) handleManageCallback(ctx context.Context, cb telegram.Callback) bool {
	data := cb.Data
	if data == "home" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "")
		u.showHome(ctx, cb.MessageID)
		return true
	}
	if data == "checkall" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "开始检查全部")
		_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "⏳ 正在检查全部容器更新…", nil)
		go func() {
			if err := u.CheckOnce(ctx); err != nil {
				_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 检查失败\n\n错误："+err.Error(), homeKeyboard())
				return
			}
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "✅ 全部容器检查完成\n\n有更新时会单独发送更新按钮通知。", homeKeyboard())
		}()
		return true
	}
	if data == "settings" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "")
		u.showSettings(ctx, cb.MessageID)
		return true
	}
	if strings.HasPrefix(data, "interval:") {
		u.handleSetInterval(ctx, cb, strings.TrimPrefix(data, "interval:"))
		return true
	}
	if strings.HasPrefix(data, "project:") {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "")
		u.showProject(ctx, cb.MessageID, strings.TrimPrefix(data, "project:"))
		return true
	}
	if strings.HasPrefix(data, "pcheck:") {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "开始检查")
		u.manualProjectCheck(ctx, cb.MessageID, strings.TrimPrefix(data, "pcheck:"))
		return true
	}
	if strings.HasPrefix(data, "pupdate:") {
		u.handleManualUpdate(ctx, cb, strings.TrimPrefix(data, "pupdate:"))
		return true
	}
	if strings.HasPrefix(data, "pstart:") || strings.HasPrefix(data, "pstop:") || strings.HasPrefix(data, "prestart:") {
		u.handleProjectAction(ctx, cb)
		return true
	}
	if strings.HasPrefix(data, "pdelete:") {
		u.confirmProjectDelete(ctx, cb, strings.TrimPrefix(data, "pdelete:"))
		return true
	}
	if strings.HasPrefix(data, "delok:") {
		u.executeProjectDelete(ctx, cb, strings.TrimPrefix(data, "delok:"))
		return true
	}
	if data == "cancel" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "已取消")
		u.showHome(ctx, cb.MessageID)
		return true
	}
	return false
}

func (u *Updater) showHome(ctx context.Context, messageID int64) {
	projects, err := u.docker.Projects(ctx)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 获取 Docker 项目失败\n\n错误："+err.Error(), homeKeyboard())
		return
	}
	text := fmt.Sprintf("🐳 DockUP Docker 管理\n\n项目数：%d\n自动检查间隔：%s\n\n选择一个项目查看状态和操作。", len(projects), u.interval().String())
	rows := [][]telegram.Button{}
	for _, p := range projects {
		icon := "🐳"
		if p.Type == "compose" {
			icon = "📦"
		}
		rows = append(rows, []telegram.Button{{Text: fmt.Sprintf("%s %s", icon, p.Name), Data: "project:" + p.Key}})
	}
	rows = append(rows, []telegram.Button{{Text: "检查全部更新", Data: "checkall"}, {Text: "设置间隔", Data: "settings"}})
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) showProject(ctx context.Context, messageID int64, key string) {
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 项目不存在", homeKeyboard())
		return
	}
	text := u.formatProjectStatus(ctx, p)
	rows := [][]telegram.Button{
		{{Text: "检查更新", Data: "pcheck:" + p.Key}, {Text: "重启", Data: "prestart:" + p.Key}},
		{{Text: "启动", Data: "pstart:" + p.Key}, {Text: "停止", Data: "pstop:" + p.Key}},
		{{Text: "删除", Data: "pdelete:" + p.Key}},
		{{Text: "返回", Data: "home"}},
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) formatProjectStatus(ctx context.Context, p dockerx.ProjectInfo) string {
	lines := []string{fmt.Sprintf("🐳 %s", p.Name), ""}
	lines = append(lines, "类型："+p.Type)
	if p.WorkingDir != "" {
		lines = append(lines, "目录："+p.WorkingDir)
	}
	lines = append(lines, fmt.Sprintf("容器：%d", len(p.Containers)), "")
	for _, c := range p.Containers {
		d, err := u.docker.ContainerDetail(ctx, c.ID)
		if err != nil {
			lines = append(lines, fmt.Sprintf("- %s：读取失败", c.Name))
			continue
		}
		state := d.State
		if d.Health != "" {
			state += "/" + d.Health
		}
		lines = append(lines, fmt.Sprintf("- %s：%s", d.Info.Name, state))
		lines = append(lines, fmt.Sprintf("  镜像：%s", d.Info.Image))
		if d.Ports != "-" {
			lines = append(lines, fmt.Sprintf("  端口：%s", d.Ports))
		}
	}
	return strings.Join(lines, "\n")
}

func (u *Updater) manualProjectCheck(ctx context.Context, messageID int64, key string) {
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 项目不存在", homeKeyboard())
		return
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "⏳ 正在检查更新："+p.Name, nil)
	updates := []pendingUpdate{}
	for _, c := range p.Containers {
		oldV, newV, err := u.checkContainerVersions(ctx, c)
		if err != nil {
			u.log.Warn("manual check failed", "container", c.Name, "error", err)
			continue
		}
		if normalizeID(oldV.ID) != normalizeID(newV.ID) {
			updates = append(updates, pendingUpdate{Token: randomToken(), Container: c, OldVersion: oldV, NewVersion: newV, MessageID: messageID, CreatedAt: time.Now()})
		}
	}
	if len(updates) == 0 {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "✅ 没有发现更新\n\n"+p.Name+" 当前已是最新。", telegram.Keyboard([][]telegram.Button{{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "首页", Data: "home"}}}))
		return
	}
	u.mu.Lock()
	for _, up := range updates {
		u.manualChecks[up.Token] = up
	}
	u.mu.Unlock()
	text := "🐳 发现更新\n\n项目：" + p.Name + "\n\n"
	rows := [][]telegram.Button{}
	for _, up := range updates {
		text += fmt.Sprintf("- %s：%s → %s\n", up.Container.Name, up.OldVersion.Display(), up.NewVersion.Display())
		rows = append(rows, []telegram.Button{{Text: "更新 " + up.Container.Name, Data: "pupdate:" + up.Token}})
	}
	rows = append(rows, []telegram.Button{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "首页", Data: "home"}})
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) checkContainerVersions(ctx context.Context, c dockerx.ContainerInfo) (dockerx.ImageVersion, dockerx.ImageVersion, error) {
	oldVersion, err := u.docker.InspectImageVersion(ctx, c.Image)
	if err != nil {
		return oldVersion, dockerx.ImageVersion{}, err
	}
	if oldVersion.ID == "" {
		oldVersion.ID = c.ImageID
	}
	if err := u.docker.PullImage(ctx, c.Image); err != nil {
		return oldVersion, dockerx.ImageVersion{}, err
	}
	newVersion, err := u.docker.InspectImageVersion(ctx, c.Image)
	return oldVersion, newVersion, err
}

func (u *Updater) handleManualUpdate(ctx context.Context, cb telegram.Callback, token string) {
	u.mu.Lock()
	p, ok := u.manualChecks[token]
	if ok {
		delete(u.manualChecks, token)
	}
	u.mu.Unlock()
	if !ok {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "这个更新已经处理或过期")
		return
	}
	_ = u.bot.AnswerCallback(ctx, cb.ID, "开始更新")
	_ = u.bot.EditMessage(ctx, cb.MessageID, formatUpdating(p))
	go u.applyUpdate(ctx, p)
}

func (u *Updater) handleProjectAction(ctx context.Context, cb telegram.Callback) {
	parts := strings.SplitN(cb.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, key := parts[0], parts[1]
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "项目不存在")
		return
	}
	_ = u.bot.AnswerCallback(ctx, cb.ID, "执行中")
	for _, c := range p.Containers {
		switch action {
		case "pstart":
			err = u.docker.StartContainer(ctx, c.ID)
		case "pstop":
			err = u.docker.StopContainer(ctx, c.ID)
		case "prestart":
			err = u.docker.RestartContainer(ctx, c.ID)
		}
		if err != nil {
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 操作失败\n\n错误："+err.Error(), telegram.Keyboard([][]telegram.Button{{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "首页", Data: "home"}}}))
			return
		}
	}
	u.showProject(ctx, cb.MessageID, p.Key)
}

func (u *Updater) confirmProjectDelete(ctx context.Context, cb telegram.Callback, key string) {
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "项目不存在")
		return
	}
	token := randomToken()
	u.mu.Lock()
	u.confirmDeletes[token] = confirmDelete{Token: token, Project: p, MessageID: cb.MessageID, CreatedAt: time.Now()}
	u.mu.Unlock()
	_ = u.bot.AnswerCallback(ctx, cb.ID, "需要二次确认")
	text := fmt.Sprintf("⚠️ 确认删除 Docker 项目？\n\n项目：%s\n类型：%s\n容器数：%d\n\n这会强制删除容器，但不会删除 volume。", p.Name, p.Type, len(p.Containers))
	rows := [][]telegram.Button{{{Text: "确认删除", Data: "delok:" + token}}, {{Text: "取消", Data: "project:" + p.Key}}}
	_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, text, telegram.Keyboard(rows))
}

func (u *Updater) executeProjectDelete(ctx context.Context, cb telegram.Callback, token string) {
	u.mu.Lock()
	p, ok := u.confirmDeletes[token]
	if ok {
		delete(u.confirmDeletes, token)
	}
	u.mu.Unlock()
	if !ok {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "删除确认已过期")
		return
	}
	_ = u.bot.AnswerCallback(ctx, cb.ID, "正在删除")
	for _, c := range p.Project.Containers {
		if err := u.docker.DeleteContainer(ctx, c.ID); err != nil {
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 删除失败\n\n错误："+err.Error(), homeKeyboard())
			return
		}
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "✅ 已删除项目："+p.Project.Name, homeKeyboard())
}

func (u *Updater) showSettings(ctx context.Context, messageID int64) {
	text := fmt.Sprintf("⚙️ 自动检查间隔\n\n当前：%s\n\n选择新的自动检查间隔。", u.interval().String())
	rows := [][]telegram.Button{
		{{Text: "30 分钟", Data: "interval:30m"}, {Text: "1 小时", Data: "interval:1h"}},
		{{Text: "6 小时", Data: "interval:6h"}, {Text: "12 小时", Data: "interval:12h"}},
		{{Text: "24 小时", Data: "interval:24h"}},
		{{Text: "返回", Data: "home"}},
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) handleSetInterval(ctx context.Context, cb telegram.Callback, raw string) {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "间隔无效")
		return
	}
	u.mu.Lock()
	u.currentInterval = d
	u.mu.Unlock()
	_ = u.bot.AnswerCallback(ctx, cb.ID, "已设置为 "+d.String())
	u.showSettings(ctx, cb.MessageID)
}

func (u *Updater) interval() time.Duration {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.currentInterval <= 0 {
		return u.cfg.CheckInterval
	}
	return u.currentInterval
}

func homeKeyboard() map[string]any {
	return telegram.Keyboard([][]telegram.Button{{{Text: "首页", Data: "home"}}})
}
