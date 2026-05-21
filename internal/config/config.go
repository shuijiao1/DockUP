package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken string
	TelegramChatID   string
	CheckInterval    time.Duration
	TimeZone         string
	RunOnce          bool
	Cleanup          bool
	Timeout          time.Duration
	SetupTestMessage bool
}

func Load() (Config, error) {
	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TG_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("TG_CHAT_ID")),
		CheckInterval:    time.Hour,
		TimeZone:         getenv("TZ", "Asia/Shanghai"),
		Timeout:          10 * time.Minute,
	}

	if v := strings.TrimSpace(os.Getenv("CHECK_INTERVAL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			seconds, convErr := strconv.Atoi(v)
			if convErr != nil {
				return cfg, fmt.Errorf("invalid CHECK_INTERVAL %q: %w", v, err)
			}
			d = time.Duration(seconds) * time.Second
		}
		if d <= 0 {
			return cfg, fmt.Errorf("CHECK_INTERVAL must be greater than 0")
		}
		cfg.CheckInterval = d
	}

	cfg.RunOnce = parseBool(os.Getenv("RUN_ONCE"))
	cfg.Cleanup = parseBoolDefault(os.Getenv("CLEANUP"), true)
	cfg.SetupTestMessage = parseBoolDefault(os.Getenv("SETUP_TEST_MESSAGE"), true)

	if v := strings.TrimSpace(os.Getenv("UPDATE_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid UPDATE_TIMEOUT %q: %w", v, err)
		}
		if d <= 0 {
			return cfg, fmt.Errorf("UPDATE_TIMEOUT must be greater than 0")
		}
		cfg.Timeout = d
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseBool(v string) bool {
	return parseBoolDefault(v, false)
}

func parseBoolDefault(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	case "":
		return fallback
	default:
		return fallback
	}
}
