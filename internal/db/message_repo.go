package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Message 表示 messages 表中的一条聊天消息。
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"` // user、assistant、system 或 tool
	Content        string    `json:"content"`
	ToolCalls      string    `json:"tool_calls,omitempty"` // JSON 编码后的工具调用
	ToolCallID     string    `json:"tool_call_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// MessageRepo 负责消息的增删改查。
type MessageRepo struct {
	db *sql.DB
}

// NewMessageRepo 创建消息仓储。
func NewMessageRepo(db *sql.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// Create 插入单条消息。
func (r *MessageRepo) Create(ctx context.Context, m *Message) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Role, m.Content, m.ToolCalls, m.ToolCallID, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// CreateBatch 在事务中批量插入消息。
func (r *MessageRepo) CreateBatch(ctx context.Context, msgs []*Message) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		if _, err := stmt.ExecContext(ctx,
			m.ID, m.ConversationID, m.Role, m.Content, m.ToolCalls, m.ToolCallID, m.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert message %s: %w", m.ID, err)
		}
	}

	return tx.Commit()
}

// ListByConversation 返回指定会话的所有消息，按创建时间升序排列。
func (r *MessageRepo) ListByConversation(ctx context.Context, convID string) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_call_id, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var m Message
		var toolCalls, toolCallID sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &toolCalls, &toolCallID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.ToolCalls = toolCalls.String
		m.ToolCallID = toolCallID.String
		messages = append(messages, &m)
	}
	return messages, rows.Err()
}

// DeleteByConversation 删除指定会话的所有消息。
func (r *MessageRepo) DeleteByConversation(ctx context.Context, convID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id = ?`, convID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}
