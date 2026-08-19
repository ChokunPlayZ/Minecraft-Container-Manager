CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT,
    webauthn_id   TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE servers (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    server_type  TEXT NOT NULL,
    version      TEXT NOT NULL,
    build        TEXT,
    ram_mb       INTEGER NOT NULL,
    host_port    INTEGER NOT NULL UNIQUE,
    container_id TEXT,
    state        TEXT NOT NULL DEFAULT 'stopped',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE backups (
    id         TEXT PRIMARY KEY,
    server_id  TEXT,
    name       TEXT,
    size_bytes INTEGER,
    location   TEXT,
    status     TEXT,
    created_at TEXT
);
