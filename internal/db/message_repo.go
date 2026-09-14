package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	Valence        float64   `json:"valence,omitempty"`
	Dominance      float64   `json:"dominance,omitempty"`
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
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, valence, dominance, gesture, hand, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Role, m.Content, nullableString(m.ToolCalls), nullableString(m.ToolCallID), nullableString(m.SourceKind),
		nullableString(m.Emotion), nullableString(m.Mood), m.Energy, m.Valence, m.Dominance, nullableString(m.Gesture), nullableString(m.Hand), m.CreatedAt,
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
		`INSERT INTO messages (id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, valence, dominance, gesture, hand, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		if _, err := stmt.ExecContext(ctx,
			m.ID, m.ConversationID, m.Role, m.Content, nullableString(m.ToolCalls), nullableString(m.ToolCallID), nullableString(m.SourceKind),
			nullableString(m.Emotion), nullableString(m.Mood), m.Energy, m.Valence, m.Dominance, nullableString(m.Gesture), nullableString(m.Hand), m.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert message %s: %w", m.ID, err)
		}
	}

	return tx.Commit()
}

func (r *MessageRepo) ListByConversation(ctx context.Context, convID string) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_call_id, source_kind, emotion, mood, energy, valence, dominance, gesture, hand, created_at
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
		var energy, valence, dominance sql.NullFloat64
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &toolCalls, &toolCallID, &sourceKind, &emotion, &mood, &energy, &valence, &dominance, &gesture, &hand, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.ToolCalls = toolCalls.String
		m.ToolCallID = toolCallID.String
		m.SourceKind = sourceKind.String
		m.Emotion = emotion.String
		m.Mood = mood.String
		m.Energy = energy.Float64
		m.Valence = valence.Float64
		m.Dominance = dominance.Float64
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

// DeleteMessages 按 id 批量删除消息（用于「打断后丢弃未播出的内容」）。
// 传入空列表时直接返回，避免生成空的 IN 子句。
func (r *MessageRepo) DeleteMessages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := `DELETE FROM messages WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete messages by id: %w", err)
	}
	return nil
}

// TailAssistantMessages 返回会话末尾连续的 assistant 消息（按插入顺序，最早在前）。
//
// 用途：打断（barge-in）时判断「哪几句其实还没播出」。用 SQLite 的隐式 rowid 排序，
// 因为同一轮回复的多条消息 created_at 相同，仅靠时间无法确定先后（见 REALISM-ANALYSIS P1-7）。
// 从末尾往前扫，遇到第一条非 assistant 消息即停止——只处理"最后一段连续的助手发言"。
func (r *MessageRepo) TailAssistantMessages(ctx context.Context, convID string) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, content FROM messages WHERE conversation_id = ? ORDER BY rowid DESC`, convID)
	if err != nil {
		return nil, fmt.Errorf("list tail assistant messages: %w", err)
	}
	defer rows.Close()

	newestFirst := make([]*Message, 0, 4)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			return nil, fmt.Errorf("scan tail message: %w", err)
		}
		if m.Role != "assistant" {
			break // 只取末尾连续的一段助手发言
		}
		newestFirst = append(newestFirst, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 反转为插入顺序（最早在前），便于调用方按「前 N 句已播出」截断。
	ordered := make([]*Message, 0, len(newestFirst))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		ordered = append(ordered, newestFirst[i])
	}
	return ordered, nil
}
