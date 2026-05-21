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

	notifier := telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID)
	log.Info("DockUP booting", "version", version, "interval", cfg.CheckInterval.String(), "telegram", notifier.Enabled())

	app := updater.New(cfg, docker, notifier, log)
	if err := app.Run(ctx); err != nil && err != context.Canceled {
		log.Error("DockUP stopped with error", "error", err)
		os.Exit(1)
	}
}
