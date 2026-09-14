package chat

import (
	"math"
	"strings"
	"testing"
	"time"
)

// TestMaybeUpdateSkipsWhenGateMisses 是"稀疏情绪更新"的核心断言：
// 门控未命中时情绪**保持不动**（心情延续），只按时间做衰减。
func TestMaybeUpdateSkipsWhenGateMisses(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: EmotionHappy, Mood: MoodCheer, Valence: 0.8, Arousal: 0.8}, now)

	// r 远大于概率（不感兴趣时概率上限 ≤ moodBaseUpdateProbability）→ 必然不推进。
	got, advanced := store.MaybeUpdate("c1",
		EmotionVector{Emotion: EmotionSad, Mood: MoodComfort, Valence: -0.9, Arousal: 0.2},
		0.0, 0.99, now.Add(time.Second))

	if advanced {
		t.Fatalf("门控未命中时不应推进情绪")
	}
	if got.Emotion != EmotionHappy {
		t.Fatalf("心情应延续（happy），实际 %q", got.Emotion)
	}
	if got.Valence <= 0 {
		t.Fatalf("未推进时不应跳到负值，实际 %v", got.Valence)
	}
}

// TestMaybeUpdateAdvancesWhenGateHits 门控命中时按惯性推进。
func TestMaybeUpdateAdvancesWhenGateHits(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: EmotionHappy, Mood: MoodCheer, Valence: 0.8}, now)

	got, advanced := store.MaybeUpdate("c1",
		EmotionVector{Emotion: EmotionHappy, Mood: MoodCheer, Valence: -0.8},
		1.0, 0.0, now) // 用同一时刻，避免时间衰减干扰位移断言

	if !advanced {
		t.Fatalf("r=0 必然命中门控")
	}
	// 仍受惯性限制：0.8 + (-0.8-0.8)*0.35 = 0.24
	want := 0.8 + (-0.8-0.8)*emotionInertia
	if math.Abs(got.Valence-want) > 1e-6 {
		t.Fatalf("命中后位移应为惯性比例：want %v got %v", want, got.Valence)
	}
}

// TestMaybeUpdateLowInterestIsHarderToAdvance 兴趣越低越难推进情绪
// （用同一个随机数，低兴趣不应命中、高兴趣应命中）。
func TestMaybeUpdateLowInterestIsHarderToAdvance(t *testing.T) {
	now := time.Now()

	// 门槛用同一个 r 比较：取一个介于"低兴趣概率上限"与"高兴趣概率"之间的值。
	// 初始（无历史）时 delta 固定为 1，因此这里取 r=0.4：
	// 低兴趣 probability ≈ 0.25~0.5、高兴趣 ≈ 0.5~1.0 → r=0.4 时前者常不命中、后者命中。
	low := newEmotionStateStore()
	low.Update("c", EmotionVector{Emotion: EmotionHappy, Valence: 0.8}, now)
	_, lowAdvanced := low.MaybeUpdate("c", EmotionVector{Emotion: EmotionSad, Valence: -0.8}, 0.0, 0.45, now.Add(time.Second))

	high := newEmotionStateStore()
	high.Update("c", EmotionVector{Emotion: EmotionHappy, Valence: 0.8}, now)
	_, highAdvanced := high.MaybeUpdate("c", EmotionVector{Emotion: EmotionSad, Valence: -0.8}, 1.0, 0.45, now.Add(time.Second))

	if !highAdvanced {
		t.Fatalf("高兴趣在 r=0.45 时应能推进（概率≈0.55）")
	}
	if lowAdvanced {
		t.Fatalf("低兴趣在 r=0.45 时不应推进（概率≈0.28）")
	}
}

// TestExtractInterestSignals 验证兴趣度的客观信号：
// 纯应答词几乎不推动情绪，有内容/疑问/情绪词的消息推动更强。
func TestExtractInterestSignals(t *testing.T) {
	cases := []struct {
		text string
		min  float64
		max  float64
	}{
		{"嗯", 0.0, 0.1},                 // 纯应声
		{"好的", 0.0, 0.1},               // 纯应声
		{"哦？", 0.3, 1.0},               // 疑问是在等回应，不算应声
		{"今天面试好紧张啊", 0.5, 1.0},        // 有内容 + 情绪词
		{"你能帮我看看这个报错吗？", 0.6, 1.0},   // 疑问 + 长度
		{"我好累啊", 0.5, 1.0},            // 情绪词
		{"", 0.0, 0.0},                 // 空
	}
	for _, tc := range cases {
		got := ExtractInterest(tc.text)
		if got < tc.min-1e-9 || got > tc.max+1e-9 {
			t.Fatalf("ExtractInterest(%q) = %.3f，期望落在 [%.2f, %.2f]", tc.text, got, tc.min, tc.max)
		}
	}

	// 应声的兴趣度必须明显低于有内容的消息。
	if ExtractInterest("嗯") >= ExtractInterest("我今天有点难过") {
		t.Fatalf("应声不应比有情绪内容的消息更容易推动情绪")
	}
}

// TestEmotionPromptLineIsVagueNotNumeric 锁定"情绪以模糊措辞注入"这一取舍。
//
// 关键不是措辞本身，而是**不能泄露数值**：一旦 prompt 里出现 0.72 这类数字，
// 模型会开始机械地执行数字而非"带着心情说话"（见 REALISM-ANALYSIS 对 MaiMBot 的观察）。
func TestEmotionPromptLineIsVagueNotNumeric(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		v    EmotionVector
		want []string // 必须包含的片段
	}{
		{"开心且激动", EmotionVector{Valence: 0.9, Arousal: 0.9, UpdatedAt: now}, []string{"心情很好", "比较激动"}},
		{"难过且低落", EmotionVector{Valence: -0.8, Arousal: 0.1, UpdatedAt: now}, []string{"心情不太好", "提不起劲"}},
		{"稍好", EmotionVector{Valence: 0.2, Arousal: 0.5, UpdatedAt: now}, []string{"心情还不错"}},
		{"稍低", EmotionVector{Valence: -0.2, Arousal: 0.5, UpdatedAt: now}, []string{"情绪有点低"}},
		{"中性", EmotionVector{Valence: 0, Arousal: 0.5, UpdatedAt: now}, []string{"心情比较平静"}},
		{"没底气", EmotionVector{Valence: 0, Arousal: 0.5, Dominance: -0.8, UpdatedAt: now}, []string{"没底气"}},
		{"有底气", EmotionVector{Valence: 0, Arousal: 0.5, Dominance: 0.8, UpdatedAt: now}, []string{"有底气"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := emotionPromptLine(tc.v)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Fatalf("描述应包含 %q，实际 %q", w, got)
				}
			}
			if !strings.Contains(got, "情绪底色") {
				t.Fatalf("应说明这是情绪底色，实际 %q", got)
			}
			// 不能出现数值：模型看到数字会开始"执行参数"而不是表演情绪。
			for _, bad := range []string{"0.", "-0.", "1.0", "0.9", "%"} {
				if strings.Contains(got, bad) {
					t.Fatalf("描述不应泄露数值 %q，实际 %q", bad, got)
				}
			}
		})
	}
}

// TestEmotionPromptLineEmptyWithoutState 会话刚开始（无情绪状态）时应返回空串，
// 让 Planner 拿到 "(none; ...)" 而不是伪造一个心情。
func TestEmotionPromptLineEmptyWithoutState(t *testing.T) {
	if got := emotionPromptLine(EmotionVector{Valence: 0.9, Arousal: 0.9}); got != "" {
		t.Fatalf("无 UpdatedAt（未建立状态）时应返回空串，实际 %q", got)
	}
}

// TestCurrentEmotionReflectsStore 验证 Service 层能读到状态机的当前情绪。
func TestCurrentEmotionReflectsStore(t *testing.T) {
	s, convID := newInterruptionService(t)
	if _, ok := s.CurrentEmotion(convID); ok {
		t.Fatalf("尚未产生情绪时不应返回 ok")
	}

	s.emotions = newEmotionStateStore()
	s.emotions.Update(convID, EmotionVector{Emotion: EmotionHappy, Valence: 0.8, Arousal: 0.7}, time.Now())

	got, ok := s.CurrentEmotion(convID)
	if !ok {
		t.Fatalf("应能读到已建立的情绪状态")
	}
	if got.Emotion != EmotionHappy {
		t.Fatalf("应读到 happy，实际 %q", got.Emotion)
	}
}

// TestDecisionWithEmotionKeepsConsistency 锁定「平滑情绪写回 decision」这一步。
//
// 这一步是表情与台词一致性的关键：前端表情读 EventTypeEmotion（平滑值），
// Replyer 的表演指令与消息落库读 PlannerDecision。如果哪天有人删掉写回，
// 就会出现"表情难过、台词雀跃"的矛盾，这个测试会立刻失败。
func TestDecisionWithEmotionKeepsConsistency(t *testing.T) {
	original := PlannerDecision{
		Action:  "reply",
		Reason:  "user asked something",
		Emotion: EmotionHappy,
		Mood:    MoodCheer,
		Valence: 0.9,
		Energy:  0.9,
	}
	applied := EmotionVector{
		Emotion: EmotionSad, Mood: MoodComfort,
		Valence: -0.2, Arousal: 0.4, Dominance: -0.3,
	}

	got := decisionWithEmotion(original, applied)

	// 情绪字段必须被覆盖为实际生效的值。
	if got.Emotion != EmotionSad || got.Mood != MoodComfort {
		t.Fatalf("情绪字段应写回平滑值，实际 %q/%q", got.Emotion, got.Mood)
	}
	if math.Abs(got.Valence-(-0.2)) > 1e-9 || math.Abs(got.Energy-0.4) > 1e-9 || math.Abs(got.Dominance-(-0.3)) > 1e-9 {
		t.Fatalf("VAD 应写回平滑值，实际 %v/%v/%v", got.Valence, got.Energy, got.Dominance)
	}
	// 决策信息不能被情绪逻辑破坏。
	if got.Action != original.Action || got.Reason != original.Reason {
		t.Fatalf("不应改动决策字段，实际 action=%q reason=%q", got.Action, got.Reason)
	}

	// 写回后，Replyer 看到的表演指令应与前端表情一致。
	if directive := formatEmotionDirective(got); directive == "" || !containsAll(directive, "sad") {
		t.Fatalf("表演指令应反映写回后的情绪，实际 %q", directive)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}

// TestEmotionStateFirstUpdateAdoptsSuggestion 首次更新（无历史）应直接采用建议值。
func TestEmotionStateFirstUpdateAdoptsSuggestion(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	got := store.Update("c1", EmotionVector{
		Emotion: "happy", Mood: "cheer", Valence: 0.8, Arousal: 0.9, Dominance: 0.4,
	}, now)

	if got.Emotion != "happy" || got.Mood != "cheer" {
		t.Fatalf("首次应直接采用建议的标签与基调，实际 %q/%q", got.Emotion, got.Mood)
	}
	if math.Abs(got.Valence-0.8) > 1e-9 {
		t.Fatalf("首次应保留建议效价 0.8，实际 %v", got.Valence)
	}
}

// TestEmotionStateInertiaPreventsFlip 是情绪惯性的核心断言：
// 单轮的情绪反转必须被抑制（不能从 +0.9 一步跳到 -0.9）。
func TestEmotionStateInertiaPreventsFlip(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Mood: "cheer", Valence: 0.9, Arousal: 0.8}, now)

	// 紧接着（无衰减时间）给出完全相反的建议。
	got := store.Update("c1", EmotionVector{Emotion: "sad", Mood: "comfort", Valence: -0.9, Arousal: 0.3}, now)

	// 预期：从 +0.9 只走 35% → 0.9 + (-0.9-0.9)*0.35 = 0.27。
	// 关键点是它**没有一步跨到负值**（那才是"变脸"），仍然偏向正区间。
	want := 0.9 + (-0.9-0.9)*emotionInertia
	if math.Abs(got.Valence-want) > 1e-6 {
		t.Fatalf("位移应恰为惯性比例：want %v got %v", want, got.Valence)
	}
	if got.Valence < 0 {
		t.Fatalf("单轮不应穿过中性点翻到负值，实际效价 %v", got.Valence)
	}
	if got.Valence >= 0.9 {
		t.Fatalf("应确实向建议值移动了，实际仍为 %v", got.Valence)
	}
}

// TestEmotionStateLabelHeldDuringHoldWindow 刚切换过的标签在同一时间点不应再次抖动。
func TestEmotionStateLabelHeldDuringHoldWindow(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Mood: "cheer", Valence: 0.9}, now)

	// 保持期内给出另一个标签，但效价变化不明显 → 应沿用旧标签。
	got := store.Update("c1", EmotionVector{Emotion: "thinking", Mood: "curious", Valence: 0.75}, now.Add(3*time.Second))
	if got.Emotion != "happy" {
		t.Fatalf("保持期内不应抖动标签，实际切到 %q", got.Emotion)
	}
}

// TestEmotionStateLabelSwitchesAfterHold 超过保持时长后应允许切换标签。
func TestEmotionStateLabelSwitchesAfterHold(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Mood: "cheer", Valence: 0.9}, now)

	still := store.Update("c1", EmotionVector{Emotion: "thinking", Mood: "curious", Valence: 0.8}, now)
	if still.Emotion != "happy" {
		t.Fatalf("保持期内应沿用旧标签，实际 %q", still.Emotion)
	}
	later := store.Update("c1", EmotionVector{Emotion: "thinking", Mood: "curious", Valence: 0.8},
		now.Add(emotionLabelHold+time.Second))
	if later.Emotion != "thinking" {
		t.Fatalf("超过保持时长应允许切换，实际 %q", later.Emotion)
	}
	if later.Mood != "curious" {
		t.Fatalf("切换标签时 mood 应跟随建议值，实际 %q", later.Mood)
	}
}

// TestEmotionStateStrongShiftBypassesHold 效价变化足够大时，即使仍在保持期也应允许换表情
// （避免"刚被凶了还在笑"这类明显违和的滞后）。
func TestEmotionStateStrongShiftBypassesHold(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Mood: "cheer", Valence: 0.9}, now)

	// 立刻给出强烈负面：虽在保持期内，但效价位移远超 margin，应允许切换。
	got := store.Update("c1", EmotionVector{Emotion: "sad", Mood: "comfort", Valence: -1.0}, now.Add(time.Second))
	if got.Emotion != "sad" {
		t.Fatalf("强烈情绪变化应允许切换标签，实际 %q（valence=%v）", got.Emotion, got.Valence)
	}
}

// TestEmotionStateDecaysTowardBaseline 长时间无互动后，情绪应明显向基线平复。
func TestEmotionStateDecaysTowardBaseline(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Mood: "cheer", Valence: 1.0, Arousal: 1.0}, now)

	// 一个半衰期后再互动，且本轮建议值也是中性的 → 应接近基线。
	got := store.Update("c1", EmotionVector{Emotion: "neutral", Mood: "calm", Valence: 0, Arousal: 0.5},
		now.Add(emotionDecayHalfLife))

	if got.Valence > 0.5 {
		t.Fatalf("经过一个半衰期后效价应明显回落，实际 %v", got.Valence)
	}
	if math.Abs(got.Arousal-arousalBase) > 0.25 {
		t.Fatalf("唤醒度应向基线 %v 平复，实际 %v", arousalBase, got.Arousal)
	}
}

// TestEmotionStatePerConversationIsolated 不同会话的情绪互不影响。
func TestEmotionStatePerConversationIsolated(t *testing.T) {
	store := newEmotionStateStore()
	now := time.Now()
	store.Update("c1", EmotionVector{Emotion: "happy", Valence: 0.9}, now)
	store.Update("c2", EmotionVector{Emotion: "sad", Valence: -0.9}, now)

	c1, ok := store.Current("c1")
	if !ok || c1.Emotion != "happy" {
		t.Fatalf("c1 情绪应独立保持，实际 %+v", c1)
	}
	c2, ok := store.Current("c2")
	if !ok || c2.Emotion != "sad" {
		t.Fatalf("c2 情绪应独立保持，实际 %+v", c2)
	}
	if _, ok := store.Current("c3"); ok {
		t.Fatalf("未出现过的会话不应有情绪状态")
	}
}

// TestEmotionStateClampsOutOfRange 越界输入应被钳制在合法区间。
func TestEmotionStateClampsOutOfRange(t *testing.T) {
	store := newEmotionStateStore()
	got := store.Update("c1", EmotionVector{Emotion: "bogus", Mood: "bogus", Valence: 5, Arousal: -3, Dominance: 9}, time.Now())
	if got.Valence != 1 {
		t.Fatalf("valence 应钳制到 1，实际 %v", got.Valence)
	}
	if got.Arousal != 0 {
		t.Fatalf("arousal 应钳制到 0，实际 %v", got.Arousal)
	}
	if got.Dominance != 1 {
		t.Fatalf("dominance 应钳制到 1，实际 %v", got.Dominance)
	}
	// 非法标签应回退到白名单默认值。
	if got.Emotion != EmotionNeutral || got.Mood != MoodCalm {
		t.Fatalf("非法标签应回退，实际 %q/%q", got.Emotion, got.Mood)
	}
}
