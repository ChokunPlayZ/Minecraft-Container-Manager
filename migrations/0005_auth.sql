ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE passkeys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    credential_id TEXT NOT NULL UNIQUE,
    credential    TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
