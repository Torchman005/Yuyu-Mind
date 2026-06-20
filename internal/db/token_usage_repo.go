package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TokenUsageRecord 表示一次用户请求产生的模型 token 消耗。
type TokenUsageRecord struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversation_id"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Mode             string    `json:"mode"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	ModelCalls       int       `json:"model_calls"`
	DurationMS       int64     `json:"duration_ms"`
	Status           string    `json:"status"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// TokenUsageSummary 表示 token 消耗的聚合结果。
type TokenUsageSummary struct {
	ConversationID    string `json:"conversation_id,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	RequestCount      int    `json:"request_count"`
	FailedCount       int    `json:"failed_count"`
	ModelCalls        int    `json:"model_calls"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	TotalTokens       int    `json:"total_tokens"`
	TotalDurationMS   int64  `json:"total_duration_ms"`
	AverageDurationMS int64  `json:"average_duration_ms"`
}

// TokenUsageRepo 负责 token 用量的写入和查询。
type TokenUsageRepo struct {
	db *sql.DB
}

// NewTokenUsageRepo 创建 token 用量仓储。
func NewTokenUsageRepo(db *sql.DB) *TokenUsageRepo {
	return &TokenUsageRepo{db: db}
}

// Create 写入一条 token 用量记录。
func (r *TokenUsageRepo) Create(ctx context.Context, u *TokenUsageRecord) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO token_usage (
			id, conversation_id, provider, model, mode,
			prompt_tokens, completion_tokens, total_tokens, model_calls,
			duration_ms, status, error, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.ConversationID, u.Provider, u.Model, u.Mode,
		u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.ModelCalls,
		u.DurationMS, u.Status, nullableString(u.Error), u.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert token usage: %w", err)
	}
	return nil
}

// ListByConversation 返回指定会话的 token 用量明细，按时间倒序排列。
func (r *TokenUsageRepo) ListByConversation(ctx context.Context, conversationID string) ([]*TokenUsageRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, provider, model, mode,
			prompt_tokens, completion_tokens, total_tokens, model_calls,
			duration_ms, status, error, created_at
		 FROM token_usage
		 WHERE conversation_id = ?
		 ORDER BY created_at DESC`, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list token usage: %w", err)
	}
	defer rows.Close()

	var records []*TokenUsageRecord
	for rows.Next() {
		record, err := scanTokenUsage(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// Summary 返回所有会话的 token 用量总览。
func (r *TokenUsageRepo) Summary(ctx context.Context) (*TokenUsageSummary, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status != 'success' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(model_calls), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(duration_ms), 0)
		 FROM token_usage`,
	)
	return scanTokenUsageSummary(row)
}

// SummaryByConversation 返回指定会话的 token 用量总览。
func (r *TokenUsageRepo) SummaryByConversation(ctx context.Context, conversationID string) (*TokenUsageSummary, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status != 'success' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(model_calls), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(duration_ms), 0)
		 FROM token_usage
		 WHERE conversation_id = ?`, conversationID,
	)
	summary, err := scanTokenUsageSummary(row)
	if err != nil {
		return nil, err
	}
	summary.ConversationID = conversationID
	return summary, nil
}

// SummaryByProviderModel 按供应商和模型维度汇总 token 用量。
func (r *TokenUsageRepo) SummaryByProviderModel(ctx context.Context) ([]*TokenUsageSummary, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT provider, model,
			COUNT(*),
			COALESCE(SUM(CASE WHEN status != 'success' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(model_calls), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(duration_ms), 0)
		 FROM token_usage
		 GROUP BY provider, model
		 ORDER BY total_tokens DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("summary token usage by provider model: %w", err)
	}
	defer rows.Close()

	var summaries []*TokenUsageSummary
	for rows.Next() {
		var s TokenUsageSummary
		if err := rows.Scan(
			&s.Provider, &s.Model, &s.RequestCount, &s.FailedCount, &s.ModelCalls,
			&s.PromptTokens, &s.CompletionTokens, &s.TotalTokens, &s.TotalDurationMS,
		); err != nil {
			return nil, fmt.Errorf("scan token usage summary: %w", err)
		}
		if s.RequestCount > 0 {
			s.AverageDurationMS = s.TotalDurationMS / int64(s.RequestCount)
		}
		summaries = append(summaries, &s)
	}
	return summaries, rows.Err()
}

type tokenUsageScanner interface {
	Scan(dest ...any) error
}

func scanTokenUsage(scanner tokenUsageScanner) (*TokenUsageRecord, error) {
	var u TokenUsageRecord
	var errText sql.NullString
	if err := scanner.Scan(
		&u.ID, &u.ConversationID, &u.Provider, &u.Model, &u.Mode,
		&u.PromptTokens, &u.CompletionTokens, &u.TotalTokens, &u.ModelCalls,
		&u.DurationMS, &u.Status, &errText, &u.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan token usage: %w", err)
	}
	u.Error = errText.String
	return &u, nil
}

func scanTokenUsageSummary(scanner tokenUsageScanner) (*TokenUsageSummary, error) {
	var s TokenUsageSummary
	if err := scanner.Scan(
		&s.RequestCount, &s.FailedCount, &s.ModelCalls,
		&s.PromptTokens, &s.CompletionTokens, &s.TotalTokens, &s.TotalDurationMS,
	); err != nil {
		return nil, fmt.Errorf("scan token usage summary: %w", err)
	}
	if s.RequestCount > 0 {
		s.AverageDurationMS = s.TotalDurationMS / int64(s.RequestCount)
	}
	return &s, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
