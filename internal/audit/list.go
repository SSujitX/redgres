package audit

import (
	"context"
	"database/sql"
)

const (
	// DefaultListLimit and MaxListLimit bound one page of audit history.
	DefaultListLimit = 50
	MaxListLimit     = 500
)

// EventSummary is the audit projection exposed to readers. It has no metadata
// field on purpose: redactMetadata is a substring heuristic applied at write
// time, so historical rows may hold whatever it let through. The read path
// never selects that column, which keeps secret material out of process memory
// instead of relying on a filter staying correct.
type EventSummary struct {
	ID        int64  `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Outcome   string `json:"outcome"`
	RequestID string `json:"request_id"`
	ClientIP  string `json:"client_ip"`
	CreatedAt string `json:"created_at"`
}

// Page is one keyset page of audit history, newest first.
type Page struct {
	Events  []EventSummary
	HasMore bool
}

// ClampListLimit bounds a caller-supplied page size. Out-of-range values fall
// back to the default rather than failing, matching the row-browse clamp.
func ClampListLimit(limit int) int {
	if limit <= 0 || limit > MaxListLimit {
		return DefaultListLimit
	}
	return limit
}

const listSelect = `SELECT id, actor, action, target, outcome, request_id, client_ip, created_at FROM audit_events`

const (
	listNewestStmt = listSelect + ` ORDER BY id DESC LIMIT ?`
	listBeforeStmt = listSelect + ` WHERE id < ? ORDER BY id DESC LIMIT ?`
)

// List returns up to limit events newest first. A before value above zero
// returns only events with a smaller id, exclusive of before itself.
//
// Paging orders by id, never by created_at. id is the SQLite rowid alias, so it
// is unique and assigned during the insert. created_at is TEXT holding
// RFC3339Nano, whose fractional second drops trailing zeros, so SQLite's string
// comparison disagrees with chronological order: "…05Z" sorts above "…05.5Z"
// while being earlier, and "…05.1Z" sorts above "…05.12Z". It is also sampled
// before the insert, so it can disagree with commit order.
func (s Store) List(ctx context.Context, before int64, limit int) (Page, error) {
	limit = ClampListLimit(limit)

	// One extra row distinguishes a full page from a final page without a count
	// over a table that grows without bound.
	var rows *sql.Rows
	var err error
	if before > 0 {
		rows, err = s.DB.QueryContext(ctx, listBeforeStmt, before, limit+1)
	} else {
		rows, err = s.DB.QueryContext(ctx, listNewestStmt, limit+1)
	}
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]EventSummary, 0, limit)
	for rows.Next() {
		var (
			event                   EventSummary
			actor, target, clientIP sql.NullString
		)
		if err := rows.Scan(
			&event.ID,
			&actor,
			&event.Action,
			&target,
			&event.Outcome,
			&event.RequestID,
			&clientIP,
			&event.CreatedAt,
		); err != nil {
			return Page{}, err
		}
		event.Actor = actor.String
		event.Target = target.String
		event.ClientIP = clientIP.String
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}

	page := Page{Events: events}
	if len(events) > limit {
		page.Events = events[:limit]
		page.HasMore = true
	}
	return page, nil
}
