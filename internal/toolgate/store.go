// Package toolgate mints one-time launch tickets and holds short-lived tool
// cookies so pgAdmin/RedisInsight origins require a Redgres UI launch (ADR-014).
package toolgate

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	ToolPgAdmin      = "pgadmin"
	ToolRedisInsight = "redisinsight"
	ticketTTL        = 60 * time.Second
	sessionTTL       = 4 * time.Hour
	CookieName       = "redgres_tool"
	LaunchPath       = "/__redgres/launch"
)

var (
	ErrInvalidTool   = errors.New("invalid expert tool")
	ErrTicket        = errors.New("launch ticket is invalid or expired")
	ErrNotConfigured = errors.New("expert tool is not configured")
)

type ticket struct {
	tool    string
	expires time.Time
}

type session struct {
	tool    string
	expires time.Time
}

// Memory is a process-local ticket and tool-session store.
type Memory struct {
	now      func() time.Time
	mu       sync.Mutex
	tickets  map[string]ticket
	sessions map[string]session
}

func NewMemory() *Memory {
	return &Memory{
		now:      func() time.Time { return time.Now().UTC() },
		tickets:  map[string]ticket{},
		sessions: map[string]session{},
	}
}

func ValidTool(name string) bool {
	return name == ToolPgAdmin || name == ToolRedisInsight
}

func (m *Memory) Issue(tool string) (string, error) {
	if !ValidTool(tool) {
		return "", ErrInvalidTool
	}
	raw, err := randomHex(32)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(raw))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gcLocked()
	m.tickets[hex.EncodeToString(sum[:])] = ticket{tool: tool, expires: m.now().Add(ticketTTL)}
	return raw, nil
}

func (m *Memory) Consume(raw, tool string) (string, error) {
	if !ValidTool(tool) || raw == "" {
		return "", ErrTicket
	}
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tickets[key]
	if !ok || item.tool != tool || !m.now().Before(item.expires) {
		delete(m.tickets, key)
		return "", ErrTicket
	}
	delete(m.tickets, key)
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	sessSum := sha256.Sum256([]byte(token))
	m.sessions[hex.EncodeToString(sessSum[:])] = session{tool: tool, expires: m.now().Add(sessionTTL)}
	return token, nil
}

func (m *Memory) ValidSession(raw, tool string) bool {
	if raw == "" || !ValidTool(tool) {
		return false
	}
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.sessions[key]
	if !ok || item.tool != tool || !m.now().Before(item.expires) {
		delete(m.sessions, key)
		return false
	}
	return true
}

func (m *Memory) gcLocked() {
	now := m.now()
	for key, item := range m.tickets {
		if !now.Before(item.expires) {
			delete(m.tickets, key)
		}
	}
	for key, item := range m.sessions {
		if !now.Before(item.expires) {
			delete(m.sessions, key)
		}
	}
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func ConstantTimeTicket(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
