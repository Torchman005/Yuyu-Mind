-- +migrate Up
-- 学习会话表：记录每次番茄钟学习时段
CREATE TABLE IF NOT EXISTS study_sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT 'default',
    start_time  DATETIME NOT NULL,
    end_time    DATETIME,
    mode        TEXT NOT NULL,  -- 'study' | 'break'
    duration_minutes INTEGER DEFAULT 0,
    focus_score REAL DEFAULT 1.0,  -- 专注度评分 0-1
    notes       TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 学习记录表：每日汇总统计
CREATE TABLE IF NOT EXISTS study_records (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL DEFAULT 'default',
    date          DATE NOT NULL,
    total_minutes INTEGER DEFAULT 0,
    topics        TEXT,  -- JSON 数组：今天学了哪些内容
    screenshots   INTEGER DEFAULT 0,  -- 今天截图次数
    cards_created INTEGER DEFAULT 0,  -- 今天创建的卡片数
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, date)
);

-- 记忆卡片表：用于复习
CREATE TABLE IF NOT EXISTS flashcards (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL DEFAULT 'default',
    question       TEXT NOT NULL,
    answer         TEXT NOT NULL,
    tags           TEXT,  -- JSON 数组
    source         TEXT,  -- 卡片来源（PDF、截图等）
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_review_at DATETIME,
    review_count   INTEGER DEFAULT 0,
    difficulty     INTEGER DEFAULT 0  -- 难度：0=新卡片，1-5=复习次数影响
);

-- 学习目标表
CREATE TABLE IF NOT EXISTS study_goals (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT 'default',
    title       TEXT NOT NULL,
    description TEXT,
    target_minutes INTEGER,  -- 目标学习时长（分钟）
    deadline    DATE,
    status      TEXT NOT NULL DEFAULT 'active',  -- 'active' | 'completed' | 'cancelled'
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_study_sessions_user_time ON study_sessions(user_id, start_time);
CREATE INDEX IF NOT EXISTS idx_study_records_user_date ON study_records(user_id, date);
CREATE INDEX IF NOT EXISTS idx_flashcards_user_review ON flashcards(user_id, next_review_at);
CREATE INDEX IF NOT EXISTS idx_study_goals_user_status ON study_goals(user_id, status);

-- +migrate Down
DROP INDEX IF EXISTS idx_study_goals_user_status;
DROP INDEX IF EXISTS idx_flashcards_user_review;
DROP INDEX IF EXISTS idx_study_records_user_date;
DROP INDEX IF EXISTS idx_study_sessions_user_time;
DROP TABLE IF EXISTS study_goals;
DROP TABLE IF EXISTS flashcards;
DROP TABLE IF EXISTS study_records;
DROP TABLE IF EXISTS study_sessions;
