package chat

import "strings"

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

// ClampEnergy 将能量值（≈唤醒 arousal）限制到 [0,1]，非法值回退 0。
func ClampEnergy(energy float64) float64 {
	if energy < 0 {
		return 0
	}
	if energy > 1 {
		return 1
	}
	return energy
}

// ClampValence 将效价（valence，消极↔积极）限制到 [-1,1]，非法值回退 0（中性）。
// 与 arousal/dominance 共同构成连续的 VAD 情绪空间（参考 soullink-emotion-sdk）。
func ClampValence(valence float64) float64 {
	if valence < -1 {
		return -1
	}
	if valence > 1 {
		return 1
	}
	return valence
}

// ClampDominance 将支配度（dominance，顺从↔自信）限制到 [-1,1]，非法值回退 0（中性）。
func ClampDominance(dominance float64) float64 {
	if dominance < -1 {
		return -1
	}
	if dominance > 1 {
		return 1
	}
	return dominance
}

// InferEmotionFromText 用简单关键词从文本推断离散表情（快速通道的兜底，非精确）。
// 只做保守的词组级判断，避免单字误命中；否则回退 neutral。
func InferEmotionFromText(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(text, "开心") || strings.Contains(text, "太好") || strings.Contains(text, "喜欢") ||
		strings.Contains(text, "哈哈") || strings.Contains(text, "谢谢") || strings.Contains(lower, "great"):
		return EmotionHappy
	case strings.Contains(text, "难过") || strings.Contains(text, "低落") || strings.Contains(text, "伤心") ||
		strings.Contains(text, "抱歉") || strings.Contains(lower, "sorry"):
		return EmotionSad
	case strings.Contains(text, "惊讶") || strings.Contains(text, "居然") || strings.Contains(text, "竟然") ||
		strings.Contains(text, "真的吗") || strings.Contains(lower, "surprise"):
		return EmotionSurprised
	case strings.Contains(text, "报错") || strings.Contains(text, "错误") || strings.Contains(text, "失败") ||
		strings.Contains(text, "代码") || strings.Contains(lower, "error"):
		return EmotionThinking
	default:
		return EmotionNeutral
	}
}
