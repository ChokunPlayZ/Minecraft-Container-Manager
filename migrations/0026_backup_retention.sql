ALTER TABLE backups ADD COLUMN protected INTEGER NOT NULL DEFAULT 0 CHECK (protected IN (0, 1));
ALTER TABLE backups ADD COLUMN deleted_at TEXT NOT NULL DEFAULT '';
CREATE INDEX backups_trash ON backups(server_id, deleted_at);

CREATE TABLE backup_policies (
    server_id TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    keep_count INTEGER NOT NULL CHECK (keep_count BETWEEN 0 AND 100000),
    max_age_days INTEGER NOT NULL CHECK (max_age_days BETWEEN 0 AND 36500),
    trash_retention_days INTEGER NOT NULL CHECK (trash_retention_days BETWEEN 0 AND 36500)
);
