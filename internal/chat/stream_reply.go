package chat

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

// streamingSentencer 增量地把流式文本切分成完整句子。
// 每遇到句末标点（。！？.!?\n）即产出一个完整句；若单句过长（超过 maxRunes）则强制切分，
// 使 TTS 能「逐句」并行启动，而不是等全文生成完毕。
type streamingSentencer struct {
	buf      strings.Builder
	maxRunes int
}

func newStreamingSentencer(maxRunes int) *streamingSentencer {
	if maxRunes <= 0 {
		maxRunes = 90
	}
	return &streamingSentencer{maxRunes: maxRunes}
}

func (s *streamingSentencer) feed(chunk string) []string {
	s.buf.WriteString(chunk)
	return s.drain(false)
}

func (s *streamingSentencer) flush() []string {
	return s.drain(true)
}

func (s *streamingSentencer) drain(force bool) []string {
	text := s.buf.String()
	parts := make([]string, 0, 2)

	for {
		runes := []rune(text)
		// 跳过前导空白。
		start := 0
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t' || runes[start] == '\r' || runes[start] == '\n') {
			start++
		}
		if start >= len(runes) {
			text = ""
			break
		}
		runes = runes[start:]
		text = string(runes)

		cut := -1
		for i, r := range runes {
			if isSentenceBoundary(r) {
				cut = i + 1
				break
			}
		}
		if cut < 0 && len(runes) > s.maxRunes {
			cut = s.maxRunes
		}
		if cut < 0 {
			break
		}
		parts = append(parts, strings.TrimSpace(string(runes[:cut])))
		text = string(runes[cut:])
	}

	if force && strings.TrimSpace(text) != "" {
		parts = append(parts, strings.TrimSpace(text))
		text = ""
	}

	s.buf.Reset()
	s.buf.WriteString(text)
	return parts
}

// streamReply 流式生成回复：边生成边按完整句子 emit + 持久化，降低「首句」延迟。
func (s *Service) streamReply(
	ctx context.Context,
	replyer *ReplyerAgent,
	snapshot TurnSnapshot,
	decision PlannerDecision,
	memories []string,
	toolResults []ToolResult,
	emitter Emitter,
	rt *ConversationRuntime,
) ([]string, error) {
	reader, err := replyer.Stream(ctx, snapshot, decision, memories, toolResults)
	if err != nil {
		return nil, fmt.Errorf("replyer stream: %w", err)
	}
	defer reader.Close()

	sentencer := newStreamingSentencer(replyer.cfg.SplitMaxChars)
	now := time.Now()
	parts := make([]string, 0, 8)
	totalRunes := 0

	flushPart := func(part string) error {
		totalRunes += len([]rune(part))
		if replyer.cfg.MaxReplyChars > 0 && totalRunes > replyer.cfg.MaxReplyChars {
			return fmt.Errorf("reply length exceeds max %d", replyer.cfg.MaxReplyChars)
		}
		parts = append(parts, part)
		if emitter != nil {
			emitter.Emit(ChatEvent{Type: EventTypeToken, Content: part})
		}
		return s.db.Messages.Create(ctx, &db.Message{
			ID:             uuid.New().String(),
			ConversationID: snapshot.Target.ConversationID,
			Role:           "assistant",
			Content:        part,
			SourceKind:     "guided_reply",
			Emotion:        decision.Emotion,
			Mood:           decision.Mood,
			Energy:         decision.Energy,
			Valence:        decision.Valence,
			Dominance:      decision.Dominance,
			Gesture:        decision.Gesture,
			Hand:           decision.Hand,
			CreatedAt:      now,
		})
	}

	emitSentences := func(sentences []string) error {
		for _, sentence := range sentences {
			part := postprocessReply(sentence)
			if part == "" {
				continue
			}
			if err := flushPart(part); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil && recvErr != io.EOF {
			return nil, fmt.Errorf("replyer stream recv: %w", recvErr)
		}
		if recvErr == io.EOF {
			break
		}
		if chunk == nil {
			continue
		}
		if err := emitSentences(sentencer.feed(chunk.Content)); err != nil {
			return nil, err
		}
	}
	if err := emitSentences(sentencer.flush()); err != nil {
		return nil, err
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("replyer produced empty reply")
	}
	rt.CompleteReply(parts, now)
	return parts, nil
}
