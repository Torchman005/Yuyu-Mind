package memory

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// Store 是会话记忆接口。
// 它负责在 Eino 消息格式和持久化层之间做转换。
type Store interface {
	// GetHistory 返回 Eino 消息格式的会话历史。
	GetHistory(ctx context.Context, conversationID string) ([]*schema.Message, error)

	// AppendMessage 追加单条消息到会话历史。
	AppendMessage(ctx context.Context, conversationID string, msg *schema.Message) error

	// AppendMessages 追加多条消息到会话历史。
	AppendMessages(ctx context.Context, conversationID string, msgs []*schema.Message) error
}
