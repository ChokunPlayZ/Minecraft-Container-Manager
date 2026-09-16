CREATE TABLE servers_new (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL,
    server_type             TEXT NOT NULL,
    version                 TEXT NOT NULL,
    build                   TEXT,
    ram_mb                  INTEGER NOT NULL,
    host_port               INTEGER NOT NULL DEFAULT 0,
    container_id            TEXT,
    state                   TEXT NOT NULL DEFAULT 'stopped',
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL,
    backup_enabled          INTEGER NOT NULL DEFAULT 1,
    backup_interval_minutes INTEGER NOT NULL DEFAULT 720,
    last_activity           TEXT,
    idle_timeout_minutes    INTEGER,
    cpu_limit               REAL NOT NULL DEFAULT 0,
    memory_limit_mb         INTEGER NOT NULL DEFAULT 0,
    extra_ports             TEXT NOT NULL DEFAULT '[]',
    spin_down_disabled      INTEGER NOT NULL DEFAULT 0,
    spin_down_enabled       INTEGER NOT NULL DEFAULT 0,
    started_at              TEXT,
    container_config        TEXT,
    java_version            INTEGER NOT NULL DEFAULT 21
);

INSERT INTO servers_new (
    id, name, server_type, version, build, ram_mb, host_port, container_id,
    state, created_at, updated_at, backup_enabled, backup_interval_minutes,
    last_activity, idle_timeout_minutes, cpu_limit, memory_limit_mb, extra_ports,
    spin_down_disabled, spin_down_enabled, started_at, container_config, java_version
)
SELECT
    id, name, server_type, version, build, ram_mb, host_port, container_id,
    state, created_at, updated_at, backup_enabled, backup_interval_minutes,
    last_activity, idle_timeout_minutes, cpu_limit, memory_limit_mb, extra_ports,
    spin_down_disabled, spin_down_enabled, started_at, container_config, java_version
FROM servers;

DROP TABLE servers;
ALTER TABLE servers_new RENAME TO servers;

CREATE UNIQUE INDEX idx_servers_host_port_positive ON servers(host_port) WHERE host_port > 0;
