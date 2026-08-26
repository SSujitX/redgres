package operations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

type store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return store{db: db}
}

func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s store) Get(ctx context.Context, id string) (Operation, error) {
	if !validID(id) {
		return Operation{}, ErrInvalidID
	}
	return scanOperation(s.db.QueryRowContext(ctx, operationSelectSQL, id))
}

const operationSelectSQL = `
SELECT id, action, status, actor, accepted_request_id, target, phase, result_json, error_json,
       created_at, updated_at, started_at, finished_at
  FROM operations
 WHERE id = ?`

func (s store) ListQueued(ctx context.Context) ([]Operation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, action, status, actor, accepted_request_id, target, phase, result_json, error_json,
       created_at, updated_at, started_at, finished_at
  FROM operations
 WHERE status = ?
 ORDER BY created_at ASC, id ASC`, string(StatusQueued))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		return []Operation{}, nil
	}
	return out, nil
}

func (s store) InsertQueued(ctx context.Context, op Operation, locks []ResourceLock) error {
	if !validID(op.ID) {
		return ErrInvalidID
	}
	if op.Action != ActionDuplicate {
		return ErrConflict
	}
	if strings.TrimSpace(op.Actor) == "" || strings.TrimSpace(op.AcceptedRequestID) == "" {
		return ErrConflict
	}
	if err := validateLocks(locks); err != nil {
		return err
	}
	if err := validateResult(op.Result); err != nil {
		return err
	}
	if err := validateError(op.Error); err != nil {
		return err
	}
	now := time.Now().UTC()
	created := op.CreatedAt
	if created.IsZero() {
		created = now
	}
	updated := op.UpdatedAt
	if updated.IsZero() {
		updated = created
	}
	phase := op.Phase
	if phase == "" {
		phase = PhaseAccepted
	}
	if unsafeResultValue(op.Actor) || unsafeResultValue(op.Target) || unsafeResultValue(string(phase)) || unsafeResultValue(op.AcceptedRequestID) {
		return ErrUnsafeResult
	}
	var resultJSON any
	if op.Result != nil {
		raw, err := json.Marshal(*op.Result)
		if err != nil {
			return err
		}
		resultJSON = string(raw)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO operations (
    id, action, status, actor, accepted_request_id, target, phase,
    result_json, error_json, created_at, updated_at, started_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, NULL, NULL)`,
		op.ID, string(ActionDuplicate), string(StatusQueued), op.Actor, op.AcceptedRequestID,
		nullIfEmpty(op.Target), nullIfEmpty(string(phase)), resultJSON,
		formatTime(created), formatTime(updated),
	)
	if err != nil {
		if isConstraint(err) {
			return ErrConflict
		}
		return err
	}
	for _, lock := range locks {
		_, err := tx.ExecContext(ctx, `
INSERT INTO operation_locks (resource_kind, resource_name, operation_id)
VALUES (?, ?, ?)`, string(lock.Kind), lock.Name, op.ID)
		if err != nil {
			if isConstraint(err) {
				return ErrLockHeld
			}
			return err
		}
	}
	return tx.Commit()
}

func (s store) Transition(ctx context.Context, id string, change Transition) error {
	return s.transition(ctx, id, change, time.Now().UTC())
}

func (s store) Reconcile(ctx context.Context, probe Probe, compensator Compensator, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	running, err := s.idsByStatus(ctx, StatusRunning)
	if err != nil {
		return err
	}
	for _, id := range running {
		if err := s.transition(ctx, id, Transition{
			From: StatusRunning,
			To:   StatusInterrupted,
		}, now); err != nil {
			return err
		}
	}
	compensating, err := s.idsByStatus(ctx, StatusCompensating)
	if err != nil {
		return err
	}
	for _, id := range compensating {
		if err := s.finishCompensation(ctx, id, compensator, now, &OperationError{
			Code:    "compensation_incomplete",
			Message: "Compensation did not finish.",
		}); err != nil {
			return err
		}
	}
	interrupted, err := s.idsByStatus(ctx, StatusInterrupted)
	if err != nil {
		return err
	}
	for _, id := range interrupted {
		if err := s.resolveInterrupted(ctx, id, probe, compensator, now); err != nil {
			return err
		}
	}
	return s.pruneTerminal(ctx, now)
}

func (s store) resolveInterrupted(ctx context.Context, id string, probe Probe, compensator Compensator, now time.Time) error {
	if probe == nil {
		return s.transition(ctx, id, Transition{
			From:       StatusInterrupted,
			To:         StatusIndeterminate,
			FinishedAt: &now,
			Error:      &OperationError{Code: "probe_unavailable", Message: "Probe could not determine duplicate state."},
		}, now)
	}
	op, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	outcome, err := probe.DuplicateState(ctx, op)
	if err != nil || outcome.Indeterminate {
		return s.transition(ctx, id, Transition{
			From:       StatusInterrupted,
			To:         StatusIndeterminate,
			FinishedAt: &now,
			Error:      &OperationError{Code: "probe_indeterminate", Message: "Probe could not determine duplicate state."},
		}, now)
	}
	if outcome.VaultRowExists && !outcome.CloneExists && !outcome.RoleExists {
		return s.transition(ctx, id, Transition{
			From:       StatusInterrupted,
			To:         StatusFailed,
			FinishedAt: &now,
			Error:      &OperationError{Code: "duplicate_incomplete", Message: "Duplicate did not create a database."},
		}, now)
	}
	created := 0
	if outcome.CloneExists {
		created++
	}
	if outcome.RoleExists {
		created++
	}
	if outcome.VaultRowExists {
		created++
	}
	switch created {
	case 0:
		return s.transition(ctx, id, Transition{
			From:       StatusInterrupted,
			To:         StatusFailed,
			FinishedAt: &now,
			Error:      &OperationError{Code: "duplicate_incomplete", Message: "Duplicate did not create a database."},
		}, now)
	case 3:
		locks, err := s.locksFor(ctx, id)
		if err != nil {
			return err
		}
		result := resultFrom(op, locks)
		return s.transition(ctx, id, Transition{
			From:       StatusInterrupted,
			To:         StatusSucceeded,
			Phase:      PhaseVaulting,
			Result:     &result,
			FinishedAt: &now,
		}, now)
	default:
		if err := s.transition(ctx, id, Transition{
			From:  StatusInterrupted,
			To:    StatusCompensating,
			Phase: PhaseCompensating,
		}, now); err != nil {
			return err
		}
		return s.finishCompensation(ctx, id, compensator, now, &OperationError{
			Code:    "duplicate_incomplete",
			Message: "Duplicate was incomplete.",
		})
	}
}

func (s store) finishCompensation(ctx context.Context, id string, compensator Compensator, now time.Time, fail *OperationError) error {
	if compensator != nil {
		op, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := compensator.CompensateDuplicate(ctx, op); err != nil {
			return s.transition(ctx, id, Transition{
				From:       StatusCompensating,
				To:         StatusIndeterminate,
				Phase:      PhaseCompensating,
				FinishedAt: &now,
				KeepLocks:  true,
				Error:      &OperationError{Code: "compensation_incomplete", Message: "Compensation did not finish."},
			}, now)
		}
	}
	return s.transition(ctx, id, Transition{
		From:       StatusCompensating,
		To:         StatusFailed,
		Phase:      PhaseCompensating,
		FinishedAt: &now,
		Error:      fail,
	}, now)
}

func (s store) pruneTerminal(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cutoff := formatTime(now.Add(-TerminalRetention))
	if _, err := tx.ExecContext(ctx, `
DELETE FROM operation_locks
 WHERE operation_id IN (
    SELECT id FROM operations
     WHERE status IN ('succeeded','failed','canceled','indeterminate')
       AND finished_at IS NOT NULL
       AND finished_at < ?
 )`, cutoff); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM operations
 WHERE status IN ('succeeded','failed','canceled','indeterminate')
   AND finished_at IS NOT NULL
   AND finished_at < ?`, cutoff); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM operations
 WHERE status IN ('succeeded','failed','canceled','indeterminate')
 ORDER BY finished_at ASC, id ASC`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if overflow := len(ids) - MaxTerminalOperations; overflow > 0 {
		for _, id := range ids[:overflow] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM operation_locks WHERE operation_id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM operations WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s store) idsByStatus(ctx context.Context, status Status) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM operations WHERE status = ?`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s store) locksFor(ctx context.Context, id string) ([]ResourceLock, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT resource_kind, resource_name
  FROM operation_locks
 WHERE operation_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var locks []ResourceLock
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return nil, err
		}
		locks = append(locks, ResourceLock{Kind: ResourceKind(kind), Name: name})
	}
	return locks, rows.Err()
}

func resultFrom(op Operation, locks []ResourceLock) DuplicateResult {
	out := DuplicateResult{Database: op.Target}
	for _, lock := range locks {
		if lock.Kind == ResourceRole && out.Owner == "" {
			out.Owner = lock.Name
		}
		if lock.Kind == ResourceDatabase && lock.Name != op.Target {
			out.Source = lock.Name
		}
	}
	return out
}

func (s store) transition(ctx context.Context, id string, change Transition, now time.Time) error {
	if !validID(id) {
		return ErrInvalidID
	}
	if !legalEdge(change.From, change.To) {
		return ErrIllegalEdge
	}
	if change.KeepLocks && change.To != StatusIndeterminate {
		return ErrIllegalEdge
	}
	if err := validateResult(change.Result); err != nil {
		return err
	}
	if err := validateError(change.Error); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getForUpdate(ctx, tx, id)
	if err != nil {
		return err
	}
	if current.Status == change.To {
		return tx.Commit()
	}
	if current.Status != change.From {
		return ErrIllegalEdge
	}
	if change.To == StatusSucceeded && change.Result == nil {
		return ErrUnsafeResult
	}

	updated := formatTime(now)
	started := nullTime(current.StartedAt)
	if change.StartedAt != nil {
		started = formatTime(*change.StartedAt)
	} else if change.To == StatusRunning && current.StartedAt == nil {
		started = updated
	}
	finished := nullTime(current.FinishedAt)
	if change.FinishedAt != nil {
		finished = formatTime(*change.FinishedAt)
	} else if terminalStatus(change.To) {
		finished = updated
	}
	phase := current.Phase
	if change.Phase != "" {
		phase = change.Phase
	}
	resultJSON, err := resultJSONFor(change.To, change.Result, current.Result)
	if err != nil {
		return err
	}
	errorJSON, err := errorJSONFor(change.To, change.Error, current.Error)
	if err != nil {
		return err
	}

	res, err := tx.ExecContext(ctx, `
UPDATE operations
   SET status = ?, phase = ?, result_json = ?, error_json = ?,
       updated_at = ?, started_at = ?, finished_at = ?
 WHERE id = ? AND status = ?`,
		string(change.To), nullIfEmpty(string(phase)), resultJSON, errorJSON,
		updated, started, finished, id, string(change.From),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrIllegalEdge
	}
	if terminalStatus(change.To) && !change.KeepLocks {
		if _, err := tx.ExecContext(ctx, `DELETE FROM operation_locks WHERE operation_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func getForUpdate(ctx context.Context, tx *sql.Tx, id string) (Operation, error) {
	return scanOperation(tx.QueryRowContext(ctx, operationSelectSQL, id))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOperation(row rowScanner) (Operation, error) {
	var (
		op         Operation
		phase      sql.NullString
		target     sql.NullString
		resultJSON sql.NullString
		errorJSON  sql.NullString
		createdAt  string
		updatedAt  string
		startedAt  sql.NullString
		finishedAt sql.NullString
		action     string
		status     string
	)
	err := row.Scan(
		&op.ID, &action, &status, &op.Actor, &op.AcceptedRequestID, &target, &phase,
		&resultJSON, &errorJSON, &createdAt, &updatedAt, &startedAt, &finishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	op.Action = Action(action)
	op.Status = Status(status)
	op.Target = target.String
	op.Phase = Phase(phase.String)
	created, err := parseTime(createdAt)
	if err != nil {
		return Operation{}, err
	}
	op.CreatedAt = created
	updated, err := parseTime(updatedAt)
	if err != nil {
		return Operation{}, err
	}
	op.UpdatedAt = updated
	if startedAt.Valid {
		t, err := parseTime(startedAt.String)
		if err != nil {
			return Operation{}, err
		}
		op.StartedAt = &t
	}
	if finishedAt.Valid {
		t, err := parseTime(finishedAt.String)
		if err != nil {
			return Operation{}, err
		}
		op.FinishedAt = &t
	}
	if resultJSON.Valid && resultJSON.String != "" {
		var result DuplicateResult
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
			return Operation{}, err
		}
		if err := checkResult(result); err != nil {
			return Operation{}, err
		}
		op.Result = &result
	}
	if errorJSON.Valid && errorJSON.String != "" && (op.Status == StatusFailed || op.Status == StatusIndeterminate) {
		var opErr OperationError
		if err := json.Unmarshal([]byte(errorJSON.String), &opErr); err != nil {
			return Operation{}, err
		}
		if err := checkError(opErr); err != nil {
			return Operation{}, err
		}
		op.Error = &opErr
	}
	return op, nil
}

func legalEdge(from, to Status) bool {
	switch from {
	case StatusQueued:
		return to == StatusRunning || to == StatusCanceled
	case StatusRunning:
		return to == StatusSucceeded || to == StatusFailed || to == StatusCompensating || to == StatusInterrupted
	case StatusCompensating:
		return to == StatusFailed || to == StatusIndeterminate
	case StatusInterrupted:
		return to == StatusSucceeded || to == StatusFailed || to == StatusCompensating || to == StatusIndeterminate
	default:
		return false
	}
}

func terminalStatus(status Status) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusCanceled, StatusIndeterminate:
		return true
	default:
		return false
	}
}

func resultJSONFor(to Status, next *DuplicateResult, current *DuplicateResult) (any, error) {
	chosen := next
	if chosen == nil {
		chosen = current
	}
	if to == StatusSucceeded && chosen == nil {
		return nil, ErrUnsafeResult
	}
	if chosen == nil {
		return nil, nil
	}
	if err := checkResult(*chosen); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(*chosen)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func errorJSONFor(to Status, next *OperationError, current *OperationError) (any, error) {
	chosen := next
	if chosen == nil {
		chosen = current
	}
	if to != StatusFailed && to != StatusIndeterminate {
		return nil, nil
	}
	if chosen == nil {
		return nil, nil
	}
	if err := checkError(*chosen); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(*chosen)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' {
			continue
		}
		return false
	}
	return true
}

func validateLocks(locks []ResourceLock) error {
	for _, lock := range locks {
		switch lock.Kind {
		case ResourceDatabase, ResourceRole, ResourceRedisUser:
		default:
			return ErrConflict
		}
		if strings.TrimSpace(lock.Name) == "" {
			return ErrConflict
		}
	}
	return nil
}

func validateResult(result *DuplicateResult) error {
	if result == nil {
		return nil
	}
	return checkResult(*result)
}

func validateError(opErr *OperationError) error {
	if opErr == nil {
		return nil
	}
	return checkError(*opErr)
}

func checkResult(result DuplicateResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return checkResultJSON(raw)
}

func checkResultJSON(raw []byte) error {
	keys, err := jsonObjectKeys(raw)
	if err != nil {
		return ErrUnsafeResult
	}
	if len(keys) != 3 || !keys["database"] || !keys["owner"] || !keys["source"] {
		return ErrUnsafeResult
	}
	for key := range keys {
		if unsafeJSONKey(key) {
			return ErrUnsafeResult
		}
	}
	var result DuplicateResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ErrUnsafeResult
	}
	if unsafeResultValue(result.Database) || unsafeResultValue(result.Owner) || unsafeResultValue(result.Source) {
		return ErrUnsafeResult
	}
	return nil
}

func checkError(opErr OperationError) error {
	raw, err := json.Marshal(opErr)
	if err != nil {
		return err
	}
	keys, err := jsonObjectKeys(raw)
	if err != nil {
		return err
	}
	for key := range keys {
		if key != "code" && key != "message" && key != "fields" {
			return ErrUnsafeError
		}
		if unsafeJSONKey(key) {
			return ErrUnsafeError
		}
	}
	if unsafeResultValue(opErr.Code) || unsafeResultValue(opErr.Message) || looksLikeRawErrorDump(opErr.Message) {
		return ErrUnsafeError
	}
	for key, value := range opErr.Fields {
		if unsafeJSONKey(key) || unsafeResultValue(value) || looksLikeRawErrorDump(value) {
			return ErrUnsafeError
		}
	}
	return nil
}

func jsonObjectKeys(raw []byte) (map[string]bool, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(obj))
	for key := range obj {
		keys[key] = true
	}
	return keys, nil
}

func unsafeJSONKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range []string{
		"password", "token", "csrf", "secret", "cookie", "authorization",
		"credential", "url", "session", "tunnel", "private", "cipher",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func unsafeResultValue(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "postgres://") ||
		strings.Contains(lower, "postgresql://") ||
		strings.Contains(lower, "redis://") ||
		strings.Contains(lower, "rediss://") ||
		strings.Contains(lower, "canary-secret") {
		return true
	}
	if looksLikeCiphertext(value) || looksLikePrivateKey(value) || looksLikeSQLWithValues(value) {
		return true
	}
	return false
}

func looksLikeCiphertext(value string) bool {
	if strings.HasPrefix(value, "gAAAA") && len(value) >= 80 {
		return true
	}
	if len(value) < 80 {
		return false
	}
	for _, c := range value {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '-' || c == '_' || c == '=' {
			continue
		}
		return false
	}
	return true
}

func looksLikePrivateKey(value string) bool {
	upper := strings.ToUpper(value)
	return strings.Contains(upper, "BEGIN") && strings.Contains(upper, "PRIVATE KEY")
}

func looksLikeSQLWithValues(value string) bool {
	lower := strings.ToLower(value)
	hasSQL := strings.Contains(lower, "insert ") ||
		strings.Contains(lower, "update ") ||
		strings.Contains(lower, "delete ") ||
		strings.Contains(lower, "select ")
	return hasSQL && (strings.Contains(value, "'") || strings.Contains(value, "\""))
}

func looksLikeRawErrorDump(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "sqlstate") ||
		strings.Contains(lower, "pq: ") ||
		strings.Contains(lower, "goroutine ") ||
		strings.Contains(value, "\n")
}

func isConstraint(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code()&0xff == 19 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "constraint failed")
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse operation time: %w", err)
	}
	return t.UTC(), nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
