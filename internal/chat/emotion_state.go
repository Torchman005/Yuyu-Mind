package chat

import (
	"math"
	"strings"
	"sync"
	"time"
)

// 本文件实现「情绪状态机」：把情绪从"每轮现算的标签"变成"有惯性的慢变量"。
//
// 背景（见 docs/REALISM-ANALYSIS.md，P1）：LLM 每轮独立给出 emotion/valence/arousal，
// 于是会出现"上一秒 sad 下一秒 cheer"的跳变——真人情绪不是这样，它有惯性、会随时间平复，
// 且不会单轮翻转。这里用两步修正：
//
//  1. **时间衰减**：两次互动间隔越久，情绪越向基线回归（valence→0、arousal→0.5）。
//     用指数衰减 exp(-k·dt)，因此与"过了多久"直接相关，而不是"过了几轮"。
//  2. **惯性平滑**：新值只在旧值基础上按权重移动（move 比例），单轮的 LLM 判断无法翻转情绪。
//
// 另加**最短保持时长**：刚刚才切换过的离散表情标签，短时间内不再切换，避免表情抖动。
//
// 状态目前保存在进程内存里（按会话区分）。这意味着重启应用后情绪回到基线——
// 这在语义上等同于"睡了一觉心情平复"，可以接受；若将来需要跨重启延续，可把
// EmotionVector 落库并把 Load/Store 换成仓储实现。

const (
	// emotionDecayHalfLife 是情绪回落一半所需的时间。3 分钟是一个偏"健谈"的设定：
	// 短暂沉默仍延续情绪，聊完一阵后会明显平复。
	emotionDecayHalfLife = 3 * time.Minute

	// valenceBase / arousalBase 是不对称基线：心情归中性，唤醒度归"平静但不呆滞"。
	valenceBase = 0.0
	arousalBase = 0.5

	// emotionInertia 表示"新值向 LLM 建议值移动的比例"。
	// 0.35 意味着单轮最多走 35%——想从开心翻到难过至少需要连续几轮，符合真人的情绪惯性。
	emotionInertia = 0.35

	// emotionLabelHold 是离散表情标签的最短保持时长，防止单轮抖动造成"变脸"。
	emotionLabelHold = 25 * time.Second

	// emotionSwitchMargin：当旧标签仍处于保持期内时，新标签需要在效价上明显不同才允许切换。
	emotionSwitchMargin = 0.5

	// —— 概率门控参数（稀疏情绪更新，见 MaybeUpdate）——
	// moodBaseUpdateProbability 是"基线概率"：一条普通消息推动情绪的概率。
	moodBaseUpdateProbability = 0.5
	// moodTimeMultiplierMax 是时间增益上限（距上次更新越久越容易变，避免情绪僵死）。
	moodTimeMultiplierMax = 2.0
	// interestMultiplierMin 是"完全不感兴趣"时的倍数下限（留一点余量，防止彻底锁死）。
	interestMultiplierMin = 0.5
	// moodMinUpdateProbability 是概率下限，保证情绪最终仍可能变化。
	moodMinUpdateProbability = 0.2
)

// EmotionVector 是情绪的连续状态（VAD 的简化两维 + 离散标签）。
type EmotionVector struct {
	Emotion   string
	Mood      string
	Valence   float64
	Arousal   float64 // 对应 schema 里的 energy
	Dominance float64
	UpdatedAt time.Time
}

// emotionStateStore 按会话保存情绪状态。
type emotionStateStore struct {
	mu     sync.Mutex
	states map[string]*EmotionVector
}

func newEmotionStateStore() *emotionStateStore {
	return &emotionStateStore{states: make(map[string]*EmotionVector)}
}

// Update 用本轮 LLM 给出的情绪建议更新会话状态，**不经过概率门控**（总是推进）。
//
// 保留它是为了语义清晰与可测试性：需要确定性推进时用 Update；生产路径用 MaybeUpdate
// 以获得"稀疏更新"行为（见其注释）。
func (s *emotionStateStore) Update(conversationID string, suggested EmotionVector, now time.Time) EmotionVector {
	// r < 0 是"强制推进"的显式信号（见 shouldAdvanceMood），避免依赖随机数与概率上界的边界比较。
	next, _ := s.MaybeUpdate(conversationID, suggested, 1.0, -1.0, now)
	return next
}

// MaybeUpdate 按「概率门控」决定是否用本轮建议推进情绪状态。
//
// 为什么要门控（见 docs/REALISM-ANALYSIS.md P1，对标 MoFox）：
// 如果每一轮对话都让模型重新评估心情，会有三个坏处——
//   - 情绪抖动（用户说句无关紧要的话，心情也跟着变）；
//   - 每次都要为"情绪"付出一次 LLM 判断的成本；
//   - 角色表现得像"每句话都在重新评估你"，非常假。
//
// 真人的情绪是**稀疏演化**的：大多时候心情延续，只在被真正触动时才变化。
// 因此这里用 `基础概率 × 时间增益 × 兴趣度` 作为更新概率；未命中时情绪**保持不动**
// （但仍按经过时间做衰减，所以久不互动仍会平复）。
//
// interest ∈ [0,1] 表示这条消息对情绪的触发强度（见 ExtractInterest）。
// r 是 [0,1) 的随机数，由调用方注入以便测试确定性。
func (s *emotionStateStore) MaybeUpdate(
	conversationID string,
	suggested EmotionVector,
	interest float64,
	r float64,
	now time.Time,
) (EmotionVector, bool) {
	if now.IsZero() {
		now = time.Now()
	}
	suggested.Valence = ClampValence(suggested.Valence)
	suggested.Arousal = ClampEnergy(suggested.Arousal)
	suggested.Dominance = ClampDominance(suggested.Dominance)

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.states[conversationID]
	if !ok || prev == nil {
		// 首次：直接采用建议值作为起点（没有历史可继承，不算"跳变"）。
		next := suggested
		next.UpdatedAt = now
		next.Emotion = NormalizeEmotion(next.Emotion)
		next.Mood = NormalizeMood(next.Mood)
		next.Arousal = clamp01(next.Arousal)
		s.states[conversationID] = &next
		return next, true
	}

	elapsed := now.Sub(prev.UpdatedAt)
	if elapsed < 0 {
		elapsed = 0
	}

	// 1) 时间衰减：无论是否更新，情绪都按经过时间向基线平复。
	decay := math.Exp(-math.Ln2 * elapsed.Seconds() / emotionDecayHalfLife.Seconds())
	base := EmotionVector{
		Emotion:   prev.Emotion,
		Mood:      prev.Mood,
		Valence:   valenceBase + (prev.Valence-valenceBase)*decay,
		Arousal:   arousalBase + (prev.Arousal-arousalBase)*decay,
		Dominance: prev.Dominance * decay,
		UpdatedAt: now,
	}

	// 2) 概率门控：不感兴趣的消息不推进情绪，只让它自然平复。
	if !shouldAdvanceMood(elapsed, interest, r) {
		// 未命中：保留标签与基调（情绪延续），但写入衰减后的数值。
		base.Emotion = prev.Emotion
		base.Mood = prev.Mood
		s.states[conversationID] = &base
		return base, false
	}

	// 3) 命中：向建议值做惯性移动，并决定离散标签是否允许切换。
	next := EmotionVector{
		Valence:   ClampValence(base.Valence + (suggested.Valence-base.Valence)*emotionInertia),
		Arousal:   ClampEnergy(base.Arousal + (suggested.Arousal-base.Arousal)*emotionInertia),
		Dominance: ClampDominance(base.Dominance + (suggested.Dominance-base.Dominance)*emotionInertia),
		UpdatedAt: now,
	}
	next.Emotion = decideEmotionLabel(prev, suggested, next, now)
	if next.Emotion == prev.Emotion {
		next.Mood = NormalizeMood(prev.Mood)
	} else {
		next.Mood = NormalizeMood(suggested.Mood)
	}

	s.states[conversationID] = &next
	return next, true
}

// clamp01 把数值夹到 [0,1]（用于 arousal 与兴趣度这类 0..1 量）。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// shouldAdvanceMood 计算"这次要不要推进情绪"。
//
// 概率 = baseProbability × timeMultiplier × interestMultiplier，并夹在
// [minProbability, 1] 内：
//   - 时间增益：距上次更新越久越可能变化（最多 timeMultiplierMax 倍），
//     保证长时间延续同一情绪时不会永远僵住；
//   - 兴趣倍数：不感兴趣（interest=0）时降到极低，但仍留一点余量，
//     以免完全无感的消息永远无法推动情绪；
//   - 下限：避免概率为 0 导致情绪彻底锁死。
func shouldAdvanceMood(elapsed time.Duration, interest float64, r float64) bool {
	// 负随机数是"强制推进"的显式信号：调用方（Update）用它绕过门控，
	// 避免依赖"随机数恰好小于概率上界"这类边界假设。
	if r < 0 {
		return true
	}
	if interest < 0 {
		interest = 0
	}
	if interest > 1 {
		interest = 1
	}
	// 时间增益：从 1 倍平滑增长到 timeMultiplierMax 倍（半衰期尺度）。
	timeMultiplier := 1 + (moodTimeMultiplierMax-1)*(1-math.Exp(-elapsed.Seconds()/emotionDecayHalfLife.Seconds()))
	// 兴趣倍数：0 → interestMultiplierMin，1 → 1.0。
	interestMultiplier := interestMultiplierMin + (1-interestMultiplierMin)*interest

	probability := moodBaseUpdateProbability * timeMultiplier * interestMultiplier
	if probability < moodMinUpdateProbability {
		probability = moodMinUpdateProbability
	}
	if probability > 1 {
		probability = 1
	}
	return r < probability
}

// ExtractInterest 从用户消息估计"这条消息对情绪的触发强度"（0..1）。
//
// 刻意用**客观信号**而不是再调一次 LLM：情绪门控本身就是为了省成本与降抖动，
// 若为此再加一次模型调用就本末倒置了。
//
// 信号：长度（有内容）、疑问/感叹（有情绪张力）、明显情绪词、以及是否只是纯应答词。
func ExtractInterest(text string) float64 {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	// 纯应答词（嗯/好的/收到…）几乎不推动情绪——这条规则与 TurnGate 的弱回撤判定同源，
	// 但此处独立实现，避免两个文件相互依赖。
	if isLowInterestReply(trimmed) {
		return 0.05
	}

	score := 0.3 // 有一条实质内容，基础兴趣不为 0
	if n := len([]rune(trimmed)); n >= 8 {
		score += 0.2 // 有展开说的内容
	}
	if strings.ContainsAny(trimmed, "?？") {
		score += 0.2 // 在问问题，期待回应
	}
	if strings.ContainsAny(trimmed, "!！") {
		score += 0.2 // 有情绪张力
	}
	for _, marker := range []string{
		"开心", "高兴", "太好", "喜欢", "爱", "谢谢", "哈哈", "好耶", "厉害",
		"难过", "累", "烦", "生气", "讨厌", "害怕", "担心", "失望", "焦虑", "紧张",
		"委屈", "孤独", "崩溃", "怎么办", "救命", "糟糕", "完了", "离谱",
	} {
		if strings.Contains(trimmed, marker) {
			score += 0.3
			break
		}
	}
	return clamp01(score)
}

// isLowInterestReply 判断是否只是"应声"（这类消息不该推动情绪）。
func isLowInterestReply(text string) bool {
	if len([]rune(text)) > 8 {
		return false
	}
	if strings.ContainsAny(text, "?？") {
		return false // 疑问是在等回应，不算应声
	}
	normalized := strings.ToLower(strings.Trim(text, "。．.,，!！~～、…· \t"))
	switch normalized {
	case "好", "好的", "好嘞", "好了", "行", "行了", "可以", "嗯", "嗯嗯", "嗯呢",
		"哦", "噢", "喔", "啊", "呀", "知道了", "明白了", "了解", "懂了", "收到",
		"没事", "没事儿", "ok", "okay", "k", "yeah", "yep":
		return true
	default:
		return false
	}
}

// decideEmotionLabel 决定本轮的离散表情标签。
//
// 规则（防止"变脸"）：
//   - 仍处于最短保持期内且效价变化不明显 → 保持旧标签；
//   - 已过保持期，或效价变化足够大 → 采用建议标签。
func decideEmotionLabel(prev *EmotionVector, suggested, blended EmotionVector, now time.Time) string {
	want := NormalizeEmotion(suggested.Emotion)
	if prev.Emotion == "" {
		return want
	}
	if want == prev.Emotion {
		return want
	}
	held := now.Sub(prev.UpdatedAt) < emotionLabelHold
	if held && math.Abs(blended.Valence-prev.Valence) < emotionSwitchMargin {
		return prev.Emotion // 刚换过且情绪没有明显变化：不抖
	}
	return want
}

// decisionWithEmotion 把平滑后的情绪写回 PlannerDecision。
//
// 为什么需要这一步：前端表情由 EventTypeEmotion 驱动（用平滑值），而 Replyer 的表演指令与
// 消息落库读的是 PlannerDecision。若两者用不同来源，就会出现"表情已经是难过、台词却还在雀跃"
// 的前后矛盾。这里让 decision 承载最终生效的情绪，保证全链路一致（见 REALISM-ANALYSIS P1）。
//
// 只覆盖情绪字段，不动 action/tool/task 等决策信息。
func decisionWithEmotion(d PlannerDecision, e EmotionVector) PlannerDecision {
	d.Emotion = e.Emotion
	d.Mood = e.Mood
	d.Valence = e.Valence
	d.Energy = e.Arousal
	d.Dominance = e.Dominance
	return d
}

// Current 返回会话当前的情绪状态（不存在时返回零值与 false）。
// 供测试与诊断使用。
func (s *emotionStateStore) Current(conversationID string) (EmotionVector, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.states[conversationID]
	if !ok || cur == nil {
		return EmotionVector{}, false
	}
	return *cur, true
}

// emotionPromptLine 把当前情绪转成一句**刻意模糊**的自然语言描述，供注入 Planner。
//
// 为什么用模糊措辞而不是精确数值：MaiMBot 的实现证实——情绪只应影响"选词与语气倾向"，
// 一旦写成精确指令（"输出 20 字以内"）或直接暴露数值，模型会开始机械地执行数字，
// 回复立刻露出机器感（见 docs/REALISM-ANALYSIS.md P1）。所以这里用"心情不错""有点激动"
// 这类人类自己的说法，把表演空间留给模型。
//
// 返回空串表示当前没有可用情绪状态（例如会话刚开始）。
func emotionPromptLine(v EmotionVector) string {
	if v.UpdatedAt.IsZero() {
		return ""
	}
	var b strings.Builder

	switch {
	case v.Valence >= 0.45:
		b.WriteString("你现在心情很好")
	case v.Valence >= 0.12:
		b.WriteString("你现在心情还不错")
	case v.Valence <= -0.45:
		b.WriteString("你现在心情不太好")
	case v.Valence <= -0.12:
		b.WriteString("你现在情绪有点低")
	default:
		b.WriteString("你现在心情比较平静")
	}

	switch {
	case v.Arousal >= 0.7:
		b.WriteString("，情绪比较激动")
	case v.Arousal <= 0.3:
		b.WriteString("，人比较安静、提不起劲")
	}

	// 支配度只做轻重提示，避免堆成参数表。
	if v.Dominance <= -0.4 {
		b.WriteString("，有点没底气")
	} else if v.Dominance >= 0.4 {
		b.WriteString("，说话比较有底气")
	}

	b.WriteString("。这是你此刻真实的情绪底色，回复时自然地带上它，但不要刻意强调或解释自己的心情。")
	return b.String()
}

// CurrentEmotion 返回会话当前情绪（供 Planner 注入"情绪底色"）。
// 没有历史状态时返回零值与 false。
func (s *Service) CurrentEmotion(conversationID string) (EmotionVector, bool) {
	if s == nil || s.emotions == nil {
		return EmotionVector{}, false
	}
	return s.emotions.Current(conversationID)
}
