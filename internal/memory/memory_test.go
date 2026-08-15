package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/db"
)

func msgSeq(roles ...schema.RoleType) []*schema.Message {
	out := make([]*schema.Message, 0, len(roles))
	for i, role := range roles {
		out = append(out, &schema.Message{Role: role, Content: fmt.Sprintf("m%d", i)})
	}
	return out
}

func rolesOf(msgs []*schema.Message) []schema.RoleType {
	out := make([]schema.RoleType, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Role)
	}
	return out
}

func TestWindowTruncate(t *testing.T) {
	// 不限制：原样返回。
	w := NewWindow(0)
	in := msgSeq(schema.User, schema.Assistant, schema.User, schema.Assistant)
	if got := w.Truncate(in); len(got) != 4 {
		t.Fatalf("no-limit truncate changed length: %d", len(got))
	}

	// 保留最近 2 轮。
	w = NewWindow(2)
	in = msgSeq(schema.User, schema.Assistant, schema.User, schema.Assistant, schema.User, schema.Assistant)
	got := rolesOf(w.Truncate(in))
	want := []schema.RoleType{schema.User, schema.Assistant, schema.User, schema.Assistant}
	if len(got) != len(want) {
		t.Fatalf("expected %d roles, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("role %d = %s, want %s", i, got[i], want[i])
		}
	}

	// 系统消息应被保留。
	w = NewWindow(1)
	in = msgSeq(schema.System, schema.User, schema.Assistant, schema.User, schema.Assistant)
	got = rolesOf(w.Truncate(in))
	want = []schema.RoleType{schema.System, schema.User, schema.Assistant}
	if len(got) != len(want) {
		t.Fatalf("system preserve: expected %v, got %v", want, got)
	}
}

func TestRoleMappingRoundTrip(t *testing.T) {
	for _, role := range []schema.RoleType{schema.System, schema.User, schema.Assistant, schema.Tool} {
		if got := toSchemaRole(fromSchemaRole(role)); got != role {
			t.Fatalf("role round-trip %s -> %s", role, got)
		}
	}
}

func TestMessageRowRoundTrip(t *testing.T) {
	in := &schema.Message{
		Role:    schema.Assistant,
		Content: "hi",
		ToolCalls: []schema.ToolCall{
			{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"a.txt"}`}},
		},
	}
	row, err := toMessageRow(in, "conv-1")
	if err != nil {
		t.Fatalf("toMessageRow: %v", err)
	}
	if row.Role != "assistant" || row.ConversationID != "conv-1" || row.ToolCalls == "" {
		t.Fatalf("unexpected row: %+v", row)
	}

	back := toSchemaMessages([]*db.Message{row})
	if len(back) != 1 {
		t.Fatalf("expected 1 message back, got %d", len(back))
	}
	if back[0].Role != schema.Assistant || back[0].Content != "hi" {
		t.Fatalf("unexpected message: %+v", back[0])
	}
	if len(back[0].ToolCalls) != 1 || back[0].ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool calls lost: %+v", back[0].ToolCalls)
	}
}

func TestSQLiteStoreRoundTrip(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	store := NewSQLiteStore(database.Messages, 0)
	ctx := context.Background()

	now := time.Now()
	if err := database.Conversations.Create(ctx, &db.Conversation{
		ID:        "conv-1",
		Title:     "test",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	if err := store.AppendMessages(ctx, "conv-1", []*schema.Message{
		{Role: schema.User, Content: "你好"},
		{Role: schema.Assistant, Content: "你好！"},
	}); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	history, err := store.GetHistory(ctx, "conv-1")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[0].Role != schema.User || history[0].Content != "你好" {
		t.Fatalf("unexpected first message: %+v", history[0])
	}
	if history[1].Role != schema.Assistant || history[1].Content != "你好！" {
		t.Fatalf("unexpected second message: %+v", history[1])
	}

	// 不同会话隔离。
	empty, err := store.GetHistory(ctx, "conv-2")
	if err != nil {
		t.Fatalf("GetHistory conv-2: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty history for conv-2, got %d", len(empty))
	}
}
