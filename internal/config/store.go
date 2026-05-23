package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.Mutex
	data StoreData
}

type StoreData struct {
	Servers []StoredAgent `json:"servers"`
	Pairs   []PendingPair `json:"pairs"`
}

type StoredAgent struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Token    string    `json:"token"`
	Mode     string    `json:"mode,omitempty"`
	Online   bool      `json:"online,omitempty"`
	LastSeen time.Time `json:"last_seen,omitempty"`
	Created  time.Time `json:"created"`
}

type PendingPair struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	Mode      string    `json:"mode,omitempty"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	MessageID int64     `json:"message_id,omitempty"`
}

type PairResult struct {
	Agent     StoredAgent
	MessageID int64
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/data/dockup.json"
	}
	s := &Store{path: path, data: StoreData{Servers: []StoredAgent{}, Pairs: []PendingPair{}}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	return json.Unmarshal(b, &s.data)
}

func (s *Store) saveLocked() error {
	return s.saveLockedWithBackup(false)
}

func (s *Store) saveLockedWithBackup(backup bool) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if backup {
		if err := s.backupLocked(); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(b, '\n'), 0o600)
}

func (s *Store) backupLocked() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backupDir := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("%s.%s.bak", filepath.Base(s.path), time.Now().Format("20060102-150405"))
	return os.WriteFile(filepath.Join(backupDir, name), b, 0o600)
}

func (s *Store) Agents() []AgentConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make([]AgentConfig, 0, len(s.data.Servers))
	for _, a := range s.data.Servers {
		if a.ID == "" || a.Token == "" {
			continue
		}
		out = append(out, AgentConfig{ID: a.ID, Name: a.Name, URL: a.URL, Token: a.Token, Mode: a.Mode})
	}
	_ = now
	return out
}

func (s *Store) Servers() []StoredAgent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoredAgent, len(s.data.Servers))
	copy(out, s.data.Servers)
	return out
}

func (s *Store) PendingPairs() []PendingPair {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.pruneExpiredLocked(now)
	out := make([]PendingPair, len(s.data.Pairs))
	copy(out, s.data.Pairs)
	_ = s.saveLocked()
	return out
}

func (s *Store) CreatePair(name, rawURL string, ttl time.Duration) (PendingPair, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if name == "" {
		name = DefaultAgentName(rawURL)
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	pair := PendingPair{ID: randomHex(8), Name: name, URL: rawURL, Token: randomHex(32), Mode: "reverse", Created: time.Now(), Expires: time.Now().Add(ttl)}
	if pair.ID == "" || pair.Token == "" {
		return PendingPair{}, fmt.Errorf("failed to generate pair token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(time.Now())
	s.data.Pairs = append(s.data.Pairs, pair)
	return pair, s.saveLocked()
}

func (s *Store) SetPairMessage(id string, messageID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Pairs {
		if s.data.Pairs[i].ID == id {
			s.data.Pairs[i].MessageID = messageID
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) CompletePair(id, token string) (StoredAgent, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.pruneExpiredLocked(now)
	idx := -1
	var pair PendingPair
	for i, p := range s.data.Pairs {
		if p.ID == id {
			idx = i
			pair = p
			break
		}
	}
	if idx < 0 {
		return StoredAgent{}, 0, fmt.Errorf("pair not found or expired")
	}
	if pair.Token != strings.TrimSpace(token) {
		return StoredAgent{}, 0, fmt.Errorf("invalid pair token")
	}
	agent := StoredAgent{ID: sanitizeID(pair.Name), Name: pair.Name, URL: pair.URL, Token: pair.Token, Mode: pair.Mode, Online: true, LastSeen: now, Created: now}
	if agent.ID == "" {
		agent.ID = "server-" + pair.ID
	}
	base := agent.ID
	for n := 2; s.serverIDExistsLocked(agent.ID); n++ {
		agent.ID = fmt.Sprintf("%s-%d", base, n)
	}
	s.data.Servers = append(s.data.Servers, agent)
	s.data.Pairs = append(s.data.Pairs[:idx], s.data.Pairs[idx+1:]...)
	return agent, pair.MessageID, s.saveLocked()
}

func (s *Store) TouchServerByToken(token string) (StoredAgent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Servers {
		if s.data.Servers[i].Token == strings.TrimSpace(token) {
			s.data.Servers[i].Online = true
			s.data.Servers[i].LastSeen = time.Now()
			return s.data.Servers[i], true, s.saveLocked()
		}
	}
	return StoredAgent{}, false, nil
}

func (s *Store) AddOrUpdateReverseServer(name, token string) (StoredAgent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := range s.data.Servers {
		if s.data.Servers[i].Token == strings.TrimSpace(token) {
			s.data.Servers[i].Online = true
			s.data.Servers[i].LastSeen = now
			return s.data.Servers[i], false, s.saveLocked()
		}
	}
	agent := StoredAgent{ID: sanitizeID(name), Name: strings.TrimSpace(name), Token: strings.TrimSpace(token), Mode: "reverse", Online: true, LastSeen: now, Created: now}
	if agent.Name == "" {
		agent.Name = "server-" + randomHex(4)
	}
	if agent.ID == "" {
		agent.ID = sanitizeID(agent.Name)
	}
	base := agent.ID
	for n := 2; s.serverIDExistsLocked(agent.ID); n++ {
		agent.ID = fmt.Sprintf("%s-%d", base, n)
	}
	s.data.Servers = append(s.data.Servers, agent)
	return agent, true, s.saveLocked()
}

func (s *Store) RemoveServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.data.Servers {
		if a.ID == id {
			s.data.Servers = append(s.data.Servers[:i], s.data.Servers[i+1:]...)
			return s.saveLockedWithBackup(true)
		}
	}
	return fmt.Errorf("server not found")
}

func (s *Store) serverIDExistsLocked(id string) bool {
	for _, a := range s.data.Servers {
		if a.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) pruneExpiredLocked(now time.Time) {
	pairs := s.data.Pairs[:0]
	for _, p := range s.data.Pairs {
		if p.Expires.After(now) {
			pairs = append(pairs, p)
		}
	}
	s.data.Pairs = pairs
}

func (s *Store) CompleteConnectedPairs() ([]PairResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := []PairResult{}
	pairs := s.data.Pairs[:0]
	changed := false
	for _, p := range s.data.Pairs {
		matched := -1
		for i, a := range s.data.Servers {
			if a.Token == p.Token {
				matched = i
				break
			}
		}
		if matched >= 0 {
			results = append(results, PairResult{Agent: s.data.Servers[matched], MessageID: p.MessageID})
			changed = true
			continue
		}
		pairs = append(pairs, p)
	}
	s.data.Pairs = pairs
	if changed {
		return results, s.saveLocked()
	}
	return results, nil
}

func (s *Store) ConsumeCompleted() []PairResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := []PairResult{}
	for _, a := range s.data.Servers {
		// MessageID is only available at completion time, so this method is kept
		// for future migration compatibility.
		_ = a
	}
	return results
}

func BuildAgentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Port() == "" {
		host := u.Hostname()
		if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		u.Host = net.JoinHostPort(host, "8748")
	}
	return strings.TrimRight(u.String(), "/")
}

func DefaultAgentName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "server"
	}
	u, err := url.Parse(BuildAgentURL(raw))
	if err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return raw
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
