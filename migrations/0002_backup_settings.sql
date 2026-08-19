ALTER TABLE servers ADD COLUMN backup_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE servers ADD COLUMN backup_interval_minutes INTEGER NOT NULL DEFAULT 720;

INSERT OR IGNORE INTO settings (key, value) VALUES ('backup_retention', '10');
