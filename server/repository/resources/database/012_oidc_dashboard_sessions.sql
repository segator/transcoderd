CREATE TABLE oidc_dashboard_sessions (
    session_hash BYTEA PRIMARY KEY,
    credential_id BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
