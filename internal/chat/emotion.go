package chat

// 本文件定义前后端共享的「情绪 Schema」。
// 这是 LLM 结构化情绪输出的唯一契约：允许值即白名单，非法值统一回退。
// 前端 Live2DStage 应消费同一套取值（见 frontend/src/components/Live2DStage.tsx）。

const (
	// 表情（expression）：驱动 Live2D 的 expression 映射。
	EmotionNeutral   = "neutral"
	EmotionHappy     = "happy"
	EmotionFocused   = "focused"
	EmotionThinking  = "thinking"
	EmotionSad       = "sad"
	EmotionSurprised = "surprised"

	// 情绪基调（mood）：驱动表演参数（微笑/眉毛/嘴型/能量）。
	MoodCalm      = "calm"
	MoodCheer     = "cheer"
	MoodCurious   = "curious"
	MoodConfident = "confident"
	MoodComfort   = "comfort"
	MoodSurprised = "surprised"
	MoodPlayful   = "playful"

	// 手势（gesture）：短期动作短语。
	GestureNone        = "none"
	GestureBounce      = "bounce"
	GestureTilt        = "tilt"
	GestureLean        = "lean"
	GesturePlayfulSway = "playfulSway"
	GestureSurprisePop = "surprisePop"
	GestureComfortNod  = "comfortNod"

	// 手部动作（hand）：抬臂等。
	HandNone  = "none"
	HandLeft  = "left"
	HandRight = "right"
	HandBoth  = "both"
)

var allowedEmotions = newStringSet(
	EmotionNeutral, EmotionHappy, EmotionFocused, EmotionThinking, EmotionSad, EmotionSurprised,
)

var allowedMoods = newStringSet(
	MoodCalm, MoodCheer, MoodCurious, MoodConfident, MoodComfort, MoodSurprised, MoodPlayful,
)

var allowedGestures = newStringSet(
	GestureNone, GestureBounce, GestureTilt, GestureLean, GesturePlayfulSway, GestureSurprisePop, GestureComfortNod,
)

var allowedHands = newStringSet(
	HandNone, HandLeft, HandRight, HandBoth,
)

func newStringSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// NormalizeEmotion 归一化表情取值，非法值回退 neutral。
func NormalizeEmotion(raw string) string {
	if allowedEmotions[raw] {
		return raw
	}
	return EmotionNeutral
}

// NormalizeMood 归一化情绪基调，非法值回退 calm。
func NormalizeMood(raw string) string {
	if allowedMoods[raw] {
		return raw
	}
	return MoodCalm
}

// NormalizeGesture 归一化手势，非法值回退 none。
func NormalizeGesture(raw string) string {
	if allowedGestures[raw] {
		return raw
	}
	return GestureNone
}

// NormalizeHand 归一化手部动作，非法值回退 none。
func NormalizeHand(raw string) string {
	if allowedHands[raw] {
		return raw
	}
	return HandNone
}

// ClampEnergy 将能量值限制到 [0,1]，非法值回退 0。
func ClampEnergy(energy float64) float64 {
	if energy < 0 {
		return 0
	}
	if energy > 1 {
		return 1
	}
	return energy
}
