-- +migrate Up
CREATE TABLE IF NOT EXISTS agent_tasks (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT,
    parent_task_id  TEXT REFERENCES agent_tasks(id) ON DELETE SET NULL,
    title           TEXT NOT NULL,
    goal            TEXT NOT NULL,
    status          TEXT NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 0,
    spec_json       TEXT NOT NULL,
    result_json     TEXT,
    error           TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME,
    completed_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_status_priority
    ON agent_tasks(status, priority DESC, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_conversation
    ON agent_tasks(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_task_events (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    level      TEXT NOT NULL DEFAULT 'info',
    message    TEXT NOT NULL,
    payload    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_task_events_task_created_at
    ON agent_task_events(task_id, created_at ASC);

CREATE TABLE IF NOT EXISTS agent_task_controls (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    payload    TEXT,
    status     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_agent_task_controls_task_status
    ON agent_task_controls(task_id, status, created_at ASC);

CREATE TABLE IF NOT EXISTS agent_operation_logs (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    target     TEXT NOT NULL,
    summary    TEXT NOT NULL,
    status     TEXT NOT NULL,
    payload    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_operation_logs_task_created_at
    ON agent_operation_logs(task_id, created_at ASC);

-- +migrate Down
DROP TABLE IF EXISTS agent_operation_logs;
DROP TABLE IF EXISTS agent_task_controls;
DROP TABLE IF EXISTS agent_task_events;
DROP TABLE IF EXISTS agent_tasks;
