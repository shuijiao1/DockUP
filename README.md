# DockUP

![Docker](https://img.shields.io/badge/Docker-Manager%20%2B%20Updater-2496ED?logo=docker&logoColor=white)
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
- **支持中心端 + 远程 Agent 管理多台 VPS**
- **远程 Agent 也参与 12 小时自动更新检查，并由中心端 Telegram 通知**
- **支持 Docker 容器和 Compose 项目识别**
- **可手动检查单个项目更新并立即更新**
- **支持启动 / 停止 / 重启 / 二次确认删除**
- **可通过按钮临时调整自动检查间隔**
- **保留原容器配置重建**
- **更新失败自动尝试回滚旧容器**
- **支持健康检查等待**
- **点击更新成功后默认清理旧镜像**
- **DockUP 本体也会检测更新，并通过临时容器执行自更新**

DockUP 不做 Web 面板、不做 Slack/邮件/Teams，也不删除 volume；自动检查和手动检查发现更新后都会给出 Telegram 按钮，是否更新由你确认。远程 VPS 通过 **Agent 主动连接中心端** 接入，不需要在远程服务器暴露 Agent 管理端口；中心端会按同一个 `CHECK_INTERVAL` 检查本机和所有远程 Agent，默认 12 小时。
---

## 🚀 快速开始

### 1. 创建 Telegram Bot

1. 在 Telegram 私聊 [@BotFather](https://t.me/BotFather)，使用 `/newbot` 创建 bot，拿到 `TG_BOT_TOKEN`
2. 获取自己的 `TG_CHAT_ID`：可以给 bot 发一条消息后访问 `https://api.telegram.org/bot<TOKEN>/getUpdates` 查看，也可以使用任意 Telegram ID 查询 bot
3. 把 bot 加到你要接收通知的聊天里，确保它能发送消息

### 2. Docker Compose 部署

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

### 3. 自定义 Compose

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

## 🌐 管理远程 VPS

DockUP 支持 **server + agent** 模式：

- **server**：Telegram 中心端，负责菜单、通知和管理入口
- **agent**：安装在每台 VPS 上，只管理本机 Docker / Compose


### Telegram 添加服务器

首次部署后默认只有本机。进入 Telegram 主菜单点击 **➕ 添加服务器**：

1. 发送服务器名称；如果不想填名称，直接发送目标服务器 IP，名称会默认使用 IP
2. Bot 会生成一条 Docker Compose 安装命令
3. 在目标服务器执行命令后，Agent 会主动连接中心端
4. 接入成功后 Bot 会通知，并可在 **🌐 远程 VPS** 页面查看和管理

接入后的服务器会持久化到 `DOCKUP_DATA`，默认 `/data/dockup.json`。

> 远程 Agent 默认不映射管理端口，只通过 `DOCKUP_PUBLIC_URL` 主动连接中心端 `/v1/reverse/connect`。中心端所在机器需要能被远程 Agent 访问到 `AGENT_LISTEN` 对应端口。

### 静态配置远程 VPS（可选）

除了 Telegram 添加，也可以在中心端 `.env` 里预置远程 Agent：

```env
DOCKUP_MODE=server
DOCKUP_PUBLIC_URL=http://你的中心端IP或域名:8748
DOCKUP_AGENTS=vps1||VPS-1|单机Token,vps2||VPS-2|另一个单机Token
```

`DOCKUP_AGENTS` 格式：

```text
id|agent_url|显示名|token
```

主动连接模式下 `agent_url` 可以留空；旧的被动 HTTP Agent 仍可填写 URL 兼容使用。多个 Agent 用英文逗号分隔。

---

## ⚙️ 配置说明

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TG_BOT_TOKEN` | 空 | Telegram Bot Token；为空则不发送通知 |
| `TG_CHAT_ID` | 空 | Telegram Chat ID；为空则不发送通知 |
| `CHECK_INTERVAL` | `12h` | 检查间隔，支持 Go duration，例如 `30m`、`1h`、`12h`；设为 `0` 可关闭自动更新检查 |
| `CHECK_LOCAL` | `true` | 中心端是否检查本机 Docker；本地/通知测试可临时设为 `false` |
| `TZ` | `Asia/Shanghai` | 时区 |
| `CLEANUP` | `true` | 点击更新且成功后是否尝试清理旧镜像 |
| `RUN_ONCE` | `false` | 只运行一轮检查后退出 |
| `UPDATE_TIMEOUT` | `10m` | 单轮更新超时时间 |
| `DOCKUP_MODE` | `server` | 运行模式：`server` 或 `agent` |
| `DOCKUP_AGENT_TOKEN` | 空 | Agent Bearer Token；通过 Telegram 添加服务器时会自动生成单机 Token |
| `DOCKUP_PUBLIC_URL` | 空 | 中心端可访问地址；Agent 主动连接中心端时必填，例如 `http://1.2.3.4:8748` |
| `DOCKUP_AGENTS` | 空 | 中心端预置远程 Agent 列表，格式：`id|url|显示名|token`，主动连接模式下 `url` 可留空 |
| `AGENT_LISTEN` | `:8748` | 中心端 reverse hub / 旧 Agent HTTP API 监听地址 |
| `AGENT_PORT` | `8748` | Docker Compose 映射到宿主机的中心端监听端口 |
| `DOCKUP_DATA` | `/data/dockup.json` | 中心端持久化服务器列表和配对信息 |
| `SETUP_TEST_MESSAGE` | `true` | 启动、重启或更新后是否发送入口消息 |

---

## 💬 Telegram 交互

每次 DockUP 启动、重启或更新后都会发送入口消息，可进入 `Docker 管理` 面板。发现更新时，每个容器都会单独发一条 Telegram 通知，并带两个按钮：`更新` / `忽略`。无更新时只写日志，不刷 Telegram。

管理面板支持：

- 查看所有 Docker / Compose 项目
- 查看项目状态、镜像、端口、CPU、内存、网络和磁盘 I/O
- 自动注册 Telegram 菜单命令：`/start`、`/docker`、`/settings`、`/checkall`
- 手动检查某个项目更新，发现更新后可立即更新
- 启动、停止、重启项目
- 删除项目前二次确认
- 临时调整自动检查间隔

命令菜单会自动注册：

| 命令 | 说明 |
| --- | --- |
| `/start` | 打开 DockUP 主菜单 |
| `/docker` | 查看 Docker / Compose 项目 |
| `/checkall` | 立即检查全部容器更新 |
| `/settings` | 设置自动检查间隔 |
| `/help` | 显示帮助和主菜单 |

更新通知示例：

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

自动检查每轮会：

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

如果你需要精细白名单、复杂审批流、多通知渠道或复杂编排，DockUP 不适合；它就是一个轻量的 Telegram Docker 管理 + 自动更新提醒工具。

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
- Telegram Bot API 发送按钮通知、注册命令菜单和接收按钮/命令回调


---

## 📄 License

MIT License
