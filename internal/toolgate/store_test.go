package toolgate

import (
	"testing"
	"time"
)

func TestIssueConsumeAndRejectReuse(t *testing.T) {
	t.Parallel()
	m := NewMemory()
	raw, err := m.Issue(ToolPgAdmin)
	if err != nil || raw == "" {
		t.Fatalf("Issue: %v %q", err, raw)
	}
	token, err := m.Consume(raw, ToolPgAdmin)
	if err != nil || token == "" {
		t.Fatalf("Consume: %v", err)
	}
	if !m.ValidSession(token, ToolPgAdmin) {
		t.Fatal("session should be valid")
	}
	if _, err := m.Consume(raw, ToolPgAdmin); err == nil {
		t.Fatal("expected reused ticket to fail")
	}
	if m.ValidSession(token, ToolRedisInsight) {
		t.Fatal("session must be tool-scoped")
	}
}

func TestExpiredTicket(t *testing.T) {
	t.Parallel()
	m := NewMemory()
	now := time.Now().UTC()
	m.now = func() time.Time { return now }
	raw, err := m.Issue(ToolRedisInsight)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now.Add(2 * ticketTTL) }
	if _, err := m.Consume(raw, ToolRedisInsight); err == nil {
		t.Fatal("expected expired ticket")
	}
}

func TestInvalidTool(t *testing.T) {
	t.Parallel()
	m := NewMemory()
	if _, err := m.Issue("other"); err == nil {
		t.Fatal("expected invalid tool")
	}
}
