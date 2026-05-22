# DockUP

![Docker](https://img.shields.io/badge/Docker-Update%20Notifier-2496ED?logo=docker&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)
![Version](https://img.shields.io/github/v/release/shuijiao1/DockUP?label=Release)
![GHCR](https://img.shields.io/badge/GHCR-dockup-blue)

**中文** | [English](README.en.md)

**DockUP 是一个轻量 Telegram Docker 管理工具，保留自动更新提醒，也支持手动管理容器和 Compose 项目。**

> 默认每 12 小时检测一次当前机器正在运行的 Docker 容器；也可以在 Telegram 里进入项目面板，查看状态、手动检查更新、启动/停止/重启或二次确认删除。

---

## 🎯 核心特性

- **默认检测所有运行中的容器**
- **默认每 12 小时检查一次**
- **Telegram 按钮式 Docker 管理面板**
- **支持 Docker 容器和 Compose 项目识别**
- **可手动检查单个项目更新并立即更新**
- **支持启动 / 停止 / 重启 / 二次确认删除**
- **可通过按钮临时调整自动检查间隔**
- **保留原容器配置重建**
- **更新失败自动尝试回滚旧容器**
- **支持健康检查等待**
- **点击更新成功后默认清理旧镜像**
- **DockUP 本体也会检测更新，并通过临时容器执行自更新**

DockUP 不做 Web 面板、不做 HTTP API、不做 Slack/邮件/Teams，也不删除 volume；自动检查和手动检查发现更新后都会给出 Telegram 按钮，是否更新由你确认。

---

## 🚀 快速开始

推荐使用 Docker Compose：

```bash
mkdir -p /opt/dockup && cd /opt/dockup
curl -Lo docker-compose.yml https://github.com/shuijiao1/DockUP/releases/latest/download/docker-compose.yml
cat > .env <<'ENV'
TZ=Asia/Shanghai
TG_BOT_TOKEN=你的 Telegram Bot Token
TG_CHAT_ID=你的 Telegram Chat ID
CHECK_INTERVAL=12h
CLEANUP=true
SETUP_TEST_MESSAGE=true
ENV

docker compose pull
docker compose up -d
docker compose logs -f
```

也可以直接写 compose：

```yaml
services:
  dockup:
    image: ghcr.io/shuijiao1/dockup:latest
    container_name: dockup
    restart: unless-stopped
    environment:
      TZ: Asia/Shanghai
      TG_BOT_TOKEN: your_bot_token
      TG_CHAT_ID: your_chat_id
      CHECK_INTERVAL: 12h
      CLEANUP: "true"
      SETUP_TEST_MESSAGE: "true"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

---

## ⚙️ 配置说明

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TG_BOT_TOKEN` | 空 | Telegram Bot Token；为空则不发送通知 |
| `TG_CHAT_ID` | 空 | Telegram Chat ID；为空则不发送通知 |
| `CHECK_INTERVAL` | `12h` | 检查间隔，支持 Go duration，例如 `30m`、`1h`、`12h` |
| `TZ` | `Asia/Shanghai` | 时区 |
| `CLEANUP` | `true` | 点击更新且成功后是否尝试清理旧镜像 |
| `RUN_ONCE` | `false` | 只运行一轮检查后退出 |
| `UPDATE_TIMEOUT` | `10m` | 单轮更新超时时间 |
| `SETUP_TEST_MESSAGE` | `true` | 启动、重启或更新后是否发送一条带无操作按钮的测试消息 |

---

## 💬 Telegram 通知

每次 DockUP 启动、重启或更新后都会发送入口消息，可进入 `Docker 管理` 面板。发现更新时，每个容器都会单独发一条 Telegram 通知，并带两个按钮：`更新` / `忽略`。无更新时只写日志，不刷 Telegram。

管理面板支持：

- 查看所有 Docker / Compose 项目
- 查看项目状态、镜像、端口、CPU、内存、网络和磁盘 I/O
- 自动注册 Telegram 菜单命令：`/start`、`/docker`、`/settings`、`/checkall`
- 手动检查某个项目更新，发现更新后可立即更新
- 启动、停止、重启项目
- 删除项目前二次确认
- 临时调整自动检查间隔

示例：

```text
🐳 发现 Docker 镜像更新

容器：example
镜像：nginx:latest
旧版本：2.23.18 (abc123def456)
新版本：2.23.19 (fed456cba123)

请选择是否更新。
```

---

## 🛠 工作方式

DockUP 每轮会：

1. 列出所有正在运行的容器
2. 拉取容器当前使用的 `image:tag`
3. 对比拉取前后的镜像 ID，并尽量从 registry digest 反查语义化 tag
4. 如果镜像变化：
   - 发送单容器 Telegram 按钮通知，版本优先显示 tag，拿不到 tag 时回退为短镜像 ID
   - 点击 `更新` 后停止旧容器
   - 将旧容器重命名为备份容器
   - 用原配置创建新容器
   - 启动新容器
   - 等待 healthcheck 变为 healthy
   - 成功后删除备份容器
   - 可选清理旧镜像
5. 如果新容器启动或健康检查失败，尝试删除新容器并恢复旧容器

---

## ⚠️ 注意事项

DockUP 设计上就是“发现更新就直接推送按钮给你”。这很方便，但也意味着：

- 上游镜像发坏版本时，你的容器可能会被提示更新到坏版本；点击更新前仍建议确认核心服务风险
- 数据库、反向代理、监控面板等核心服务点击更新前最好确认自己能接受风险
- DockUP 需要挂载 `/var/run/docker.sock`，等价于拥有宿主机 Docker 管理权限

如果你需要精细白名单、复杂审批流、多通知渠道或复杂编排，DockUP 不适合；它就是一个小而直接的 Docker 更新提醒 + 按钮确认工具。

---

## 📦 镜像

```bash
docker pull ghcr.io/shuijiao1/dockup:latest
docker pull ghcr.io/shuijiao1/dockup:<version>
```

支持架构：

- `linux/amd64`
- `linux/arm64`

---

## 🔐 隐私说明

DockUP 不上传你的容器列表和配置。网络请求只包括：

- Docker 镜像仓库拉取镜像用于检测更新
- Telegram Bot API 发送按钮通知和接收按钮回调


---

## 📄 License

MIT License
