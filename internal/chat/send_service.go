package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/config"
	"github.com/yuyu-mind/backend/internal/db"
)

type SendService struct {
	db  *db.DB
	cfg config.ChatConfig
}

func NewSendService(database *db.DB, cfg config.ChatConfig) *SendService {
	if cfg.MaxReplyChars <= 0 {
		cfg.MaxReplyChars = 500
	}
	if cfg.SplitMaxChars <= 0 {
		cfg.SplitMaxChars = 90
	}
	return &SendService{db: database, cfg: cfg}
}

func (s *SendService) SendGuidedReply(
	ctx context.Context,
	rt *ConversationRuntime,
	snapshot TurnSnapshot,
	raw string,
	emotion EmotionInfo,
	emitter Emitter,
) ([]string, error) {
	cleaned := postprocessReply(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("replyer produced empty reply")
	}
	if len([]rune(cleaned)) > s.cfg.MaxReplyChars {
		return nil, fmt.Errorf("reply length %d exceeds max %d", len([]rune(cleaned)), s.cfg.MaxReplyChars)
	}

	parts := splitReply(cleaned, s.cfg.SplitMaxChars)
	if len(parts) == 0 {
		return nil, fmt.Errorf("reply split produced no sendable messages")
	}

	now := time.Now()
	for _, part := range parts {
		if emitter != nil {
			emitter.Emit(ChatEvent{Type: EventTypeToken, Content: part})
		}
		if err := s.db.Messages.Create(ctx, &db.Message{
			ID:             uuid.New().String(),
			ConversationID: snapshot.Target.ConversationID,
			Role:           "assistant",
			Content:        part,
			SourceKind:     "guided_reply",
			Emotion:        emotion.Emotion,
			Mood:           emotion.Mood,
			Energy:         emotion.Energy,
			Valence:        emotion.Valence,
			Dominance:      emotion.Dominance,
			Gesture:        emotion.Gesture,
			Hand:           emotion.Hand,
			CreatedAt:      now,
		}); err != nil {
			return nil, fmt.Errorf("persist guided reply: %w", err)
		}
	}

	rt.CompleteReply(parts, now)
	return parts, nil
}

var stageLinePattern = regexp.MustCompile(`(?m)^\s*[\(（\[\[【][^\n]{0,80}[\)）\]\]】]\s*$`)
var leadingStagePattern = regexp.MustCompile(`^\s*[\(（\[\[【][^\n]{0,40}[\)）\]\]】]\s*`)

func postprocessReply(raw string) string {
	text := strings.TrimSpace(raw)
	text = stageLinePattern.ReplaceAllString(text, "")
	text = leadingStagePattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	text = strings.Join(nonEmpty(lines), "\n")
	return strings.TrimSpace(text)
}

func splitReply(text string, maxRunes int) []string {
	if maxRunes <= 0 || len([]rune(text)) <= maxRunes {
		return []string{text}
	}

	var parts []string
	var current strings.Builder
	currentRunes := 0
	for _, r := range text {
		current.WriteRune(r)
		currentRunes++
		if isSentenceBoundary(r) || currentRunes >= maxRunes {
			part := trimSentencePart(current.String())
			if part != "" {
				parts = append(parts, part)
			}
			current.Reset()
			currentRunes = 0
		}
	}
	if rest := trimSentencePart(current.String()); rest != "" {
		parts = append(parts, rest)
	}
	return parts
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '.', '!', '?', '\n':
		return true
	default:
		return false
	}
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}
