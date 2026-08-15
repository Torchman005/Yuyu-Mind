package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ToolCalls      string    `json:"tool_calls,omitempty"`
	ToolCallID     string    `json:"tool_call_id,omitempty"`
	SourceKind     string    `json:"source_kind,omitempty"`
	Emotion        string    `json:"emotion,omitempty"`
	Mood           string    `json:"mood,omitempty"`
	Energy         float64   `json:"energy,omitempty"`
	Gesture        string    `json:"gesture,omitempty"`
	Hand           string    `json:"hand,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type MessageRepo struct {
	db *sql.DB
}

func NewMessageRepo(db *sql.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

func (r *MessageRepo) Create(ctx context.Context, m *Message) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, gesture, hand, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Role, m.Content, nullableString(m.ToolCalls), nullableString(m.ToolCallID), nullableString(m.SourceKind),
		nullableString(m.Emotion), nullableString(m.Mood), m.Energy, nullableString(m.Gesture), nullableString(m.Hand), m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *MessageRepo) CreateBatch(ctx context.Context, msgs []*Message) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, gesture, hand, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		if _, err := stmt.ExecContext(ctx,
			m.ID, m.ConversationID, m.Role, m.Content, nullableString(m.ToolCalls), nullableString(m.ToolCallID), nullableString(m.SourceKind),
			nullableString(m.Emotion), nullableString(m.Mood), m.Energy, nullableString(m.Gesture), nullableString(m.Hand), m.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert message %s: %w", m.ID, err)
		}
	}

	return tx.Commit()
}

func (r *MessageRepo) ListByConversation(ctx context.Context, convID string) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, gesture, hand, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`, convID,
	)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var m Message
		var toolCalls, toolCallID, sourceKind, emotion, mood, gesture, hand sql.NullString
		var energy sql.NullFloat64
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &toolCalls, &toolCallID, &sourceKind, &emotion, &mood, &energy, &gesture, &hand, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.ToolCalls = toolCalls.String
		m.ToolCallID = toolCallID.String
		m.SourceKind = sourceKind.String
		m.Emotion = emotion.String
		m.Mood = mood.String
		m.Energy = energy.Float64
		m.Gesture = gesture.String
		m.Hand = hand.String
		messages = append(messages, &m)
	}
	return messages, rows.Err()
}

func (r *MessageRepo) DeleteByConversation(ctx context.Context, convID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id = ?`, convID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	return nil
}
