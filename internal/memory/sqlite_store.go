package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

// SQLiteStore 使用 SQLite 实现会话记忆。
type SQLiteStore struct {
	msgRepo *db.MessageRepo
	window  *Window
}

// NewSQLiteStore 创建 SQLite 会话记忆存储。
func NewSQLiteStore(msgRepo *db.MessageRepo, maxTurns int) *SQLiteStore {
	return &SQLiteStore{
		msgRepo: msgRepo,
		window:  NewWindow(maxTurns),
	}
}

// GetHistory 返回 Eino schema.Message 格式的会话历史。
func (s *SQLiteStore) GetHistory(ctx context.Context, conversationID string) ([]*schema.Message, error) {
	rows, err := s.msgRepo.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	msgs := toSchemaMessages(rows)
	return s.window.Truncate(msgs), nil
}

// AppendMessage 追加单条消息到会话。
func (s *SQLiteStore) AppendMessage(ctx context.Context, conversationID string, msg *schema.Message) error {
	row, err := toMessageRow(msg, conversationID)
	if err != nil {
		return fmt.Errorf("convert message: %w", err)
	}
	return s.msgRepo.Create(ctx, row)
}

// AppendMessages 追加多条消息到会话。
func (s *SQLiteStore) AppendMessages(ctx context.Context, conversationID string, msgs []*schema.Message) error {
	rows := make([]*db.Message, 0, len(msgs))
	for _, msg := range msgs {
		row, err := toMessageRow(msg, conversationID)
		if err != nil {
			return fmt.Errorf("convert message: %w", err)
		}
		rows = append(rows, row)
	}
	return s.msgRepo.CreateBatch(ctx, rows)
}

// toSchemaMessages 将数据库记录转换为 Eino 消息。
func toSchemaMessages(rows []*db.Message) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(rows))
	for _, row := range rows {
		msg := &schema.Message{
			Role:    toSchemaRole(row.Role),
			Content: row.Content,
		}

		// 如果存在工具调用，则反序列化。
		if row.ToolCalls != "" {
			var toolCalls []schema.ToolCall
			if err := json.Unmarshal([]byte(row.ToolCalls), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}

		// 工具响应消息需要保留对应的工具调用 ID。
		if row.ToolCallID != "" {
			msg.ToolCallID = row.ToolCallID
		}

		msgs = append(msgs, msg)
	}
	return msgs
}

// toMessageRow 将 Eino 消息转换为数据库记录。
func toMessageRow(msg *schema.Message, conversationID string) (*db.Message, error) {
	row := &db.Message{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		Role:           fromSchemaRole(msg.Role),
		Content:        msg.Content,
		CreatedAt:      time.Now(),
	}

	// 如果存在工具调用，则序列化为 JSON。
	if len(msg.ToolCalls) > 0 {
		data, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("marshal tool calls: %w", err)
		}
		row.ToolCalls = string(data)
	}

	// 工具响应消息需要保存对应的工具调用 ID。
	if msg.ToolCallID != "" {
		row.ToolCallID = msg.ToolCallID
	}

	return row, nil
}

// toSchemaRole 将数据库角色字符串映射为 Eino 角色。
func toSchemaRole(role string) schema.RoleType {
	switch role {
	case "system":
		return schema.System
	case "user":
		return schema.User
	case "assistant":
		return schema.Assistant
	case "tool":
		return schema.Tool
	default:
		return schema.User
	}
}

// fromSchemaRole 将 Eino 角色映射为数据库角色字符串。
func fromSchemaRole(role schema.RoleType) string {
	switch role {
	case schema.System:
		return "system"
	case schema.User:
		return "user"
	case schema.Assistant:
		return "assistant"
	case schema.Tool:
		return "tool"
	default:
		return "user"
	}
}
