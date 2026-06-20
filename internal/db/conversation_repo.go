package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Conversation 表示 conversations 表中的一条会话记录。
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationRepo 负责会话的增删改查。
type ConversationRepo struct {
	db *sql.DB
}

// NewConversationRepo 创建会话仓储。
func NewConversationRepo(db *sql.DB) *ConversationRepo {
	return &ConversationRepo{db: db}
}

// Create 插入新会话。
func (r *ConversationRepo) Create(ctx context.Context, c *Conversation) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, provider, model, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Provider, c.Model, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}
	return nil
}

// GetByID 按 ID 查询会话。
func (r *ConversationRepo) GetByID(ctx context.Context, id string) (*Conversation, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, title, provider, model, created_at, updated_at
		 FROM conversations WHERE id = ?`, id,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return &c, nil
}

// List 返回所有会话，按更新时间倒序排列。
func (r *ConversationRepo) List(ctx context.Context) ([]*Conversation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, provider, model, created_at, updated_at
		 FROM conversations ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []*Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, &c)
	}
	return conversations, rows.Err()
}

// Update 更新会话标题、供应商、模型和更新时间。
func (r *ConversationRepo) Update(ctx context.Context, c *Conversation) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE conversations SET title = ?, provider = ?, model = ?, updated_at = ?
		 WHERE id = ?`,
		c.Title, c.Provider, c.Model, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	return nil
}

// Delete 删除会话，并通过外键级联删除消息和用量记录。
func (r *ConversationRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}
