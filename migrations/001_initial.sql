CREATE TABLE owners (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash BLOB NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    owner_id INTEGER NOT NULL REFERENCES owners(id),
    token_hash BLOB NOT NULL UNIQUE,
    csrf_hash BLOB NOT NULL,
    idle_expires_at TEXT NOT NULL,
    absolute_expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX sessions_owner_id_idx ON sessions (owner_id);
CREATE INDEX sessions_idle_expires_at_idx ON sessions (idle_expires_at);

CREATE TABLE login_attempts (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL,
    client_ip TEXT NOT NULL,
    succeeded INTEGER NOT NULL,
    attempted_at TEXT NOT NULL
);

CREATE INDEX login_attempts_lookup_idx ON login_attempts (username, client_ip, attempted_at);

CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY,
    actor TEXT,
    action TEXT NOT NULL,
    target TEXT,
    outcome TEXT NOT NULL,
    request_id TEXT NOT NULL,
    client_ip TEXT,
    metadata TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX audit_events_created_at_idx ON audit_events (created_at);
CREATE INDEX audit_events_request_id_idx ON audit_events (request_id);
