package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

var errTrailingJSON = errors.New("request body must contain exactly one JSON value")

func decodeJSON(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errTrailingJSON
		}
		return err
	}
	return nil
}

func (s *Server) writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) || strings.Contains(err.Error(), "http: request body too large") {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Request body is too large")
		return
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid JSON")
		return
	}
	if strings.Contains(err.Error(), "unknown field") {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Unknown field")
		return
	}
	s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid JSON")
}
