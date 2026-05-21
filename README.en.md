# DockUP

![Docker](https://img.shields.io/badge/Docker-Update%20Notifier-2496ED?logo=docker&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

[中文](README.md) | **English**

**DockUP is a tiny Docker update notifier with Telegram approval buttons.**

> By default, DockUP checks all running containers every 24 hours. If a newer image is available, it sends a separate Telegram message with Update and Ignore buttons for each container.

---

## 🎯 Features

- **Checks all running containers by default**
- **Checks every 24 hours by default**
- **Telegram button confirmation only**
- **Recreates containers with their original configuration**
- **Attempts rollback if an approved update fails to start**
- **Waits for Docker health checks**
- **Cleans up old images by default after approved successful updates**
- **DockUP also checks and updates itself through a temporary helper container**

DockUP does not provide a web UI, HTTP API, Slack/email/Teams notifications, stopped-container handling, or volume deletion. It only sends Telegram approval buttons when updates are found.

---

## 🚀 Quick Start

Docker Compose is recommended:

```bash
mkdir -p /opt/dockup && cd /opt/dockup
curl -Lo docker-compose.yml https://github.com/shuijiao1/DockUP/releases/latest/download/docker-compose.yml
cat > .env <<'ENV'
TZ=Asia/Shanghai
TG_BOT_TOKEN=your Telegram bot token
TG_CHAT_ID=your Telegram chat id
CHECK_INTERVAL=24h
CLEANUP=true
ENV

docker compose pull
docker compose up -d
docker compose logs -f
```

Or write your own compose file:

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
      CHECK_INTERVAL: 24h
      CLEANUP: "true"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

---

## ⚙️ Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `TG_BOT_TOKEN` | empty | Telegram Bot Token; notifications are disabled if empty |
| `TG_CHAT_ID` | empty | Telegram Chat ID; notifications are disabled if empty |
| `CHECK_INTERVAL` | `24h` | Check interval, Go duration format such as `30m`, `12h`, `24h` |
| `TZ` | `Asia/Shanghai` | Time zone |
| `CLEANUP` | `true` | Try to remove old images after approved successful updates |
| `RUN_ONCE` | `false` | Run one check and exit |
| `UPDATE_TIMEOUT` | `10m` | Timeout for one update pass |

---

## 💬 Telegram Notifications

When an update is found, DockUP sends one Telegram message per container with two buttons: `Update` and `Ignore`. No-update runs are logged only.

---

## 🛠 How It Works

For each run, DockUP:

1. Lists all running containers
2. Pulls the current `image:tag` used by each container
3. Compares the image ID before and after pulling
4. If the image changed:
   - Sends a per-container Telegram message with buttons
   - After `Update` is clicked, stops the old container
   - Renames it as a backup container
   - Creates a new container with the original configuration
   - Starts the new container
   - Waits for health checks to become healthy
   - Removes the backup container after success
   - Optionally removes the old image
5. If the new container fails to start or becomes unhealthy, DockUP attempts to remove it and restore the old container

---

## ⚠️ Notes

DockUP is intentionally direct: if an update is found, it asks for confirmation in Telegram.

That means:

- A bad upstream image may still break your service if you approve the update
- Core services such as databases, reverse proxies, and dashboards should be reviewed before clicking Update
- Mounting `/var/run/docker.sock` gives DockUP Docker management access on the host

If you need allowlists, complex approval workflows, multiple notification channels, or orchestration, DockUP is not the right tool. It is designed to stay small and simple.

---

## 📦 Image

```bash
docker pull ghcr.io/shuijiao1/dockup:latest
```

Supported platforms:

- `linux/amd64`
- `linux/arm64`

---

## 🔐 Privacy

DockUP does not upload your container list or configuration. Network requests are limited to:

- Pulling images from Docker registries to detect updates
- Sending button notifications and receiving callback events through the Telegram Bot API

---

## 📄 License

MIT License
