-- +migrate Up
CREATE TABLE IF NOT EXISTS user_memories (
    id                TEXT PRIMARY KEY,
    scope             TEXT NOT NULL,
    kind              TEXT NOT NULL,
    key               TEXT NOT NULL,
    value_json        TEXT NOT NULL,
    text              TEXT NOT NULL,
    confidence        REAL NOT NULL DEFAULT 1.0,
    source            TEXT NOT NULL,
    source_message_id TEXT,
    status            TEXT NOT NULL DEFAULT 'active',
    use_count         INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at      DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_memories_scope_kind_key
    ON user_memories(scope, kind, key);

CREATE INDEX IF NOT EXISTS idx_user_memories_lookup
    ON user_memories(scope, kind, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS memory_events (
    id         TEXT PRIMARY KEY,
    memory_id  TEXT REFERENCES user_memories(id) ON DELETE SET NULL,
    type       TEXT NOT NULL,
    message    TEXT NOT NULL,
    payload    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_events_memory_created_at
    ON memory_events(memory_id, created_at ASC);

CREATE TABLE IF NOT EXISTS memory_candidates (
    id             TEXT PRIMARY KEY,
    scope          TEXT NOT NULL,
    kind           TEXT NOT NULL,
    key            TEXT NOT NULL,
    value_json     TEXT NOT NULL,
    text           TEXT NOT NULL,
    evidence_count INTEGER NOT NULL DEFAULT 1,
    confidence     REAL NOT NULL DEFAULT 0.5,
    status         TEXT NOT NULL DEFAULT 'pending',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_candidates_status
    ON memory_candidates(status, confidence DESC, updated_at DESC);

CREATE TABLE IF NOT EXISTS conversation_summaries (
    conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    summary         TEXT NOT NULL,
    token_estimate  INTEGER NOT NULL DEFAULT 0,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS task_context_snapshots (
    id         TEXT PRIMARY KEY,
    task_id    TEXT REFERENCES agent_tasks(id) ON DELETE CASCADE,
    scope      TEXT NOT NULL,
    context_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_context_snapshots_task_created_at
    ON task_context_snapshots(task_id, created_at DESC);

-- +migrate Down
DROP TABLE IF EXISTS task_context_snapshots;
DROP TABLE IF EXISTS conversation_summaries;
DROP TABLE IF EXISTS memory_candidates;
DROP TABLE IF EXISTS memory_events;
DROP TABLE IF EXISTS user_memories;
