package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SSujitX/redgres/internal/operations"
)

func TestGetOperationRequiresSession(t *testing.T) {
	srv, _ := testServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+mustOperationID(t), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"operation"`) {
		t.Fatalf("401 leaked operation: %s", raw)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeUnauthorized {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestGetOperationRejectsInvalidID(t *testing.T) {
	srv, h, cookie := authedOperations(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/not-an-id", cookie, "", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeValidationError {
		t.Fatalf("code = %q", body.Error.Code)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "not-an-id") || strings.Contains(raw, `"operation"`) {
		t.Fatalf("400 leaked id or operation: %s", raw)
	}
	_ = srv
}

func TestGetOperationMissingIsNotFound(t *testing.T) {
	_, h, cookie := authedOperations(t)
	id := mustOperationID(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/"+id, cookie, "", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeNotFound {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if strings.Contains(rec.Body.String(), `"operation"`) {
		t.Fatalf("404 leaked operation: %s", rec.Body.String())
	}
}

func TestGetOperationMethodNotAllowed(t *testing.T) {
	_, h, cookie := authedOperations(t)
	id := mustOperationID(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/operations/"+id, cookie, "csrf", "{}"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeMethodNotAllowed {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestGetOperationQueuedOmitsResultAndError(t *testing.T) {
	srv, h, cookie := authedOperations(t)
	op := insertQueuedDuplicate(t, srv)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/"+op.ID, cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var body operationGetBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !requestIDOK(body.RequestID) {
		t.Fatalf("request_id = %q", body.RequestID)
	}
	if body.Operation.ID != op.ID || body.Operation.Action != string(operations.ActionDuplicate) || body.Operation.Status != string(operations.StatusQueued) {
		t.Fatalf("operation = %+v", body.Operation)
	}
	if body.Operation.Actor != "admin" || body.Operation.Target != "project_a_copy" {
		t.Fatalf("identity = %+v", body.Operation)
	}
	if body.Operation.Phase != string(operations.PhaseAccepted) {
		t.Fatalf("phase = %q", body.Operation.Phase)
	}
	if body.Operation.Result != nil || body.Operation.Error != nil {
		t.Fatalf("queued leaked result/error: %+v", body.Operation)
	}
	if body.Operation.StartedAt != nil || body.Operation.FinishedAt != nil {
		t.Fatalf("queued start/finish = %+v %+v", body.Operation.StartedAt, body.Operation.FinishedAt)
	}
	if _, err := time.Parse(time.RFC3339Nano, body.Operation.CreatedAt); err != nil {
		t.Fatalf("created_at = %q", body.Operation.CreatedAt)
	}
}

func TestGetOperationSucceededIncludesResultOnly(t *testing.T) {
	srv, h, cookie := authedOperations(t)
	op := insertQueuedDuplicate(t, srv)
	store := operations.NewStore(srv.db)
	if err := store.Transition(context.Background(), op.ID, operations.Transition{From: operations.StatusQueued, To: operations.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, operations.Transition{
		From: operations.StatusRunning,
		To:   operations.StatusSucceeded,
		Result: &operations.DuplicateResult{
			Database: "project_a_copy",
			Owner:    "app_project_a_copy",
			Source:   "project_a",
		},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/"+op.ID, cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body operationGetBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Operation.Status != string(operations.StatusSucceeded) || body.Operation.Result == nil {
		t.Fatalf("succeeded = %+v", body.Operation)
	}
	if body.Operation.Result.Database != "project_a_copy" || body.Operation.Result.Owner != "app_project_a_copy" || body.Operation.Result.Source != "project_a" {
		t.Fatalf("result = %+v", body.Operation.Result)
	}
	if body.Operation.Error != nil {
		t.Fatalf("succeeded leaked error: %+v", body.Operation.Error)
	}
	raw := rec.Body.String()
	for _, leak := range []string{"password", "postgres://", "canary-secret", "credential"} {
		if strings.Contains(strings.ToLower(raw), leak) {
			t.Fatalf("body leaked %q: %s", leak, raw)
		}
	}
}

func TestGetOperationFailedIncludesErrorOnly(t *testing.T) {
	srv, h, cookie := authedOperations(t)
	op := insertQueuedDuplicate(t, srv)
	store := operations.NewStore(srv.db)
	if err := store.Transition(context.Background(), op.ID, operations.Transition{From: operations.StatusQueued, To: operations.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(context.Background(), op.ID, operations.Transition{
		From:  operations.StatusRunning,
		To:    operations.StatusFailed,
		Error: &operations.OperationError{Code: "duplicate_incomplete", Message: "Duplicate did not create a database."},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/"+op.ID, cookie, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body operationGetBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Operation.Status != string(operations.StatusFailed) || body.Operation.Error == nil || body.Operation.Result != nil {
		t.Fatalf("failed = %+v", body.Operation)
	}
	if body.Operation.Error.Code != "duplicate_incomplete" {
		t.Fatalf("error = %+v", body.Operation.Error)
	}
}

func TestGetOperationStorageFailureDoesNotLeak(t *testing.T) {
	srv, path := testServer(t, nil)
	seedOwner(t, srv)
	cookie, _ := login(t, srv.Handler())
	if _, err := srv.db.Exec(`DROP TABLE operations`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/v1/operations/"+mustOperationID(t), cookie, "", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != CodeDependencyUnavailable || body.Error.Message != storageUnavailable {
		t.Fatalf("error = %+v", body.Error)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, `"operation"`) {
		t.Fatalf("503 leaked operation: %s", raw)
	}
	lower := strings.ToLower(raw)
	for _, leak := range []string{path, "sqlite", "modernc", "operations", "no such table"} {
		if strings.Contains(lower, strings.ToLower(leak)) {
			t.Fatalf("error leaked %q: %s", leak, raw)
		}
	}
}

func authedOperations(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	srv, _ := testServer(t, nil)
	seedOwner(t, srv)
	h := srv.Handler()
	cookie, _ := login(t, h)
	return srv, h, cookie
}

func insertQueuedDuplicate(t *testing.T, srv *Server) operations.Operation {
	t.Helper()
	op := operations.Operation{
		ID:                mustOperationID(t),
		Action:            operations.ActionDuplicate,
		Actor:             "admin",
		Target:            "project_a_copy",
		AcceptedRequestID: mustOperationID(t),
	}
	locks := []operations.ResourceLock{
		{Kind: operations.ResourceDatabase, Name: "project_a"},
		{Kind: operations.ResourceDatabase, Name: "project_a_copy"},
		{Kind: operations.ResourceRole, Name: "app_project_a_copy"},
	}
	if err := operations.NewStore(srv.db).InsertQueued(context.Background(), op, locks); err != nil {
		t.Fatal(err)
	}
	return op
}

func mustOperationID(t *testing.T) string {
	t.Helper()
	id, err := operations.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
