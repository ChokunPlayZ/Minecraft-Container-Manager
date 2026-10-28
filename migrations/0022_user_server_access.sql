-- Panel roles. Admins manage everything; members manage only the servers
-- assigned to them in server_access. Existing accounts keep full access.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin';

CREATE TABLE server_access (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id  TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (user_id, server_id)
);

CREATE INDEX idx_server_access_server ON server_access(server_id);
