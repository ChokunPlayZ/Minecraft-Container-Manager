-- A server's chosen address must outlive its published record (which is
-- removed when the server stops).
CREATE TABLE server_dns_preferences (
    server_id TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    subdomain TEXT NOT NULL
);

INSERT INTO server_dns_preferences (server_id, subdomain)
SELECT d.server_id, d.subdomain FROM dns_records d
JOIN servers s ON s.id = d.server_id WHERE d.subdomain <> '';

ALTER TABLE dns_records ADD COLUMN provider TEXT NOT NULL DEFAULT 'cloudflare';
INSERT OR IGNORE INTO settings (key, value) VALUES ('dns_provider', 'cloudflare');
