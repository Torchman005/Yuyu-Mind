package chat

import (
	"encoding/json"
	"strings"
)

// DialogItem 是结构化回复中「一句台词 + 该句自己的情绪/动作参数」。
// 对齐 Shinsekai：模型流式输出 {"dialog":[{speech,emotion,mood,energy,valence,dominance,gesture,hand}]}，
// 使「表情/动作随台词走」（每句一个表情），而非仅由 Planner 一次性给出整体情绪。
type DialogItem struct {
	Speech    string  `json:"speech"`
	Emotion   string  `json:"emotion"`
	Mood      string  `json:"mood"`
	Energy    float64 `json:"energy"`
	Valence   float64 `json:"valence"`
	Dominance float64 `json:"dominance"`
	Gesture   string  `json:"gesture"`
	Hand      string  `json:"hand"`
}

// dialogStreamParser 从 LLM 流式输出中按完整 JSON 对象切分并解析为 DialogItem。
// 与流式/非流式（单 chunk）均可复用；完整原文保存在 accumulated 供 flat-text 回退。
type dialogStreamParser struct {
	buffer       string
	accumulated  string
	yieldedItems int
	parseFailure int
	lastError    string
}

func newDialogStreamParser() *dialogStreamParser {
	return &dialogStreamParser{}
}

// feed 把新到达的文本并入缓冲区，并对其中已完整的 JSON 逐条 yield。
func (p *dialogStreamParser) feed(chunk string) []DialogItem {
	if chunk != "" {
		p.buffer += chunk
		p.accumulated += chunk
	}
	return p.drain()
}

// drain 解析缓冲区中所有已完整的 JSON 对象。
func (p *dialogStreamParser) drain() []DialogItem {
	items := []DialogItem{}
	for {
		start, end, found := completeJSONObjectSpan(p.buffer)
		if !found {
			break
		}
		objStr := p.buffer[start:end]
		p.buffer = strings.TrimSpace(p.buffer[end:])
		if !p.handleObject(objStr, &items) {
			break
		}
	}
	return items
}

// handleObject 处理一个已完整的 JSON 对象：可能是 {"dialog":[...]} 包装本身，
// 也可能是单个台词对象（流式时每个内部对象先于包装闭合）。返回 next 是否继续。
func (p *dialogStreamParser) handleObject(objStr string, items *[]DialogItem) bool {
	var wrapped struct {
		Dialog []DialogItem `json:"dialog"`
	}
	if err := json.Unmarshal([]byte(objStr), &wrapped); err == nil && len(wrapped.Dialog) > 0 {
		for _, it := range wrapped.Dialog {
			p.yieldItem(it, items)
		}
		return true
	}

	var single DialogItem
	if err := json.Unmarshal([]byte(objStr), &single); err == nil && strings.TrimSpace(single.Speech) != "" {
		p.yieldItem(single, items)
		return true
	}

	p.parseFailure++
	p.lastError = objStr
	return true
}

// yieldItem 归一化并收集一个对话项（跳过空格文本）。
func (p *dialogStreamParser) yieldItem(it DialogItem, items *[]DialogItem) {
	if strings.TrimSpace(it.Speech) == "" {
		return
	}
	it.Speech = strings.TrimSpace(it.Speech)
	it.Emotion = NormalizeEmotion(it.Emotion)
	it.Mood = NormalizeMood(it.Mood)
	it.Gesture = NormalizeGesture(it.Gesture)
	it.Hand = NormalizeHand(it.Hand)
	it.Energy = ClampEnergy(it.Energy)
	it.Valence = ClampValence(it.Valence)
	it.Dominance = ClampDominance(it.Dominance)
	p.yieldedItems++
	*items = append(*items, it)
}

// completeJSONObjectSpan 返回文本中第一个完整的顶层 JSON 对象区间 [start,end) 与是否找到。
// 扫描器忽略 JSON 字符串内的花括号并支持嵌套；若前面有损坏的 { 一直不闭合，
// 后续候选起点仍会被考虑，以便从坏 JSON 或多余前言中恢复。
func completeJSONObjectSpan(text string) (int, int, bool) {
	starts := []int{}
	for i, ch := range text {
		if ch == '{' {
			starts = append(starts, i)
		}
	}
	for _, start := range starts {
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(text); i++ {
			ch := text[i]
			if inString {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return start, i + 1, true
				}
				if depth < 0 {
					break
				}
			}
		}
	}
	return 0, 0, false
}
