package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// newInterruptionTestDB 建一个带会话的最小库（打断逻辑需要 conversation 外键）。
func newInterruptionTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "interruption.db")
	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { database.Close() })

	conv := &Conversation{
		ID:        "c1",
		Title:     "Test",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.Conversations.Create(context.Background(), conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return database, conv.ID
}

func createMessage(t *testing.T, database *DB, id, role, content string) {
	t.Helper()
	// 刻意使用同一时间戳：验证「同批写入、时间相同」时 tail 仍能按插入序判定。
	if err := database.Messages.Create(context.Background(), &Message{
		ID:             id,
		ConversationID: "c1",
		Role:           role,
		Content:        content,
		CreatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("create message %s: %v", id, err)
	}
}

// TestTailAssistantMessagesOrdersByInsertion 验证 tail 查询在 created_at 相同的情况下
// 仍按插入顺序返回（SQLite rowid），并且只取末尾连续的一段助手发言。
func TestTailAssistantMessagesOrdersByInsertion(t *testing.T) {
	database, convID := newInterruptionTestDB(t)
	ctx := context.Background()

	createMessage(t, database, "u1", "user", "在吗")
	createMessage(t, database, "a1", "assistant", "第一句")
	createMessage(t, database, "a2", "assistant", "第二句")
	createMessage(t, database, "a3", "assistant", "第三句")

	tail, err := database.Messages.TailAssistantMessages(ctx, convID)
	if err != nil {
		t.Fatalf("TailAssistantMessages: %v", err)
	}
	if len(tail) != 3 {
		t.Fatalf("应取到末尾 3 条助手消息，实际 %d", len(tail))
	}
	wantOrder := []string{"第一句", "第二句", "第三句"}
	for i, m := range tail {
		if m.Content != wantOrder[i] {
			t.Fatalf("顺序错误：第 %d 条 = %q，期望 %q", i, m.Content, wantOrder[i])
		}
	}
}

// TestTailAssistantMessagesStopsAtNonAssistant 验证遇到非 assistant 消息即停止，
// 不会把上一轮助手发言也算进来。
func TestTailAssistantMessagesStopsAtNonAssistant(t *testing.T) {
	database, convID := newInterruptionTestDB(t)
	ctx := context.Background()

	createMessage(t, database, "a_old", "assistant", "上一轮的旧回复")
	createMessage(t, database, "u1", "user", "新问题")
	createMessage(t, database, "a1", "assistant", "本轮第一句")
	createMessage(t, database, "a2", "assistant", "本轮第二句")

	tail, err := database.Messages.TailAssistantMessages(ctx, convID)
	if err != nil {
		t.Fatalf("TailAssistantMessages: %v", err)
	}
	if len(tail) != 2 {
		t.Fatalf("应只取本轮 2 条助手消息，实际 %d（%+v）", len(tail), tail)
	}
	if tail[0].Content != "本轮第一句" {
		t.Fatalf("不应包含上一轮消息，实际首条 = %q", tail[0].Content)
	}
}

// TestDeleteMessages 验证按 id 批量删除；空列表应为无害 no-op。
func TestDeleteMessages(t *testing.T) {
	database, convID := newInterruptionTestDB(t)
	ctx := context.Background()

	createMessage(t, database, "a1", "assistant", "保留")
	createMessage(t, database, "a2", "assistant", "删除")

	if err := database.Messages.DeleteMessages(ctx, nil); err != nil {
		t.Fatalf("空列表删除应无错，实际 %v", err)
	}
	if err := database.Messages.DeleteMessages(ctx, []string{"a2"}); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}

	remaining, err := database.Messages.ListByConversation(ctx, convID)
	if err != nil {
		t.Fatalf("ListByConversation: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "a1" {
		t.Fatalf("应只剩 a1，实际 %+v", remaining)
	}
}
