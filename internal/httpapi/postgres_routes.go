package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/postgresadmin"
	"github.com/go-chi/chi/v5"
)

const postgresCreateTimeout = 30 * time.Second
const postgresRotateTimeout = 30 * time.Second
const postgresDuplicateTimeout = 30 * time.Second
const postgresRowsDeleteTimeout = 30 * time.Second
const postgresRowsDeleteOffMessage = "Row delete is turned off."
const postgresRowsDeleteConfirmMessage = "Type the exact table name to confirm deletion"
const postgresRowsDeletePKValuesMessage = "Select between 1 and 500 primary key values."
const postgresRotateConfirmMessage = "Type the database name exactly to confirm rotation"
const postgresRotateInProgressMessage = "A password rotation is already in progress for this role."
const postgresRotateVaultUnsyncedMessage = "The PostgreSQL password was changed but the vault could not be saved. Rotate again."
const postgresDuplicateInProgressMessage = "A database duplicate is already in progress."
const postgresDuplicateIsolationMessage = "The source database ownership or CONNECT ACL changed during duplicate. The clone was rolled back."
const postgresDuplicateSameNameMessage = "The copy must use a new database name."

type postgresListBody struct {
	postgresadmin.ListResult
	RequestID string `json:"request_id"`
}

type postgresDetailsBody struct {
	Database  postgresadmin.DatabaseDetails `json:"database"`
	RequestID string                        `json:"request_id"`
}

func (s *Server) handlePostgresDatabases(w http.ResponseWriter, r *http.Request) {
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.List(r.Context())
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresListBody{ListResult: result, RequestID: requestID(r)})
}

type postgresCreateRequest struct {
	Database string `json:"database"`
	Owner    string `json:"owner"`
}

func (s *Server) handlePostgresDatabasesCreate(w http.ResponseWriter, r *http.Request) {
	var body postgresCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	fields := map[string]string{}
	if err := postgresadmin.ValidateIdentifier(body.Database); err != nil {
		fields["database"] = "invalid"
	}
	if err := postgresadmin.ValidateIdentifier(body.Owner); err != nil {
		fields["owner"] = "invalid"
	}
	if len(fields) > 0 {
		msg := "Invalid database name"
		if _, ok := fields["database"]; !ok {
			msg = "Invalid role name."
		}
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, msg, fields)
		return
	}
	policy := postgresadmin.NewPolicy(s.cfg)
	if policy.DatabaseDenied(body.Database) || policy.OwnerDenied(body.Owner) {
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This PostgreSQL name is protected")
		return
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": body.Database, "owner": body.Owner}
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.database.create", body.Database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresCreateTimeout)
	defer cancel()
	created, err := s.postgres.Create(ctx, body.Database, body.Owner)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	cred := postgresRevealCredential{
		Username: created.Owner,
		Password: created.Password,
		OneTime:  false,
	}
	var urls postgresRevealURLs
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Direct = u
		}
	}
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Pooled = u
		}
	}
	if urls.Direct != "" || urls.Pooled != "" {
		cred.URLs = &urls
	}
	if err := s.audit.Record(sess.Username, "postgres.database.create", created.Database, "success", requestID(r), auth.ClientIP(r.RemoteAddr), map[string]any{"database": created.Database, "owner": created.Owner}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusCreated, postgresRevealResponse{
		Resource:   postgresRevealResource{Type: "postgres_database", Name: created.Database},
		Credential: cred,
		RequestID:  requestID(r),
	})
}

func (s *Server) handlePostgresDatabase(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	details, err := s.postgres.Details(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresDetailsBody{Database: details, RequestID: requestID(r)})
}

type postgresConnectionBody struct {
	postgresadmin.Connection
	MaskedDirectURL string `json:"masked_direct_url,omitempty"`
	MaskedPooledURL string `json:"masked_pooled_url,omitempty"`
	RequestID       string `json:"request_id"`
}

func (s *Server) handlePostgresConnection(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	conn, err := s.postgres.Connection(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	body := postgresConnectionBody{Connection: conn, RequestID: requestID(r)}
	if conn.SavedCredential.Status == "present" {
		if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
			if u, urlErr := postgresadmin.MaskedProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, conn.Owner, conn.Database); urlErr == nil {
				body.MaskedDirectURL = u
			}
		}
		if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
			if u, urlErr := postgresadmin.MaskedProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, conn.Owner, conn.Database); urlErr == nil {
				body.MaskedPooledURL = u
			}
		}
	}
	s.writeJSON(w, r, http.StatusOK, body)
}

type postgresRevealResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type postgresRevealURLs struct {
	Direct string `json:"direct,omitempty"`
	Pooled string `json:"pooled,omitempty"`
}

type postgresRevealCredential struct {
	Username string              `json:"username"`
	Password string              `json:"password"`
	OneTime  bool                `json:"one_time"`
	URLs     *postgresRevealURLs `json:"urls,omitempty"`
}

type postgresRevealResponse struct {
	Resource   postgresRevealResource   `json:"resource"`
	Credential postgresRevealCredential `json:"credential"`
	RequestID  string                   `json:"request_id"`
}

func (s *Server) handlePostgresConnectionReveal(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	revealed, err := s.postgres.Reveal(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	cred := postgresRevealCredential{
		Username: revealed.Owner,
		Password: revealed.Password,
		OneTime:  false,
	}
	var urls postgresRevealURLs
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, revealed.Owner, revealed.Password, revealed.Database); urlErr == nil {
			urls.Direct = u
		}
	}
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, revealed.Owner, revealed.Password, revealed.Database); urlErr == nil {
			urls.Pooled = u
		}
	}
	if urls.Direct != "" || urls.Pooled != "" {
		cred.URLs = &urls
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": revealed.Database, "owner": revealed.Owner}
	if err := s.audit.Record(sess.Username, "postgres.credential.reveal", revealed.Database, "success", requestID(r), auth.ClientIP(r.RemoteAddr), meta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresRevealResponse{
		Resource:   postgresRevealResource{Type: "postgres_database", Name: revealed.Database},
		Credential: cred,
		RequestID:  requestID(r),
	})
}

type postgresRotateRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Server) handlePostgresCredentialsRotate(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	var body postgresRotateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	if body.Confirmation != name {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresRotateConfirmMessage, map[string]string{"confirmation": "invalid"})
		return
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": name}
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.credential.rotate", name, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresRotateTimeout)
	defer cancel()
	rotated, err := s.postgres.Rotate(ctx, name)
	if err != nil {
		var unsynced postgresadmin.VaultUnsynced
		if errors.As(err, &unsynced) {
			_ = s.audit.Record(sess.Username, "postgres.credential.rotate", unsynced.Database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), map[string]any{"database": unsynced.Database, "owner": unsynced.Owner})
		}
		s.writePostgresError(w, r, err)
		return
	}
	cred := postgresRevealCredential{
		Username: rotated.Owner,
		Password: rotated.Password,
		OneTime:  false,
	}
	var urls postgresRevealURLs
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, rotated.Owner, rotated.Password, rotated.Database); urlErr == nil {
			urls.Direct = u
		}
	}
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, rotated.Owner, rotated.Password, rotated.Database); urlErr == nil {
			urls.Pooled = u
		}
	}
	if urls.Direct != "" || urls.Pooled != "" {
		cred.URLs = &urls
	}
	if err := s.audit.Record(sess.Username, "postgres.credential.rotate", rotated.Database, "success", requestID(r), auth.ClientIP(r.RemoteAddr), map[string]any{"database": rotated.Database, "owner": rotated.Owner}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresRevealResponse{
		Resource:   postgresRevealResource{Type: "postgres_database", Name: rotated.Database},
		Credential: cred,
		RequestID:  requestID(r),
	})
}

type postgresDuplicateRequest struct {
	Database string `json:"database"`
	Owner    string `json:"owner"`
}

func (s *Server) handlePostgresDatabasesDuplicate(w http.ResponseWriter, r *http.Request) {
	source, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	var body postgresDuplicateRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	fields := map[string]string{}
	if err := postgresadmin.ValidateIdentifier(body.Database); err != nil {
		fields["database"] = "invalid"
	}
	if err := postgresadmin.ValidateIdentifier(body.Owner); err != nil {
		fields["owner"] = "invalid"
	}
	if len(fields) > 0 {
		msg := "Invalid database name"
		if _, ok := fields["database"]; !ok {
			msg = "Invalid role name."
		}
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, msg, fields)
		return
	}
	if source == body.Database {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresDuplicateSameNameMessage, map[string]string{"database": "invalid"})
		return
	}
	policy := postgresadmin.NewPolicy(s.cfg)
	if policy.DatabaseDenied(body.Database) || policy.OwnerDenied(body.Owner) {
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This PostgreSQL name is protected")
		return
	}
	sess := sessionFrom(r)
	meta := map[string]any{"database": body.Database, "owner": body.Owner, "source": source}
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.database.duplicate", body.Database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresDuplicateTimeout)
	defer cancel()
	created, err := s.postgres.Duplicate(ctx, source, body.Database, body.Owner)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	cred := postgresRevealCredential{
		Username: created.Owner,
		Password: created.Password,
		OneTime:  false,
	}
	var urls postgresRevealURLs
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresDirectPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresDirectPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Direct = u
		}
	}
	if s.cfg.PostgresPublicHost != "" && s.cfg.PostgresPooledPort != "" {
		if u, urlErr := postgresadmin.ProjectConnectionURL(s.cfg.PostgresPublicHost, s.cfg.PostgresPooledPort, created.Owner, created.Password, created.Database); urlErr == nil {
			urls.Pooled = u
		}
	}
	if urls.Direct != "" || urls.Pooled != "" {
		cred.URLs = &urls
	}
	if err := s.audit.Record(sess.Username, "postgres.database.duplicate", created.Database, "success", requestID(r), auth.ClientIP(r.RemoteAddr), map[string]any{"database": created.Database, "owner": created.Owner, "source": source}); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusCreated, postgresRevealResponse{
		Resource:   postgresRevealResource{Type: "postgres_database", Name: created.Database},
		Credential: cred,
		RequestID:  requestID(r),
	})
}

type postgresTablesBody struct {
	postgresadmin.TableListResult
	RequestID string `json:"request_id"`
}

func (s *Server) handlePostgresTables(w http.ResponseWriter, r *http.Request) {
	name, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.Tables(r.Context(), name)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresTablesBody{TableListResult: result, RequestID: requestID(r)})
}

type postgresRowsBody struct {
	postgresadmin.RowPage
	RequestID string `json:"request_id"`
}

type postgresSecurityBody struct {
	postgresadmin.SecurityOverview
	RequestID string `json:"request_id"`
}

func (s *Server) handlePostgresRows(w http.ResponseWriter, r *http.Request) {
	database, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	schema, err := decodePathIdentifier(chi.URLParam(r, "schema"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid schema name")
		return
	}
	table, err := decodePathIdentifier(chi.URLParam(r, "table"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid table name")
		return
	}
	q := r.URL.Query().Get("q")
	if utf8.RuneCountInString(q) > postgresadmin.MaxRowQueryRunes {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Query is too long", map[string]string{"q": "too_long"})
		return
	}
	offset, ok := parseOptionalInt(r.URL.Query().Get("offset"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid offset", map[string]string{"offset": "invalid"})
		return
	}
	limit, ok := parseOptionalInt(r.URL.Query().Get("limit"), 0)
	if !ok {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, "Invalid limit", map[string]string{"limit": "invalid"})
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.Rows(r.Context(), database, schema, table, q, offset, limit)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresRowsBody{RowPage: result, RequestID: requestID(r)})
}

type postgresPrimaryKeyBody struct {
	PrimaryKey []string `json:"primary_key"`
	RequestID  string   `json:"request_id"`
}

func (s *Server) handlePostgresPrimaryKey(w http.ResponseWriter, r *http.Request) {
	database, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	schema, err := decodePathIdentifier(chi.URLParam(r, "schema"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid schema name")
		return
	}
	table, err := decodePathIdentifier(chi.URLParam(r, "table"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid table name")
		return
	}
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	cols, err := s.postgres.PrimaryKey(r.Context(), database, schema, table)
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	if cols == nil {
		cols = []string{}
	}
	s.writeJSON(w, r, http.StatusOK, postgresPrimaryKeyBody{PrimaryKey: cols, RequestID: requestID(r)})
}

type postgresRowsDeleteRequest struct {
	TableConfirmation string `json:"table_confirmation"`
	OwnerPassword     string `json:"owner_password"`
	PrimaryKeyValues  []any  `json:"primary_key_values"`
}

type postgresRowsDeleteResponse struct {
	Deleted   int64  `json:"deleted"`
	RequestID string `json:"request_id"`
}

func (s *Server) handlePostgresRowsDelete(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.FeaturePostgresRowDelete {
		s.writeError(w, r, http.StatusForbidden, CodeForbidden, postgresRowsDeleteOffMessage)
		return
	}
	database, err := decodePathIdentifier(chi.URLParam(r, "db"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
		return
	}
	schema, err := decodePathIdentifier(chi.URLParam(r, "schema"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid schema name")
		return
	}
	table, err := decodePathIdentifier(chi.URLParam(r, "table"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid table name")
		return
	}
	var body postgresRowsDeleteRequest
	if err := decodeJSON(r, &body); err != nil {
		s.writeDecodeError(w, r, err)
		return
	}
	if body.TableConfirmation != table {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresRowsDeleteConfirmMessage, map[string]string{"table_confirmation": "invalid"})
		return
	}
	if !validRowDeletePKValues(body.PrimaryKeyValues) {
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, postgresRowsDeletePKValuesMessage, map[string]string{"primary_key_values": "invalid"})
		return
	}
	sess := sessionFrom(r)
	meta := postgresRowsDeleteMeta(database, schema, table)
	if err := auth.Reauthenticate(s.db, sess.Username, body.OwnerPassword); err != nil {
		switch {
		case errors.Is(err, auth.ErrReauthRequired):
			_ = s.audit.Record(sess.Username, "postgres.rows.delete", database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
			s.writeError(w, r, http.StatusForbidden, CodeReauthRequired, "Owner password is incorrect")
			return
		case errors.Is(err, auth.ErrUnauthorized):
			s.writeError(w, r, http.StatusUnauthorized, CodeUnauthorized, authRequiredMessage)
			return
		default:
			s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, storageUnavailable)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), postgresRowsDeleteTimeout)
	defer cancel()
	if s.postgres == nil {
		_ = s.audit.Record(sess.Username, "postgres.rows.delete", database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	deleted, err := s.postgres.DeleteRows(ctx, database, schema, table, body.PrimaryKeyValues)
	if err != nil {
		_ = s.audit.Record(sess.Username, "postgres.rows.delete", database, "failure", requestID(r), auth.ClientIP(r.RemoteAddr), meta)
		s.writePostgresError(w, r, err)
		return
	}
	successMeta := postgresRowsDeleteMeta(database, schema, table)
	successMeta["deleted"] = deleted
	if err := s.audit.Record(sess.Username, "postgres.rows.delete", database, "success", requestID(r), auth.ClientIP(r.RemoteAddr), successMeta); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresRowsDeleteResponse{Deleted: deleted, RequestID: requestID(r)})
}

func postgresRowsDeleteMeta(database, schema, table string) map[string]any {
	return map[string]any{"database": database, "schema": schema, "table": table}
}

func validRowDeletePKValues(values []any) bool {
	if len(values) < 1 || len(values) > postgresadmin.MaxRowDeleteValues {
		return false
	}
	for _, value := range values {
		switch value.(type) {
		case string, float64, bool:
		default:
			return false
		}
	}
	return true
}

func (s *Server) handlePostgresSecurity(w http.ResponseWriter, r *http.Request) {
	if s.postgres == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
		return
	}
	result, err := s.postgres.SecurityOverview(r.Context())
	if err != nil {
		s.writePostgresError(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, postgresSecurityBody{SecurityOverview: result, RequestID: requestID(r)})
}

func parseOptionalInt(raw string, fallback int) (int, bool) {
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func decodePathIdentifier(raw string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", postgresadmin.ErrInvalidIdentifier
	}
	if err := postgresadmin.ValidateIdentifier(name); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Server) writePostgresError(w http.ResponseWriter, r *http.Request, err error) {
	var duplicateInProgress postgresadmin.DuplicateInProgress
	if errors.As(err, &duplicateInProgress) {
		s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, postgresDuplicateInProgressMessage)
		return
	}
	var isolation postgresadmin.IsolationChanged
	if errors.As(err, &isolation) {
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, postgresDuplicateIsolationMessage)
		return
	}
	var field postgresadmin.FieldError
	if errors.As(err, &field) {
		key := field.Field
		if key == "" {
			key = "database"
		}
		s.writeErrorFields(w, r, http.StatusBadRequest, CodeValidationError, field.Error(), map[string]string{key: "invalid"})
		return
	}
	switch {
	case errors.Is(err, postgresadmin.ErrInvalidIdentifier):
		s.writeError(w, r, http.StatusBadRequest, CodeValidationError, "Invalid database name")
	case errors.Is(err, postgresadmin.ErrProtected):
		s.writeError(w, r, http.StatusForbidden, CodeProtectedResource, "This PostgreSQL name is protected")
	case errors.Is(err, postgresadmin.ErrOperationInProgress):
		s.writeError(w, r, http.StatusConflict, CodeOperationInProgress, postgresRotateInProgressMessage)
	case errors.Is(err, postgresadmin.ErrVaultUnsynced):
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, postgresRotateVaultUnsyncedMessage)
	case errors.Is(err, postgresadmin.ErrConflict):
		var conflict postgresadmin.Conflict
		if errors.As(err, &conflict) && conflict.Field != "" {
			s.writeErrorFields(w, r, http.StatusConflict, CodeConflict, conflict.Error(), map[string]string{conflict.Field: "exists"})
			return
		}
		s.writeError(w, r, http.StatusConflict, CodeConflict, "Conflict")
	case errors.Is(err, postgresadmin.ErrNotFound):
		s.writeError(w, r, http.StatusNotFound, CodeNotFound, "Not found")
	case errors.Is(err, postgresadmin.ErrUnavailable):
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
	default:
		s.writeError(w, r, http.StatusServiceUnavailable, CodeDependencyUnavailable, "PostgreSQL is unavailable")
	}
}
