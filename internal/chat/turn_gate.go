package chat

import (
	"strings"
	"time"

	"github.com/yuyu-mind/backend/internal/config"
)

type GateDecision struct {
	ShouldPlan bool
	Score      float64
	Threshold  float64
	Reasons    []string
}

type TurnGate struct {
	cfg config.ChatConfig
}

func NewTurnGate(cfg config.ChatConfig) *TurnGate {
	if cfg.ReplyThreshold <= 0 {
		cfg.ReplyThreshold = 0.45
	}
	if cfg.ReplyFrequency <= 0 {
		cfg.ReplyFrequency = 1
	}
	return &TurnGate{cfg: cfg}
}

func (g *TurnGate) Evaluate(snapshot TurnSnapshot) GateDecision {
	score := 0.0
	reasons := make([]string, 0, 8)
	target := snapshot.Target
	content := strings.TrimSpace(target.Content)

	if target.Mentioned {
		score += 0.75
		reasons = append(reasons, "mentioned")
	}

	// 这是「一对一私有桌宠」：用户主动发的每一条消息都应得到回复。
	// 语音循环里的噪声/弱回撤已由前端 isUsableVoiceTranscript 等在上游过滤，
	// 因此这里把直接消息的基础分抬到阈值之上，保证「发消息必回复」。
	score += 0.60
	reasons = append(reasons, "private_session")

	if looksLikeQuestionOrRequest(content) {
		score += 0.25
		reasons = append(reasons, "question_or_request")
	}
	if looksLikeWeakBackchannel(content) {
		// 极短的应答词（「好的/嗯/哦/知道了」）降分，避免对无意义复读。
		score -= 0.30
		reasons = append(reasons, "weak_backchannel")
	}
	if len(snapshot.Pending) > 1 {
		score += minFloat(float64(len(snapshot.Pending)-1)*0.08, 0.25)
		reasons = append(reasons, "pending_batch")
	}
	if snapshot.LastBotAt.IsZero() || snapshot.Now.Sub(snapshot.LastBotAt) >= g.averageInterval(snapshot) {
		score += 0.12
		reasons = append(reasons, "idle_gap")
	}
	if g.cfg.MinReplyIntervalSeconds > 0 && !snapshot.LastBotAt.IsZero() {
		if snapshot.Now.Sub(snapshot.LastBotAt) < time.Duration(g.cfg.MinReplyIntervalSeconds)*time.Second && !target.Mentioned {
			score -= 0.25
			reasons = append(reasons, "reply_cooldown")
		}
	}

	score *= g.cfg.ReplyFrequency
	threshold := g.cfg.ReplyThreshold
	return GateDecision{
		ShouldPlan: score >= threshold,
		Score:      score,
		Threshold:  threshold,
		Reasons:    reasons,
	}
}

func (g *TurnGate) averageInterval(snapshot TurnSnapshot) time.Duration {
	if g.cfg.AverageMessageIntervalSeconds > 0 {
		return time.Duration(g.cfg.AverageMessageIntervalSeconds) * time.Second
	}
	return 8 * time.Second
}

func looksLikeQuestionOrRequest(content string) bool {
	lower := strings.ToLower(content)
	if strings.ContainsAny(content, "?？吗呢么") {
		return true
	}
	for _, marker := range []string{
		"帮我", "可以", "能不能", "要不要", "怎么", "为什么", "如何", "建议", "想问",
		"please", "can you", "could you", "how", "why", "what should",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// looksLikeWeakBackchannel 判断是否为「弱回撤」——极短且是常见应答词，
// 这类消息不值得逐条复读（如「好的」「嗯」「知道了」）。
func looksLikeWeakBackchannel(content string) bool {
	c := strings.TrimSpace(content)
	if len([]rune(c)) > 6 {
		return false
	}
	switch strings.ToLower(c) {
	case "好的", "好", "嗯", "嗯嗯", "哦", "噢", "啊", "知道了", "收到", "行", "ok", "okay":
		return true
	default:
		return false
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
