CREATE TABLE users (
    id                   INTEGER PRIMARY KEY,
    username             TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash        TEXT NOT NULL,
    role                 TEXT NOT NULL CHECK (role IN ('viewer', 'operator', 'admin')),
    must_change_password INTEGER NOT NULL DEFAULT 0,
    failed_logins        INTEGER NOT NULL DEFAULT 0,
    locked_until         INTEGER NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
) STRICT;

CREATE TABLE sessions (
    id_hash      BLOB PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_token   TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    ip           TEXT NOT NULL,
    user_agent   TEXT NOT NULL
) STRICT;
CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE audit_log (
    id       INTEGER PRIMARY KEY,
    ts       INTEGER NOT NULL,
    user_id  INTEGER,
    username TEXT NOT NULL,
    action   TEXT NOT NULL,
    target   TEXT NOT NULL,
    ip       TEXT NOT NULL,
    details  TEXT NOT NULL
) STRICT;
CREATE INDEX audit_log_ts ON audit_log (ts);

CREATE TABLE nodes (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE COLLATE NOCASE,
    mode        TEXT NOT NULL DEFAULT 'agentless' CHECK (mode IN ('agentless', 'agent')),
    monitor_url TEXT NOT NULL,
    workers     INTEGER NOT NULL DEFAULT 1 CHECK (workers BETWEEN 1 AND 64),
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
) STRICT;

CREATE TABLE plugin_samples (
    node_id                  INTEGER NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    ts                       INTEGER NOT NULL,
    worker                   INTEGER NOT NULL,
    plugin_id                TEXT NOT NULL,
    type                     TEXT NOT NULL,
    buffer_queue_length      INTEGER,
    buffer_queue_byte_size   INTEGER,
    buffer_stage_length      INTEGER,
    buffer_stage_byte_size   INTEGER,
    buffer_total_queued_size INTEGER,
    buffer_available_ratio   REAL,
    retry_count              INTEGER,
    emit_records             INTEGER,
    emit_count               INTEGER,
    write_count              INTEGER,
    rollback_count           INTEGER,
    slow_flush_count         INTEGER,
    flush_time_count         INTEGER
) STRICT;
CREATE INDEX plugin_samples_node_ts ON plugin_samples (node_id, ts);
CREATE INDEX plugin_samples_ts ON plugin_samples (ts);
