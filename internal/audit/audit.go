package audit

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type Event struct {
	ID        int64
	Actor     string
	Action    string
	Target    string
	Outcome   string
	RequestID string
	ClientIP  string
	Metadata  map[string]any
	CreatedAt time.Time
}

type Store struct {
	DB *sql.DB
}

func (s Store) Record(actor, action, target, outcome, requestID, clientIP string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(redactMetadata(metadata))
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(
		`INSERT INTO audit_events (actor, action, target, outcome, request_id, client_ip, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(actor), action, nullIfEmpty(target), outcome, requestID, nullIfEmpty(clientIP), string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func redactMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "token") || strings.Contains(lk, "csrf") ||
			strings.Contains(lk, "secret") || strings.Contains(lk, "cookie") || strings.Contains(lk, "authorization") ||
			strings.Contains(lk, "credential") || lk == "url" {
			continue
		}
		if s, ok := v.(string); ok {
			if strings.Contains(s, "redis://") || strings.Contains(s, "rediss://") || strings.Contains(s, "postgresql://") {
				continue
			}
		}
		out[k] = v
	}
	return out
}

func ContainsSecret(blob string) bool {
	s := strings.ToLower(blob)
	if strings.Contains(s, "redis://") || strings.Contains(s, "rediss://") || strings.Contains(s, "postgresql://") {
		return true
	}
	if strings.Contains(s, `"password"`) || strings.Contains(s, "csrf_token") {
		return true
	}
	return false
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
