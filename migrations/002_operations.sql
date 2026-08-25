CREATE TABLE operations (
    id TEXT PRIMARY KEY,
    action TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'queued','running','compensating','interrupted',
        'indeterminate','succeeded','failed','canceled'
    )),
    actor TEXT NOT NULL,
    accepted_request_id TEXT NOT NULL,
    target TEXT,
    phase TEXT,
    result_json TEXT,
    error_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT
);

CREATE INDEX operations_status_idx ON operations (status);
CREATE INDEX operations_created_at_idx ON operations (created_at);

CREATE TABLE operation_locks (
    resource_kind TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    operation_id TEXT NOT NULL REFERENCES operations(id),
    PRIMARY KEY (resource_kind, resource_name)
);

CREATE INDEX operation_locks_operation_id_idx ON operation_locks (operation_id);
