package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Notifier struct {
	token  string
	chatID string
	client *http.Client
}

func New(token, chatID string) *Notifier {
	return &Notifier{token: strings.TrimSpace(token), chatID: strings.TrimSpace(chatID), client: &http.Client{Timeout: 20 * time.Second}}
}

func (n *Notifier) Enabled() bool {
	return n != nil && n.token != "" && n.chatID != ""
}

func (n *Notifier) Send(ctx context.Context, text string) error {
	if !n.Enabled() || strings.TrimSpace(text) == "" {
		return nil
	}
	payload := map[string]any{
		"chat_id":                  n.chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed: %s", resp.Status)
	}
	return nil
}
