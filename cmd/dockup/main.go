package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shuijiao1/DockUP/internal/config"
	"github.com/shuijiao1/DockUP/internal/dockerx"
	"github.com/shuijiao1/DockUP/internal/telegram"
	"github.com/shuijiao1/DockUP/internal/updater"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config failed", "error", err)
		os.Exit(1)
	}

	docker, err := dockerx.New(log)
	if err != nil {
		log.Error("connect docker failed", "error", err)
		os.Exit(1)
	}
	defer docker.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if targetID := os.Getenv("DOCKUP_APPLY_CONTAINER_ID"); targetID != "" {
		imageRef := os.Getenv("DOCKUP_APPLY_IMAGE_REF")
		log.Info("DockUP self-update helper applying update", "container", targetID, "image", imageRef)
		_, _, err := docker.UpdateContainer(ctx, targetID, imageRef, cfg.Cleanup)
		if err != nil {
			log.Error("DockUP self-update helper failed", "error", err)
			os.Exit(1)
		}
		log.Info("DockUP self-update helper finished")
		return
	}

	bot := telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID)
	log.Info("DockUP booting", "version", version, "interval", cfg.CheckInterval.String(), "telegram", bot.Enabled())
	if cfg.SetupTestMessage && bot.Enabled() {
		if _, err := bot.SendSetupTest(ctx, "✅ DockUP 已成功启动\n\n这是一条 Telegram Bot 测试消息。下面两个按钮仅用于确认按钮样式，不会执行任何操作。"); err != nil {
			log.Warn("setup test message failed", "error", err)
		}
	}

	app := updater.New(cfg, docker, bot, log)
	if err := app.Run(ctx); err != nil && err != context.Canceled {
		log.Error("DockUP stopped with error", "error", err)
		os.Exit(1)
	}
}
