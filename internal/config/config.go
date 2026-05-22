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
	CheckLocal       bool
	Cleanup          bool
	Timeout          time.Duration
	SetupTestMessage bool
	AgentToken       string
	AgentListen      string
	Agents           []AgentConfig
	LocalName        string
	DataPath         string
	PublicURL        string
}

type AgentConfig struct {
	ID    string
	Name  string
	URL   string
	Token string
	Mode  string
}

func Load() (Config, error) {
	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TG_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("TG_CHAT_ID")),
		CheckInterval:    12 * time.Hour,
		TimeZone:         getenv("TZ", "Asia/Shanghai"),
		Timeout:          10 * time.Minute,
		AgentListen:      getenv("AGENT_LISTEN", ":8748"),
		LocalName:        getenv("DOCKUP_NAME", "本机"),
		DataPath:         getenv("DOCKUP_DATA", "/data/dockup.json"),
		PublicURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("DOCKUP_PUBLIC_URL")), "/"),
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
		if d < 0 {
			return cfg, fmt.Errorf("CHECK_INTERVAL must be greater than or equal to 0")
		}
		cfg.CheckInterval = d
	}

	cfg.RunOnce = parseBool(os.Getenv("RUN_ONCE"))
	cfg.CheckLocal = parseBoolDefault(os.Getenv("CHECK_LOCAL"), true)
	cfg.Cleanup = parseBoolDefault(os.Getenv("CLEANUP"), true)
	cfg.SetupTestMessage = parseBoolDefault(os.Getenv("SETUP_TEST_MESSAGE"), true)
	cfg.AgentToken = strings.TrimSpace(os.Getenv("DOCKUP_AGENT_TOKEN"))
	cfg.Agents = parseAgents(os.Getenv("DOCKUP_AGENTS"))

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

func parseAgents(raw string) []AgentConfig {
	items := strings.Split(raw, ",")
	agents := []AgentConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.Split(item, "|")
		if len(parts) < 2 {
			continue
		}
		a := AgentConfig{ID: sanitizeID(parts[0]), Name: strings.TrimSpace(parts[0]), URL: strings.TrimRight(strings.TrimSpace(parts[1]), "/")}
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			a.ID = sanitizeID(parts[0])
			a.Name = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			a.Token = strings.TrimSpace(parts[3])
		}
		if a.ID == "" || a.URL == "" || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		agents = append(agents, a)
	}
	return agents
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func ModeFromEnv() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DOCKUP_MODE")))
	if mode == "agent" || mode == "pair" {
		return mode
	}
	return "server"
}
