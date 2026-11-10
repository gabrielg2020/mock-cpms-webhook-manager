CREATE TABLE teams (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE CHECK (length(name) BETWEEN 2 AND 60),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
) STRICT;

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name  TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 80),
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    team_id       INTEGER REFERENCES teams (id) ON DELETE RESTRICT,
    failed_logins INTEGER NOT NULL DEFAULT 0,
    locked_until  TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    CHECK (role = 'admin' OR team_id IS NOT NULL)
) STRICT;

CREATE TABLE environments (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    is_production INTEGER NOT NULL CHECK (is_production IN (0, 1)),
    sort_order    INTEGER NOT NULL
) STRICT;

CREATE TABLE event_types (
    id          INTEGER PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE CHECK (code GLOB '[a-z]*.[a-z]*'),
    description TEXT NOT NULL
) STRICT;

CREATE TABLE webhooks (
    id                INTEGER PRIMARY KEY,
    team_id           INTEGER NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    environment_id    INTEGER NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    url               TEXT NOT NULL CHECK (length(url) <= 2048),
    secret_ciphertext BLOB NOT NULL,
    secret_last4      TEXT NOT NULL,
    active            INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_by        INTEGER REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (team_id, environment_id)
) STRICT;

CREATE TABLE webhook_events (
    webhook_id    INTEGER NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    event_type_id INTEGER NOT NULL REFERENCES event_types (id) ON DELETE RESTRICT,
    PRIMARY KEY (webhook_id, event_type_id)
) STRICT, WITHOUT ROWID;

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    entity_id  INTEGER,
    detail     TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    ip_address TEXT,
    request_id TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
) STRICT;

CREATE INDEX sessions_expiry_idx ON sessions (expiry);
CREATE INDEX audit_log_created_at_idx ON audit_log (created_at);
