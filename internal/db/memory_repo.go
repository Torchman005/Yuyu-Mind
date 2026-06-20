package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type UserMemory struct {
	ID              string     `json:"id"`
	Scope           string     `json:"scope"`
	Kind            string     `json:"kind"`
	Key             string     `json:"key"`
	ValueJSON       string     `json:"value_json"`
	Text            string     `json:"text"`
	Confidence      float64    `json:"confidence"`
	Source          string     `json:"source"`
	SourceMessageID string     `json:"source_message_id,omitempty"`
	Status          string     `json:"status"`
	UseCount        int        `json:"use_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

type MemoryEvent struct {
	ID        string    `json:"id"`
	MemoryID  string    `json:"memory_id,omitempty"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type MemoryCandidate struct {
	ID            string    `json:"id"`
	Scope         string    `json:"scope"`
	Kind          string    `json:"kind"`
	Key           string    `json:"key"`
	ValueJSON     string    `json:"value_json"`
	Text          string    `json:"text"`
	EvidenceCount int       `json:"evidence_count"`
	Confidence    float64   `json:"confidence"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ConversationSummary struct {
	ConversationID string    `json:"conversation_id"`
	Summary        string    `json:"summary"`
	TokenEstimate  int       `json:"token_estimate"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TaskContextSnapshot struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id,omitempty"`
	Scope       string    `json:"scope"`
	ContextJSON string    `json:"context_json"`
	CreatedAt   time.Time `json:"created_at"`
}

type MemoryRepo struct {
	db *sql.DB
}

func NewMemoryRepo(db *sql.DB) *MemoryRepo {
	return &MemoryRepo{db: db}
}

func (r *MemoryRepo) UpsertMemory(ctx context.Context, m *UserMemory) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_memories (
			id, scope, kind, key, value_json, text, confidence, source, source_message_id,
			status, use_count, created_at, updated_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, kind, key) DO UPDATE SET
			value_json = excluded.value_json,
			text = excluded.text,
			confidence = excluded.confidence,
			source = excluded.source,
			source_message_id = excluded.source_message_id,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		m.ID, m.Scope, m.Kind, m.Key, m.ValueJSON, m.Text, m.Confidence, m.Source,
		nullableString(m.SourceMessageID), m.Status, m.UseCount, m.CreatedAt, m.UpdatedAt, m.LastUsedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert memory: %w", err)
	}
	return nil
}

func (r *MemoryRepo) GetMemoryByKey(ctx context.Context, scope, kind, key string) (*UserMemory, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, scope, kind, key, value_json, text, confidence, source, source_message_id,
			status, use_count, created_at, updated_at, last_used_at
		 FROM user_memories WHERE scope = ? AND kind = ? AND key = ?`, scope, kind, key,
	)
	m, err := scanUserMemory(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *MemoryRepo) SearchMemories(ctx context.Context, scope, kind, query string, limit int) ([]*UserMemory, error) {
	if limit <= 0 {
		limit = 50
	}
	like := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, scope, kind, key, value_json, text, confidence, source, source_message_id,
			status, use_count, created_at, updated_at, last_used_at
		 FROM user_memories
		 WHERE status = 'active'
		   AND (? = '' OR scope = ?)
		   AND (? = '' OR kind = ?)
		   AND (? = '' OR key LIKE ? OR text LIKE ?)
		 ORDER BY confidence DESC, updated_at DESC
		 LIMIT ?`,
		scope, scope, kind, kind, query, like, like, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()
	return scanUserMemories(rows)
}

func (r *MemoryRepo) MarkMemoryUsed(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_memories SET use_count = use_count + 1, last_used_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("mark memory used: %w", err)
	}
	return nil
}

func (r *MemoryRepo) ArchiveMemory(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_memories SET status = 'archived', updated_at = ? WHERE id = ?`, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("archive memory: %w", err)
	}
	return nil
}

func (r *MemoryRepo) AddEvent(ctx context.Context, e *MemoryEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO memory_events (id, memory_id, type, message, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.ID, nullableString(e.MemoryID), e.Type, e.Message, nullableString(e.Payload), e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert memory event: %w", err)
	}
	return nil
}

func (r *MemoryRepo) AddCandidate(ctx context.Context, c *MemoryCandidate) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO memory_candidates (
			id, scope, kind, key, value_json, text, evidence_count, confidence, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Scope, c.Kind, c.Key, c.ValueJSON, c.Text, c.EvidenceCount, c.Confidence,
		c.Status, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert memory candidate: %w", err)
	}
	return nil
}

func (r *MemoryRepo) GetCandidate(ctx context.Context, id string) (*MemoryCandidate, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, scope, kind, key, value_json, text, evidence_count, confidence, status, created_at, updated_at
		 FROM memory_candidates WHERE id = ?`, id,
	)
	c, err := scanMemoryCandidate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *MemoryRepo) ListCandidates(ctx context.Context, status string, limit int) ([]*MemoryCandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, scope, kind, key, value_json, text, evidence_count, confidence, status, created_at, updated_at
		 FROM memory_candidates
		 WHERE (? = '' OR status = ?)
		 ORDER BY confidence DESC, updated_at DESC
		 LIMIT ?`, status, status, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memory candidates: %w", err)
	}
	defer rows.Close()

	var candidates []*MemoryCandidate
	for rows.Next() {
		c, err := scanMemoryCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func (r *MemoryRepo) UpdateCandidateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE memory_candidates SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update memory candidate status: %w", err)
	}
	return nil
}

func (r *MemoryRepo) UpsertConversationSummary(ctx context.Context, summary *ConversationSummary) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO conversation_summaries (conversation_id, summary, token_estimate, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
			summary = excluded.summary,
			token_estimate = excluded.token_estimate,
			updated_at = excluded.updated_at`,
		summary.ConversationID, summary.Summary, summary.TokenEstimate, summary.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert conversation summary: %w", err)
	}
	return nil
}

func (r *MemoryRepo) GetConversationSummary(ctx context.Context, conversationID string) (*ConversationSummary, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT conversation_id, summary, token_estimate, updated_at
		 FROM conversation_summaries WHERE conversation_id = ?`, conversationID,
	)
	var s ConversationSummary
	if err := row.Scan(&s.ConversationID, &s.Summary, &s.TokenEstimate, &s.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan conversation summary: %w", err)
	}
	return &s, nil
}

func (r *MemoryRepo) CreateTaskContextSnapshot(ctx context.Context, snapshot *TaskContextSnapshot) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO task_context_snapshots (id, task_id, scope, context_json, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		snapshot.ID, nullableString(snapshot.TaskID), snapshot.Scope, snapshot.ContextJSON, snapshot.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task context snapshot: %w", err)
	}
	return nil
}

func scanUserMemories(rows *sql.Rows) ([]*UserMemory, error) {
	var memories []*UserMemory
	for rows.Next() {
		m, err := scanUserMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func scanUserMemory(scanner interface{ Scan(dest ...any) error }) (*UserMemory, error) {
	var m UserMemory
	var sourceMessageID sql.NullString
	var lastUsedAt sql.NullTime
	if err := scanner.Scan(
		&m.ID, &m.Scope, &m.Kind, &m.Key, &m.ValueJSON, &m.Text, &m.Confidence,
		&m.Source, &sourceMessageID, &m.Status, &m.UseCount, &m.CreatedAt, &m.UpdatedAt, &lastUsedAt,
	); err != nil {
		return nil, err
	}
	m.SourceMessageID = sourceMessageID.String
	if lastUsedAt.Valid {
		m.LastUsedAt = &lastUsedAt.Time
	}
	return &m, nil
}

func scanMemoryCandidate(scanner interface{ Scan(dest ...any) error }) (*MemoryCandidate, error) {
	var c MemoryCandidate
	if err := scanner.Scan(
		&c.ID, &c.Scope, &c.Kind, &c.Key, &c.ValueJSON, &c.Text, &c.EvidenceCount,
		&c.Confidence, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan memory candidate: %w", err)
	}
	return &c, nil
}
