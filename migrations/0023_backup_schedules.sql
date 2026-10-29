-- Backups share persistence, dispatch, and run results with all scheduled tasks.
CREATE TABLE scheduled_tasks_new (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('command', 'start', 'stop', 'restart', 'backup')),
    command TEXT NOT NULL DEFAULT '',
    backup_target TEXT NOT NULL DEFAULT '' CHECK (backup_target IN ('', 'local', 's3')),
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
    id, server_id, name, action, command, cron, timezone, enabled, next_run_at,
    last_run_at, last_status, last_error, created_at, updated_at
) SELECT * FROM scheduled_tasks;
DROP TABLE scheduled_tasks;
ALTER TABLE scheduled_tasks_new RENAME TO scheduled_tasks;
CREATE INDEX scheduled_tasks_server ON scheduled_tasks(server_id, created_at);
CREATE INDEX scheduled_tasks_due ON scheduled_tasks(enabled, next_run_at);

-- Preserve arbitrary minute intervals, paused settings, and the last successful
-- backup as the timing anchor. Invalid legacy intervals used a 12-hour fallback.
WITH legacy AS (
    SELECT id, backup_enabled,
        CASE WHEN backup_interval_minutes BETWEEN 1 AND 153722867
            THEN backup_interval_minutes ELSE 720 END AS minutes,
        COALESCE((SELECT MAX(created_at) FROM backups
            WHERE server_id=servers.id AND status='completed'), created_at) AS anchor
    FROM servers
)
INSERT INTO scheduled_tasks (
    id, server_id, name, action, cron, timezone, enabled, next_run_at, created_at, updated_at
)
SELECT 'automatic-backup-' || id, id, 'Automatic backup', 'backup',
    '@every ' || minutes || 'm', 'UTC', backup_enabled,
    CASE WHEN backup_enabled=1 THEN COALESCE(
        strftime('%Y-%m-%dT%H:%M:%SZ', anchor, '+' || minutes || ' minutes'),
        strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+' || minutes || ' minutes')
    ) ELSE '' END,
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM legacy;
