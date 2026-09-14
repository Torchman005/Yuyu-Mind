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
	// AllowSilenceOnBackchannel 是 bool，Go 的零值无法区分「未设置」与「显式关闭」。
	// 为不破坏既有调用方（如既有测试直接构造 ChatConfig），这里把「完全未配置」视为启用默认值；
	// 传入配置文件时 config.Load 已先填默认、再 JSON 覆盖，因此显式 false 仍能正确关闭。
	// 判定「未配置」的信号：其余拟人化开关同时为零值。
	if !cfg.AllowSilenceOnBackchannel && cfg.MinReplyIntervalSeconds == 0 && cfg.AverageMessageIntervalSeconds == 0 {
		cfg.AllowSilenceOnBackchannel = true
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

	// 这是「一对一私有桌宠」：正常情况下用户的消息都应得到回复。
	// 但基础分不能白送到阈值之上——否则「允许沉默」这类拟人行为永远不会发生
	// （见 docs/REALISM-ANALYSIS.md P0-1）。这里给 0.48 基线：普通陈述（含「今天好累啊」这类
	// 情绪化陈述）刚好过阈值，而弱回撤（惩罚见下）即使叠加冷场加成也到不了阈值。
	score += 0.48
	reasons = append(reasons, "private_session")

	if looksLikeQuestionOrRequest(content) {
		score += 0.25
		reasons = append(reasons, "question_or_request")
	}
	if looksLikeWeakBackchannel(content) {
		// 极短的应答词（「好的/嗯/哦/知道了」）不值得逐条复读。
		// 惩罚取 0.35：0.48-0.35=0.13，而冷场加成 0.15 也只能到 0.28，仍低于阈值 0.45，
		// 因此「允许沉默」是真正生效的——这正是真人感的关键之一（听到「嗯」不必每次都接话）。
		if g.cfg.AllowSilenceOnBackchannel {
			score -= 0.35
			reasons = append(reasons, "weak_backchannel")
		}
	}
	if len(snapshot.Pending) > 1 {
		score += minFloat(float64(len(snapshot.Pending)-1)*0.08, 0.25)
		reasons = append(reasons, "pending_batch")
	}
	if snapshot.LastBotAt.IsZero() || snapshot.Now.Sub(snapshot.LastBotAt) >= g.averageInterval(snapshot) {
		// 冷场后更愿意开口（模拟"憋了一会儿想说话"），但增益不足以把弱回撤退回阈值上。
		score += 0.15
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
//
// 注意两点：
//   - 带疑问语气（「哦？」「嗯？」）不算弱回撤——那是在追问，应当回复；
//   - 会剥掉句末标点再比对，「好的。」「嗯…」也算弱回撤。
func looksLikeWeakBackchannel(content string) bool {
	c := strings.TrimSpace(content)
	if len([]rune(c)) > 8 {
		return false
	}
	// 疑问语气说明用户在等回应，不是单纯应答。
	if strings.ContainsAny(c, "?？") {
		return false
	}
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(c), "。．.,，!！~～、…· \t"))
	switch normalized {
	case "好", "好的", "好嘞", "好滴", "好了", "行", "行了", "可以", "嗯", "嗯嗯", "嗯呢", "哦", "噢", "喔", "啊", "呀",
		"知道了", "明白了", "了解", "懂了", "收到", "没事", "没事儿",
		"ok", "okay", "k", "got it", "fine", "alright", "yeah", "yep":
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
