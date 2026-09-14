package chat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

// 本轮回复被打断时插入的标记。它让模型知道「上一句没说完就转到了新话题」，
// 从而不会在下一轮继续原来那句、也不会坚称自己说完了。
const interruptionMarker = "[被用户打断]"

// RecordInterruption 记录一次「用户打断」。
//
// 背景（见 docs/REALISM-ANALYSIS.md P1-7）：流式回复是「生成一句就落库一句」，
// 而 TTS 播放滞后于生成。用户打断时，那些**已经生成但还没播出**的句子仍留在历史里，
// 于是模型会"记得"自己说过用户从没听见的话——这是最伤真实感的穿帮之一（真人只记得自己说出口的）。
//
// 参数 playedCount 是本次回复中**已经播出**的句子数（由前端逐句播放时统计）。
// 语义：
//   - 保留末尾连续助手发言中前 playedCount 条（它们真的被听见了）；
//   - 删除其后的未播出条目；
//   - 只要发生了打断，就追加一条 [被用户打断] 标记（角色为 assistant），
//     使下一轮的上下文能看出对话是被打断的。
//
// playedCount < 0 视为 0（全部未播出）。播放完整时不调用本方法。
func (s *Service) RecordInterruption(ctx context.Context, conversationID string, playedCount int) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("record interruption: service is not ready")
	}
	if conversationID == "" {
		return fmt.Errorf("record interruption: conversation id is required")
	}
	if playedCount < 0 {
		playedCount = 0
	}

	tail, err := s.db.Messages.TailAssistantMessages(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("record interruption: list tail: %w", err)
	}

	// 删除未播出的部分。
	if playedCount < len(tail) {
		ids := make([]string, 0, len(tail)-playedCount)
		for _, m := range tail[playedCount:] {
			ids = append(ids, m.ID)
		}
		if err := s.db.Messages.DeleteMessages(ctx, ids); err != nil {
			return fmt.Errorf("record interruption: drop unplayed: %w", err)
		}
		slog.Info("[chat] interruption: dropped unplayed sentences",
			"conversation", conversationID, "kept", playedCount, "dropped", len(ids))
	}

	// 追加打断标记：让下一轮知道"话没说完就被打断了"。
	now := time.Now()
	if err := s.db.Messages.Create(ctx, &db.Message{
		ID:             uuid.New().String(),
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        interruptionMarker,
		SourceKind:     "interruption",
		CreatedAt:      now,
	}); err != nil {
		return fmt.Errorf("record interruption: insert marker: %w", err)
	}
	return nil
}
