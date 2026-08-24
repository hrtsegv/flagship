CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    key         TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE environments (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    key            TEXT NOT NULL,
    name           TEXT NOT NULL,
    config_version INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    UNIQUE (project_id, key)
);

CREATE TABLE flags (
    id             TEXT PRIMARY KEY,
    environment_id TEXT NOT NULL REFERENCES environments(id),
    key            TEXT NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    version        INT NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    UNIQUE (environment_id, key)
);

CREATE TABLE config_events (
    environment_id TEXT NOT NULL REFERENCES environments(id),
    version        INT NOT NULL,
    type           TEXT NOT NULL,
    flag_key       TEXT NOT NULL,
    enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (environment_id, version)
);
