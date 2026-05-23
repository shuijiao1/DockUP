package updater

import "strings"

func friendlyError(err error) string {
	if err == nil {
		return "未知错误"
	}
	raw := strings.TrimSpace(err.Error())
	low := strings.ToLower(raw)

	switch {
	case strings.Contains(low, "agent offline"):
		return "Agent 离线或还没有连回中心端。请确认远端 dockup-agent 正在运行，并且能访问中心端地址。"
	case strings.Contains(low, "connection refused") || strings.Contains(low, "connect: no route to host") || strings.Contains(low, "i/o timeout") || strings.Contains(low, "context deadline exceeded"):
		return "连接失败或超时。请检查网络、端口、防火墙，以及远端 Agent 是否正常运行。"
	case strings.Contains(low, "unauthorized") || strings.Contains(low, "401"):
		return "鉴权失败。请检查 Agent Token / Telegram 配置是否一致。"
	case strings.Contains(low, "pair first"):
		return "Agent 尚未完成配对。请重新在 Bot 里添加服务器并使用新的接入命令。"
	case strings.Contains(low, "permission denied") || strings.Contains(low, "connect: permission denied"):
		return "权限不足。请确认 DockUP 容器已挂载 /var/run/docker.sock，并且当前环境允许访问 Docker。"
	case strings.Contains(low, "no such image") || strings.Contains(low, "pull access denied") || strings.Contains(low, "repository does not exist"):
		return "镜像不存在或无权限拉取。请检查镜像名、Tag，或是否需要先 docker login。"
	case strings.Contains(low, "not found") && strings.Contains(low, "container"):
		return "容器不存在，可能已经被删除或刚刚重建。请刷新列表后再试。"
	case strings.Contains(low, "project not found"):
		return "项目不存在，可能已经被删除或 Compose 标签变化。请刷新列表后再试。"
	case strings.Contains(low, "server not found") || strings.Contains(low, "agent not found"):
		return "服务器记录不存在，可能已经删除。请返回远程 VPS 列表刷新。"
	case strings.Contains(low, "docker api"):
		return "Docker 操作失败：" + raw
	}
	return raw
}

func friendlyErrorText(prefix string, err error) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "❌ 操作失败"
	}
	return prefix + "\n\n错误：" + friendlyError(err)
}
