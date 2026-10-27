CREATE TABLE server_update_checks (
    server_id TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    server_type TEXT NOT NULL,
    version TEXT NOT NULL,
    build TEXT NOT NULL,
    software TEXT NOT NULL DEFAULT 'null',
    mods TEXT NOT NULL DEFAULT 'null',
    checked_at TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT ''
);
