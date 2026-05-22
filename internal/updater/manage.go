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
	if strings.HasPrefix(data, "cmd:") {
		return u.handleCommand(ctx, cb, strings.TrimPrefix(data, "cmd:"))
	}
	if data == "home" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "")
		u.showHome(ctx, cb.MessageID)
		return true
	}
	if data == "main" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "")
		u.showMainMenu(ctx, cb.MessageID)
		return true
	}
	if data == "checkall" {
		_ = u.bot.AnswerCallback(ctx, cb.ID, "开始检查全部")
		_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "⏳ 正在检查全部容器更新…", nil)
		go func() {
			if err := u.CheckOnce(ctx); err != nil {
				_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 检查失败\n\n错误："+err.Error(), navKeyboard())
				return
			}
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "✅ 全部容器检查完成\n\n有更新时会单独发送更新按钮通知。", navKeyboard())
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

func (u *Updater) handleCommand(ctx context.Context, cb telegram.Callback, cmd string) bool {
	switch cmd {
	case "start":
		u.sendMainMenu(ctx)
	case "docker":
		u.sendHome(ctx)
	case "settings":
		msg, _ := u.bot.SendMessage(ctx, "⚙️ 自动检查间隔", nil)
		u.showSettings(ctx, msg)
	case "checkall":
		msg, _ := u.bot.SendMessage(ctx, "⏳ 正在检查全部容器更新…", nil)
		go func() {
			if err := u.CheckOnce(ctx); err != nil {
				_ = u.bot.EditMessageWithKeyboard(ctx, msg, "❌ 检查失败\n\n错误："+err.Error(), navKeyboard())
				return
			}
			_ = u.bot.EditMessageWithKeyboard(ctx, msg, "✅ 全部容器检查完成\n\n有更新时会单独发送更新按钮通知。", navKeyboard())
		}()
	}
	return true
}

func (u *Updater) sendMainMenu(ctx context.Context) {
	_, _ = u.bot.SendMainMenu(ctx, mainMenuText(u.interval()))
}

func (u *Updater) showMainMenu(ctx context.Context, messageID int64) {
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, mainMenuText(u.interval()), telegram.MainMenuKeyboard())
}

func mainMenuText(interval time.Duration) string {
	return fmt.Sprintf("🐳 DockUP\n\n轻量 Telegram Docker 管理工具\n自动检查间隔：%s\n\n可查看 Docker / Compose 项目、手动检查更新、启动/停止/重启、二次确认删除。", interval.String())
}

func (u *Updater) sendHome(ctx context.Context) {
	msg, _ := u.bot.SendMessage(ctx, "🐳 Docker 管理", nil)
	u.showHome(ctx, msg)
}

func (u *Updater) showHome(ctx context.Context, messageID int64) {
	projects, err := u.docker.Projects(ctx)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 获取 Docker 项目失败\n\n错误："+err.Error(), navKeyboard())
		return
	}
	running := 0
	for _, p := range projects {
		for _, c := range p.Containers {
			d, err := u.docker.ContainerDetail(ctx, c.ID)
			if err == nil && d.State == "running" {
				running++
			}
		}
	}
	text := fmt.Sprintf("🐳 Docker 管理\n\n项目：%d 个 · 运行中：%d 个容器\n自动检查：%s\n\n选择项目查看状态和操作。", len(projects), running, u.interval().String())
	rows := [][]telegram.Button{}
	projectButtons := []telegram.Button{}
	for _, p := range projects {
		icon := "🐳"
		if p.Type == "compose" {
			icon = "📦"
		}
		label := fmt.Sprintf("%s %s", icon, p.Name)
		if len(p.Containers) > 1 {
			label = fmt.Sprintf("%s · %d", label, len(p.Containers))
		}
		projectButtons = append(projectButtons, telegram.Button{Text: label, Data: "project:" + p.Key})
		if len(projectButtons) == 2 {
			rows = append(rows, projectButtons)
			projectButtons = []telegram.Button{}
		}
	}
	if len(projectButtons) > 0 {
		rows = append(rows, projectButtons)
	}
	rows = append(rows, []telegram.Button{{Text: "检查全部更新", Data: "checkall"}, {Text: "设置间隔", Data: "settings"}})
	rows = append(rows, []telegram.Button{{Text: "返回主界面", Data: "main"}})
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) showProject(ctx context.Context, messageID int64, key string) {
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 项目不存在", navKeyboard())
		return
	}
	text := u.formatProjectStatus(ctx, p)
	rows := [][]telegram.Button{
		{{Text: "检查更新", Data: "pcheck:" + p.Key}, {Text: "重启", Data: "prestart:" + p.Key}},
		{{Text: "启动", Data: "pstart:" + p.Key}, {Text: "停止", Data: "pstop:" + p.Key}},
		{{Text: "删除", Data: "pdelete:" + p.Key}},
		{{Text: "返回列表", Data: "home"}, {Text: "返回主界面", Data: "main"}},
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, messageID, text, telegram.Keyboard(rows))
}

func (u *Updater) formatProjectStatus(ctx context.Context, p dockerx.ProjectInfo) string {
	details := []dockerx.ContainerDetail{}
	running := 0
	healthy := 0
	for _, c := range p.Containers {
		d, err := u.docker.ContainerDetail(ctx, c.ID)
		if err != nil {
			continue
		}
		details = append(details, d)
		if d.State == "running" {
			running++
		}
		if d.Health == "healthy" {
			healthy++
		}
	}

	headStatus := "🔴 未运行"
	if running == len(p.Containers) && running > 0 {
		headStatus = "🟢 运行中"
	} else if running > 0 {
		headStatus = "🟡 部分运行"
	}
	if healthy > 0 && healthy == running {
		headStatus += " · 健康"
	}

	lines := []string{
		fmt.Sprintf("🐳 %s", p.Name),
		headStatus,
		"",
		"📌 概览",
		fmt.Sprintf("🏷️ 类型：%s", typeLabel(p.Type)),
		fmt.Sprintf("📦 容器：%d 个 · 运行中 %d 个", len(p.Containers), running),
	}
	if p.WorkingDir != "" {
		lines = append(lines, "📁 目录："+p.WorkingDir)
	}
	if p.ConfigFile != "" {
		lines = append(lines, "🧩 Compose："+p.ConfigFile)
	}
	lines = append(lines, "")

	if len(details) == 0 {
		lines = append(lines, "⚠️ 容器详情读取失败。")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "📦 容器详情")
	for _, d := range details {
		lines = append(lines, formatContainerDetail(d)...)
	}
	return strings.Join(lines, "\n")
}

func formatContainerDetail(d dockerx.ContainerDetail) []string {
	state := stateLabel(d.State)
	if d.Health != "" {
		state += " · " + healthLabel(d.Health)
	}
	name := d.Info.Name
	if d.Service != "" && d.Service != d.Info.Name {
		name += " / " + d.Service
	}

	lines := []string{
		fmt.Sprintf("%s %s", stateIcon(d.State), name),
		fmt.Sprintf("🚦 状态：%s", state),
		fmt.Sprintf("🖼️ 镜像：%s", d.Info.Image),
	}
	if d.State == "running" {
		lines = append(lines, fmt.Sprintf("📊 占用：CPU %.2f%% · 内存 %s", d.CPUPercent, dockerx.FormatBytes(d.Memory)))
	}
	if d.Ports != "-" {
		lines = append(lines, "🌐 端口："+d.Ports)
	}
	if d.State == "running" && d.NetRx+d.NetTx > 0 {
		lines = append(lines, fmt.Sprintf("📡 网络：↓%s ↑%s", dockerx.FormatBytes(d.NetRx), dockerx.FormatBytes(d.NetTx)))
	}
	if d.State == "running" && d.BlockRead+d.BlockWrite > 0 {
		lines = append(lines, fmt.Sprintf("💾 磁盘：读 %s · 写 %s", dockerx.FormatBytes(d.BlockRead), dockerx.FormatBytes(d.BlockWrite)))
	}
	meta := []string{fmt.Sprintf("🆔 ID：%s", shortID(d.Info.ID))}
	if d.Restarts > 0 {
		meta = append(meta, fmt.Sprintf("🔁 重启：%d", d.Restarts))
	}
	if d.Started != "-" {
		meta = append(meta, "⏱️ 启动："+d.Started)
	}
	lines = append(lines, strings.Join(meta, " · "))
	lines = append(lines, "")
	return lines
}

func (u *Updater) manualProjectCheck(ctx context.Context, messageID int64, key string) {
	p, err := u.docker.Project(ctx, key)
	if err != nil {
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "❌ 项目不存在", navKeyboard())
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
		_ = u.bot.EditMessageWithKeyboard(ctx, messageID, "✅ 没有发现更新\n\n"+p.Name+" 当前已是最新。", telegram.Keyboard([][]telegram.Button{{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "返回列表", Data: "home"}}, {{Text: "返回主界面", Data: "main"}}}))
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
		text += fmt.Sprintf("• %s：%s → %s\n", up.Container.Name, up.OldVersion.Display(), up.NewVersion.Display())
		rows = append(rows, []telegram.Button{{Text: "更新 " + up.Container.Name, Data: "pupdate:" + up.Token}})
	}
	rows = append(rows, []telegram.Button{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "返回列表", Data: "home"}})
	rows = append(rows, []telegram.Button{{Text: "返回主界面", Data: "main"}})
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
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 操作失败\n\n错误："+err.Error(), telegram.Keyboard([][]telegram.Button{{{Text: "返回项目", Data: "project:" + p.Key}, {Text: "返回列表", Data: "home"}}, {{Text: "返回主界面", Data: "main"}}}))
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
	text := fmt.Sprintf("⚠️ 确认删除 Docker 项目？\n\n项目：%s\n类型：%s\n容器数：%d\n\n这会强制删除容器，但不会删除 volume。", p.Name, typeLabel(p.Type), len(p.Containers))
	rows := [][]telegram.Button{{{Text: "确认删除", Data: "delok:" + token}}, {{Text: "取消", Data: "project:" + p.Key}}, {{Text: "返回主界面", Data: "main"}}}
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
			_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "❌ 删除失败\n\n错误："+err.Error(), navKeyboard())
			return
		}
	}
	_ = u.bot.EditMessageWithKeyboard(ctx, cb.MessageID, "✅ 已删除项目："+p.Project.Name, navKeyboard())
}

func (u *Updater) showSettings(ctx context.Context, messageID int64) {
	text := fmt.Sprintf("⚙️ 自动检查间隔\n\n当前：%s\n\n选择新的自动检查间隔。", u.interval().String())
	rows := [][]telegram.Button{
		{{Text: "30 分钟", Data: "interval:30m"}, {Text: "1 小时", Data: "interval:1h"}},
		{{Text: "6 小时", Data: "interval:6h"}, {Text: "12 小时", Data: "interval:12h"}},
		{{Text: "24 小时", Data: "interval:24h"}},
		{{Text: "返回列表", Data: "home"}, {Text: "返回主界面", Data: "main"}},
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

func navKeyboard() map[string]any {
	return telegram.Keyboard([][]telegram.Button{{{Text: "返回列表", Data: "home"}, {Text: "返回主界面", Data: "main"}}})
}

func homeKeyboard() map[string]any { return navKeyboard() }

func typeLabel(t string) string {
	if t == "compose" {
		return "📦 Compose"
	}
	return "🐳 Docker"
}

func stateIcon(s string) string {
	switch s {
	case "running":
		return "🟢"
	case "exited", "dead":
		return "🔴"
	case "paused":
		return "🟡"
	default:
		return "⚪️"
	}
}

func stateLabel(s string) string {
	switch s {
	case "running":
		return "🟢 运行中"
	case "exited":
		return "🔴 已停止"
	case "paused":
		return "🟡 已暂停"
	case "restarting":
		return "🟡 重启中"
	case "dead":
		return "🔴 dead"
	default:
		return s
	}
}

func healthLabel(s string) string {
	switch s {
	case "healthy":
		return "健康"
	case "unhealthy":
		return "不健康"
	case "starting":
		return "健康检查中"
	default:
		return s
	}
}
