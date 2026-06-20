-- +migrate Up
CREATE TABLE IF NOT EXISTS token_usage (
    id                TEXT PRIMARY KEY,
    conversation_id   TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    mode              TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    model_calls       INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    status            TEXT NOT NULL,
    error             TEXT,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_token_usage_conversation_created_at
    ON token_usage(conversation_id, created_at);

CREATE INDEX IF NOT EXISTS idx_token_usage_provider_model_created_at
    ON token_usage(provider, model, created_at);

-- +migrate Down
DROP TABLE IF EXISTS token_usage;
