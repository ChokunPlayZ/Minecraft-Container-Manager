CREATE TABLE performance_samples (
    server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    recorded_at INTEGER NOT NULL,
    online INTEGER NOT NULL,
    cpu_percent REAL,
    memory_bytes REAL,
    players INTEGER,
    tps REAL,
    tick_ms REAL,
    PRIMARY KEY (server_id, recorded_at)
);
CREATE INDEX performance_samples_retention ON performance_samples(recorded_at);

CREATE TABLE spark_reports (
    server_id TEXT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    existing INTEGER NOT NULL DEFAULT 0
);
