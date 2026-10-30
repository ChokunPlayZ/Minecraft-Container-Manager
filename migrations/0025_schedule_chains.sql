-- Chains share the existing schedule timing and run history. Their ordered
-- steps are saved atomically with the rest of the schedule configuration.
CREATE TABLE scheduled_tasks_new (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('command', 'start', 'stop', 'restart', 'backup', 'chain')),
    command TEXT NOT NULL DEFAULT '',
    backup_target TEXT NOT NULL DEFAULT '' CHECK (backup_target IN ('', 'local', 's3')),
    steps TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(steps) AND json_type(steps) = 'array'),
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

INSERT INTO scheduled_tasks_new (
    id, server_id, name, action, command, backup_target, cron, timezone, enabled,
    next_run_at, last_run_at, last_status, last_error, created_at, updated_at
) SELECT id, server_id, name, action, command, backup_target, cron, timezone, enabled,
    next_run_at, last_run_at, last_status, last_error, created_at, updated_at
FROM scheduled_tasks;

DROP TABLE scheduled_tasks;
ALTER TABLE scheduled_tasks_new RENAME TO scheduled_tasks;
CREATE INDEX scheduled_tasks_server ON scheduled_tasks(server_id, created_at);
CREATE INDEX scheduled_tasks_due ON scheduled_tasks(enabled, next_run_at);
