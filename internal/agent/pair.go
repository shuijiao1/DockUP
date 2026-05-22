package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PairRequest struct {
	PairID string `json:"pair_id"`
	Token  string `json:"token"`
}

func Pair(ctx context.Context, agentURL, pairID, token string) error {
	agentURL = strings.TrimRight(strings.TrimSpace(agentURL), "/")
	if agentURL == "" || pairID == "" || token == "" {
		return fmt.Errorf("agent URL, pair ID and token are required")
	}
	body := PairRequest{PairID: pairID, Token: token}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentURL+"/v1/pair/complete", strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pair request failed: %s", resp.Status)
	}
	return nil
}
