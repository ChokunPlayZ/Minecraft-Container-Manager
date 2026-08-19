-- DNS / Cloudflare SRV publishing settings and per-server published-record
-- tracking. Publishing is disabled by default and enabled through the settings
-- UI/API once a zone, domain, and API token are configured.
CREATE TABLE IF NOT EXISTS dns_records (
    server_id TEXT PRIMARY KEY,
    record_id TEXT NOT NULL DEFAULT '',
    name      TEXT NOT NULL,
    target    TEXT NOT NULL,
    port      INTEGER NOT NULL,
    priority  INTEGER NOT NULL DEFAULT 0,
    weight    INTEGER NOT NULL DEFAULT 0,
    ttl       INTEGER NOT NULL DEFAULT 120,
    zone      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);

-- DNS publishing config lives in the generic settings table so operators can
-- toggle it live without a restart.
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_publish', 'false');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_domain', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_zone', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_api_token', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_host', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_service', '_minecraft');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_proto', '_tcp');
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_ttl', '120');
