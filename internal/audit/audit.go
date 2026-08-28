package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	maxMetadataFields = 16
	maxMetadataString = 1024
	maxMetadataDepth  = 4
)

var ErrUnsafeMetadata = errors.New("audit metadata is not allowed")

var allowedMetadataKeys = map[string]map[string]struct{}{
	"owner.login":                 keySet("username"),
	"owner.logout":                keySet("username"),
	"owner.replace":               keySet("previous_username", "username"),
	"redis.user.create":           keySet("username", "preset", "key_pattern", "queue_kind"),
	"redis.user.update":           keySet("username", "preset", "key_pattern", "queue_kind"),
	"redis.user.enable":           keySet("username"),
	"redis.user.disable":          keySet("username"),
	"redis.user.rotate":           keySet("username"),
	"redis.user.delete":           keySet("username"),
	"postgres.database.create":    keySet("database", "owner"),
	"postgres.credential.reveal":  keySet("database", "owner"),
	"postgres.credential.rotate":  keySet("database", "owner"),
	"postgres.database.duplicate": keySet("database", "owner", "source", "operation_id"),
	"postgres.rows.delete":        keySet("database", "schema", "table", "deleted"),
	"postgres.database.truncate":  keySet("database", "truncated", "total_tables"),
	"postgres.database.drop":      keySet("database", "owner", "dropped_role"),
	"domain.token.set":            keySet("configured"),
	"domain.apply":                keySet("zone", "hostname_count"),
	"domain.access.allow":         keySet("email_count"),
	"domain.confirm_reachable":    keySet("bootstrap_closed"),
	"domain.disconnect":           keySet("zone"),
	"domain.oauth.client":         keySet("configured"),
	"domain.oauth.connect":        keySet("configured"),
	"domain.tls.issue":            keySet("hostname_count"),
	"domain.manual.apply":         keySet("instruction_count"),
	"domain.manual.access":        keySet("confirmed"),
}

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
	return record(s.DB, actor, action, target, outcome, requestID, clientIP, metadata)
}

func RecordTx(tx *sql.Tx, actor, action, target, outcome, requestID, clientIP string, metadata map[string]any) error {
	return record(tx, actor, action, target, outcome, requestID, clientIP, metadata)
}

type executor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func record(exec executor, actor, action, target, outcome, requestID, clientIP string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if err := validateMetadata(action, metadata); err != nil {
		return err
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = exec.Exec(
		`INSERT INTO audit_events (actor, action, target, outcome, request_id, client_ip, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nullIfEmpty(actor), action, nullIfEmpty(target), outcome, requestID, nullIfEmpty(clientIP), string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func validateMetadata(action string, in map[string]any) error {
	allowed, ok := allowedMetadataKeys[action]
	if !ok || len(in) > maxMetadataFields {
		return ErrUnsafeMetadata
	}
	for k, v := range in {
		if _, ok := allowed[k]; !ok || unsafeMetadataKey(k) {
			return ErrUnsafeMetadata
		}
		if unsafeMetadataValue(v, 0) || !metadataScalar(v) {
			return ErrUnsafeMetadata
		}
	}
	return nil
}

func unsafeMetadataValue(value any, depth int) bool {
	if depth > maxMetadataDepth {
		return true
	}
	switch v := value.(type) {
	case string:
		lower := strings.ToLower(v)
		return len(v) > maxMetadataString ||
			strings.Contains(lower, "redis://") ||
			strings.Contains(lower, "rediss://") ||
			strings.Contains(lower, "postgres://") ||
			strings.Contains(lower, "postgresql://")
	case map[string]any:
		for key, nested := range v {
			if unsafeMetadataKey(key) || unsafeMetadataValue(nested, depth+1) {
				return true
			}
		}
	case []any:
		for _, nested := range v {
			if unsafeMetadataValue(nested, depth+1) {
				return true
			}
		}
	}
	return false
}

func unsafeMetadataKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "csrf") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "url")
}

func metadataScalar(value any) bool {
	switch value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

func keySet(keys ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		out[key] = struct{}{}
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
