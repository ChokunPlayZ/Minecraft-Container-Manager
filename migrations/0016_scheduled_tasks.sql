CREATE TABLE scheduled_tasks (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('command', 'start', 'stop', 'restart')),
    command TEXT NOT NULL DEFAULT '',
    cron TEXT NOT NULL,
    timezone TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    next_run_at TEXT NOT NULL DEFAULT '',
    last_run_at TEXT NOT NULL DEFAULT '',
    last_status TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX scheduled_tasks_server ON scheduled_tasks(server_id, created_at);
CREATE INDEX scheduled_tasks_due ON scheduled_tasks(enabled, next_run_at);
