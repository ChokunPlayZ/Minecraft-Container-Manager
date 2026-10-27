CREATE TABLE performance_settings (
    server_id TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    collection_mode TEXT NOT NULL DEFAULT 'off' CHECK (collection_mode IN ('off', 'rcon', 'console')),
    last_error TEXT NOT NULL DEFAULT ''
);
