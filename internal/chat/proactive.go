package chat

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/usage"
)

// 本文件实现「主动搭话」的两段式生成（见 docs/REALISM-ANALYSIS.md，P2 主动搭话三段式）。
//
// 背景：此前的主动发言是**硬编码模板**（"我还记着你刚才说的「…」，要不要继续从这里往下处理？"），
// 每次都同一个句式——比不说话更伤真实感（读者立刻识别出复读）。
//
// 现在的流程：
//  1. **选话题**（廉价 LLM 调用，max_tokens 很小）：给模型若干"最近聊过的事"，让它挑一件
//     此刻最想说的。无合适话题时可回 "none" → 不打扰用户。
//  2. **判时机**（客观信号，不花 LLM）：距上次发言太近、或刚触发过同类话题 → 放弃本次。
//  3. **生成发言**：用选定话题 + 当前情绪 + 人设，生成一句真正像"人主动开口"的话。
//
// 为什么不把"判时机"也交给 LLM：每次主动搭话都多一次调用不划算，而且"不到 90 秒内又说一次"
// 这种判断用客观信号更可靠（对标 MaiMBot 用二元 gate 挡在生成前的思路，但用更省的实现）。

const (
	// proactiveTopicCandidateCount 是喂给「选话题」的候选数量。太多会稀释，太少容易重复。
	proactiveTopicCandidateCount = 5
	// proactiveTopicMinRunes 过滤掉过短（无信息量）的候选。
	proactiveTopicMinRunes = 4
	// proactiveMinGapSeconds 两次主动发言之间的最小间隔（秒）。
	proactiveMinGapSeconds = 90
)

// ProactiveResult 是一次主动搭话的结果。
type ProactiveResult struct {
	Speech  string        // 要说的话；为空表示本次决定不打扰
	Emotion EmotionVector // 生成时的情绪（也用于前端表情）
	Topic   string        // 实际选中的话题（供日志/诊断）
	Skipped bool          // 是否因"时机不合适"或"无话题"而放弃
}

// GenerateProactive 生成一次主动搭话（两段式）。
//
// history 是最近的对话（最早在前），用于挑选话题；lastProactiveSpeechAt 是上一次主动发言的时间
// （零值表示从未说过），用于判断时机。
func (s *Service) GenerateProactive(
	ctx context.Context,
	conversationID string,
	trigger string,
	history []*schema.Message,
	lastProactiveSpeechAt time.Time,
) (ProactiveResult, error) {
	if s == nil || s.db == nil {
		return ProactiveResult{}, fmt.Errorf("proactive: service is not ready")
	}

	// 判断时机：距上次主动开口太近就不再打扰（避免"刚说完又搭话"的粘人感）。
	if since := time.Since(lastProactiveSpeechAt); !lastProactiveSpeechAt.IsZero() && since < proactiveMinGapSeconds*time.Second {
		slog.Info("[chat] proactive skipped: too soon", "since_s", int(since.Seconds()))
		return ProactiveResult{Skipped: true}, nil
	}

	candidates := buildProactiveTopicCandidates(history, proactiveTopicCandidateCount)
	if len(candidates) == 0 {
		// 没有可聊的素材（例如刚开场）：不硬凑话题。
		slog.Info("[chat] proactive skipped: no topic candidates", "trigger", trigger)
		return ProactiveResult{Skipped: true}, nil
	}

	collector := usage.NewCollector()
	trackedModel, providerID, modelName, err := s.createTrackedModel(ctx, collector)
	if err != nil {
		return ProactiveResult{}, fmt.Errorf("proactive: create model: %w", err)
	}

	// —— 第一段：选话题（廉价：max_tokens 很小）——
	topic, err := s.pickProactiveTopic(ctx, trackedModel, candidates)
	if err != nil {
		return ProactiveResult{}, err
	}
	if topic == "" {
		slog.Info("[chat] proactive skipped: model found no suitable topic", "trigger", trigger)
		return ProactiveResult{Skipped: true}, nil
	}

	// —— 第二段：生成发言（带上当前情绪与人设）——
	speech, emotion, err := s.writeProactiveSpeech(ctx, conversationID, trackedModel, topic)
	if err != nil {
		return ProactiveResult{}, err
	}
	if speech == "" {
		return ProactiveResult{Skipped: true, Topic: topic}, nil
	}

	s.persistTokenUsage(ctx, conversationID, providerID, modelName, "proactive", collector, 0, "success", nil)
	return ProactiveResult{Speech: speech, Emotion: emotion, Topic: topic}, nil
}

// buildProactiveTopicCandidates 从最近对话里挑出可聊的素材。
//
// 只取**用户说过的话**：主动搭话应是"接住对方提过的事"，而不是复述自己的发言。
func buildProactiveTopicCandidates(history []*schema.Message, limit int) []string {
	if limit <= 0 {
		limit = proactiveTopicCandidateCount
	}
	out := make([]string, 0, limit)
	seen := make(map[string]bool)
	// 从最近往前找，保证优先使用最新的话题。
	for i := len(history) - 1; i >= 0 && len(out) < limit; i-- {
		msg := history[i]
		if msg == nil || msg.Role != schema.User {
			continue
		}
		text := strings.TrimSpace(msg.Content)
		if len([]rune(text)) < proactiveTopicMinRunes {
			continue
		}
		// 截断成短话题，避免把整段话塞进 prompt。
		text = truncateForTopic(text, 40)
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

// pickProactiveTopic 让模型从候选里挑一件"此刻最想聊的"。
// 返回空串表示不聊（模型认为没有合适话题）。
func (s *Service) pickProactiveTopic(ctx context.Context, m model.BaseChatModel, candidates []string) (string, error) {
	var b strings.Builder
	for i, c := range candidates {
		fmt.Fprintf(&b, "%d. %s\n", i+1, c)
	}

	system := &schema.Message{
		Role: schema.System,
		Content: `你在决定"现在主动找主人聊点什么"。下面是主人最近提过的事：
` + b.String() + `
请从中挑一件此刻最自然、最值得开口的（通常是最新、最有情绪或最有下文的那件）。
只输出那一件的**原编号数字**（如 3）。如果你判断现在不适合开口，只输出 none。
不要输出任何解释、标点或多余文字。`,
	}
	result, err := m.Generate(ctx, []*schema.Message{system}, model.WithMaxTokens(8))
	if err != nil {
		return "", fmt.Errorf("proactive: pick topic: %w", err)
	}

	answer := strings.TrimSpace(strings.ToLower(result.Content))
	if strings.Contains(answer, "none") || answer == "" {
		return "", nil
	}
	// 从回答里取第一个数字作为编号。
	idx := 0
	for _, r := range answer {
		if r >= '1' && r <= '9' {
			idx = int(r - '0')
			break
		}
	}
	if idx < 1 || idx > len(candidates) {
		// 模型没按格式回答：保守起见当作"不聊"，而不是随机猜一个。
		slog.Info("[chat] proactive: unexpected topic answer", "answer", answer)
		return "", nil
	}
	return candidates[idx-1], nil
}

// writeProactiveSpeech 用选定话题生成一句真正的主动发言，并推进情绪状态。
func (s *Service) writeProactiveSpeech(
	ctx context.Context,
	conversationID string,
	m model.BaseChatModel,
	topic string,
) (string, EmotionVector, error) {
	styleNotes := nonEmptyString(s.cfg.Chat.StyleNotes, "Speak natural colloquial Chinese.")

	system := &schema.Message{
		Role: schema.System,
		Content: fmt.Sprintf(`你是 %s。
人设：%s
说话风格：%s

现在你**主动**开口找主人说话（没有人问你）。
想聊的事由：%s

要求：
- 用一两句自然的口语说出来，像真人忽然想到什么就开口，不要像客服开场白。
- 不要复述"你刚才说…"这种总结句式，直接说事、或表达你的想法/好奇/关心。
- 可以有语气词（嗯、诶、那个）和自己的情绪。
- 只输出你要说的话本身，不要引号、不要动作/神态描写、不要解释。`,
			nonEmptyString(s.cfg.Chat.BotName, "Yuyu"),
			nonEmptyString(s.cfg.Chat.Persona, "一个活泼、有点调皮的陪伴者。"),
			styleNotes,
			topic),
	}
	result, err := m.Generate(ctx, []*schema.Message{system}, model.WithMaxTokens(200))
	if err != nil {
		return "", EmotionVector{}, fmt.Errorf("proactive: write speech: %w", err)
	}

	speech := postprocessReply(result.Content)
	if speech == "" {
		return "", EmotionVector{}, nil
	}
	// 主动开口同样带一点口语冗余，否则"没人问你也说得像念稿"会更突兀。
	if s.cfg.Chat.AllowTypoSimulation {
		speech = applyDisfluency(speech, rand.Float64(), rand.Float64())
	}

	// 让主动发言也走情绪状态机：延续当前情绪，并按内容微调（保持与对话一致的惯性）。
	// 主动开口本身就是"被这件事触动"，因此给较高的兴趣度。
	suggested := EmotionVector{Emotion: InferEmotionFromText(speech), Mood: MoodCalm, Arousal: 0.45}
	applied := suggested
	if s.emotions != nil {
		applied, _ = s.emotions.MaybeUpdate(conversationID, suggested, 0.8, rand.Float64(), time.Now())
	}
	return speech, applied, nil
}

// truncateForTopic 按字符（而非字节）截断话题文本，避免切断多字节中文；
// 截断时加省略号，让模型知道这只是片段。
//
// 注意：agents.go 里已有一个 truncateRunes（不带省略号、用于历史渲染），
// 这里刻意分开命名，避免两处语义混淆。
func truncateForTopic(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
