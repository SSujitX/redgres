CREATE TABLE domain_oauth_pending (
    session_id INTEGER PRIMARY KEY,
    state_hash TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    created_at TEXT NOT NULL
);
