package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/SSujitX/redgres/internal/operations"
	"github.com/go-chi/chi/v5"
)

type operationGetBody struct {
	Operation operationJSON `json:"operation"`
	RequestID string        `json:"request_id"`
}

type operationJSON struct {
	ID                string                      `json:"id"`
	Action            string                      `json:"action"`
	Status            string                      `json:"status"`
	Phase             string                      `json:"phase,omitempty"`
	Actor             string                      `json:"actor"`
	Target            string                      `json:"target,omitempty"`
	AcceptedRequestID string                      `json:"accepted_request_id"`
	Result            *operations.DuplicateResult `json:"result,omitempty"`
	Error             *operations.OperationError  `json:"error,omitempty"`
	CreatedAt         string                      `json:"created_at"`
	UpdatedAt         string                      `json:"updated_at"`
	StartedAt         *string                     `json:"started_at"`
	FinishedAt        *string                     `json:"finished_at"`
}

func (s *Server) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	if s.operations == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	op, err := s.operations.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, operations.ErrInvalidID) {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid operation id")
		return
	}
	if errors.Is(err, operations.ErrNotFound) {
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return
	}
	if err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, operationGetBody{
		Operation: encodeOperation(op),
		RequestID: requestID(r),
	})
}

func encodeOperation(op operations.Operation) operationJSON {
	out := operationJSON{
		ID:                op.ID,
		Action:            string(op.Action),
		Status:            string(op.Status),
		Phase:             string(op.Phase),
		Actor:             op.Actor,
		Target:            op.Target,
		AcceptedRequestID: op.AcceptedRequestID,
		CreatedAt:         op.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:         op.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if op.Status == operations.StatusSucceeded {
		out.Result = op.Result
	}
	if op.Status == operations.StatusFailed || op.Status == operations.StatusIndeterminate {
		out.Error = op.Error
	}
	if op.StartedAt != nil {
		started := op.StartedAt.UTC().Format(time.RFC3339Nano)
		out.StartedAt = &started
	}
	if op.FinishedAt != nil {
		finished := op.FinishedAt.UTC().Format(time.RFC3339Nano)
		out.FinishedAt = &finished
	}
	return out
}
