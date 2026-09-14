package chat

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yuyu-mind/backend/internal/config"
	"github.com/yuyu-mind/backend/internal/db"
)

// newInterruptionService 构造一个只带 db 的最小 Service，用于验证打断截断语义。
func newInterruptionService(t *testing.T) (*Service, string) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "chat-interruption.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	conv := &db.Conversation{
		ID:        "c-interrupt",
		Title:     "T",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.Conversations.Create(context.Background(), conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return &Service{cfg: config.DefaultConfig(), db: database}, conv.ID
}

func addAssistant(t *testing.T, s *Service, id, content string) {
	t.Helper()
	if err := s.db.Messages.Create(context.Background(), &db.Message{
		ID:             id,
		ConversationID: "c-interrupt",
		Role:           "assistant",
		Content:        content,
		SourceKind:     "guided_reply",
		CreatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
}

// TestRecordInterruptionKeepsOnlySpoken 是 P1-7 的核心断言：
// 打断后历史里只应留下**已播出**的句子，未播出的被丢弃，并追加一条打断标记。
func TestRecordInterruptionKeepsOnlySpoken(t *testing.T) {
	s, convID := newInterruptionService(t)
	ctx := context.Background()

	addAssistant(t, s, "a1", "第一句（已播出）")
	addAssistant(t, s, "a2", "第二句（已播出）")
	addAssistant(t, s, "a3", "第三句（未播出）")
	addAssistant(t, s, "a4", "第四句（未播出）")

	if err := s.RecordInterruption(ctx, convID, 2); err != nil {
		t.Fatalf("RecordInterruption: %v", err)
	}

	msgs, err := s.db.Messages.ListByConversation(ctx, convID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 期望：两句已播出 + 一条打断标记。
	if len(msgs) != 3 {
		t.Fatalf("应剩 3 条（2 已播出 + 1 标记），实际 %d：%+v", len(msgs), contents(msgs))
	}
	if msgs[0].Content != "第一句（已播出）" || msgs[1].Content != "第二句（已播出）" {
		t.Fatalf("已播出的句子应被保留，实际 %v", contents(msgs))
	}
	for _, m := range msgs {
		if m.Content == "第三句（未播出）" || m.Content == "第四句（未播出）" {
			t.Fatalf("未播出的句子必须被丢弃，实际仍存在：%v", contents(msgs))
		}
	}
	marker := msgs[len(msgs)-1]
	if marker.Content != interruptionMarker {
		t.Fatalf("最后一条应为打断标记 %q，实际 %q", interruptionMarker, marker.Content)
	}
	if marker.SourceKind != "interruption" {
		t.Fatalf("标记的 source_kind 应为 interruption，实际 %q", marker.SourceKind)
	}
}

// TestRecordInterruptionWhenNothingPlayed 验证「一句都没播出」时清空本轮全部助手消息。
func TestRecordInterruptionWhenNothingPlayed(t *testing.T) {
	s, convID := newInterruptionService(t)
	ctx := context.Background()

	addAssistant(t, s, "a1", "还没播就被打断")
	addAssistant(t, s, "a2", "同样没播")

	if err := s.RecordInterruption(ctx, convID, 0); err != nil {
		t.Fatalf("RecordInterruption: %v", err)
	}
	msgs, err := s.db.Messages.ListByConversation(ctx, convID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != interruptionMarker {
		t.Fatalf("应只剩打断标记，实际 %v", contents(msgs))
	}
}

// TestRecordInterruptionIgnoresOverflowCount 验证 playedCount 超过实际句数时不会误删、
// 也不会重复插入标记之外的副作用（幂等保护：调用方通常已确保播放未完成）。
func TestRecordInterruptionIgnoresOverflowCount(t *testing.T) {
	s, convID := newInterruptionService(t)
	ctx := context.Background()

	addAssistant(t, s, "a1", "一句")

	if err := s.RecordInterruption(ctx, convID, 99); err != nil {
		t.Fatalf("RecordInterruption: %v", err)
	}
	msgs, err := s.db.Messages.ListByConversation(ctx, convID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("超出计数不应删除任何句，实际 %v", contents(msgs))
	}
	if msgs[0].Content != "一句" {
		t.Fatalf("原句应保留，实际 %v", contents(msgs))
	}
	if msgs[1].Content != interruptionMarker {
		t.Fatalf("仍应追加标记，实际 %v", contents(msgs))
	}
}

// TestRecordInterruptionNegativeCountClampsToZero 负值按 0 处理（全部未播出）。
func TestRecordInterruptionNegativeCountClampsToZero(t *testing.T) {
	s, convID := newInterruptionService(t)
	ctx := context.Background()

	addAssistant(t, s, "a1", "未播出")

	if err := s.RecordInterruption(ctx, convID, -3); err != nil {
		t.Fatalf("RecordInterruption: %v", err)
	}
	msgs, _ := s.db.Messages.ListByConversation(ctx, convID)
	if len(msgs) != 1 || msgs[0].Content != interruptionMarker {
		t.Fatalf("负值应按 0 处理（全部丢弃），实际 %v", contents(msgs))
	}
}

// TestRecordInterruptionRejectsEmptyConversation 空会话 id 应报错而非静默写入。
func TestRecordInterruptionRejectsEmptyConversation(t *testing.T) {
	s, _ := newInterruptionService(t)
	if err := s.RecordInterruption(context.Background(), "", 1); err == nil {
		t.Fatalf("空会话 id 应返回错误")
	}
}

func contents(msgs []*db.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}
