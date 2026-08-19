-- Gateway / wake-on-rejoin: per-server wait message and last-known-good MOTD.
-- The gateway owns each server's public game port, wakes a sleeping server on
-- the first inbound connect, and serves the last-known-good MOTD while the
-- server is spun down.
ALTER TABLE servers ADD COLUMN wake_message TEXT;
ALTER TABLE servers ADD COLUMN last_motd TEXT;
ALTER TABLE servers ADD COLUMN last_motd_updated TEXT;

INSERT OR IGNORE INTO settings (key, value) VALUES ('gateway_enabled', 'false');
INSERT OR IGNORE INTO settings (key, value) VALUES ('wake_message_default',
  'Server is waking up, please wait...');
