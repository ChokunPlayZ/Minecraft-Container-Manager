ALTER TABLE servers ADD COLUMN last_activity TEXT;
ALTER TABLE servers ADD COLUMN idle_timeout_minutes INTEGER;

INSERT OR IGNORE INTO settings (key, value) VALUES ('idle_timeout_minutes', '30');
