CREATE TABLE software_catalog (
    server_type TEXT NOT NULL,
    version TEXT NOT NULL,
    latest TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY (server_type, version)
);
