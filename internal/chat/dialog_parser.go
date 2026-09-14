package chat

import (
	"encoding/json"
	"strconv"
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

	// thought 是本轮捕获到的「内心独白」（没说出口的一句话，见 thought.go）。
	// 它必须与 dialog 分开存放：dialog 会进 TTS 队列被念出来，thought 不会。
	thought string
	// thoughtTaken 表示已从 accumulated 中读到过完整的 thought 字段（无论清洗后是否为空），
	// 避免后续每个 chunk 都重复扫描。
	thoughtTaken bool
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
	// 必须在 drain 之前捕获 thought：drain 会丢弃第一个台词对象之前的包装前缀
	// （含 {"thought":"…","dialog":[ ），此后缓冲区里就再也找不到它了。
	p.captureThought()
	return p.drain()
}

// captureThought 尝试从累计原文中取出 thought 字段（字符串闭合后才算完整）。
func (p *dialogStreamParser) captureThought() {
	if p.thoughtTaken {
		return
	}
	raw, complete := extractThoughtField(p.accumulated)
	if !complete {
		return
	}
	p.thoughtTaken = true
	p.thought = normalizeThought(raw)
}

// takeThought 返回并消费已捕获的独白（每轮回复最多一句）。
func (p *dialogStreamParser) takeThought() string {
	text := p.thought
	p.thought = ""
	return text
}

// extractThoughtField 从（可能尚未闭合的）JSON 文本中增量提取 "thought" 字段的字符串值。
//
// 为什么需要这样一个"手写"提取器：dialogStreamParser 为了逐句流式下发，
// 会丢弃第一个完整台词对象之前的所有文本；而 thought 恰好在前。用 json.Unmarshal 无法
// 处理未闭合的片段，所以这里按字节扫描值区间，遇到收尾引号才算完整。
//
// 返回 (值, 是否完整)。未找到该字段、或字符串尚未闭合时返回 ("", false)。
func extractThoughtField(text string) (string, bool) {
	const key = `"thought"`
	idx := strings.Index(text, key)
	if idx < 0 {
		return "", false
	}
	rest := text[idx+len(key):]

	// 跳过 key 与值之间的空白与冒号（容忍 `"thought" : "…"` 这类排版）。
	i := 0
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == '\r' || rest[i] == ':') {
		i++
	}
	if i >= len(rest) || rest[i] != '"' {
		// 值还没开始（或不是字符串）：等下一个 chunk。
		return "", false
	}
	i++ // 跳过值的起始引号

	var b strings.Builder
	for ; i < len(rest); i++ {
		ch := rest[i]
		switch ch {
		case '"':
			return b.String(), true
		case '\\':
			if i+1 >= len(rest) {
				return "", false // 转义序列被 chunk 切断：等后续
			}
			i++
			switch rest[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case '/':
				b.WriteByte('/')
			case 'u':
				// \uXXXX：四位十六进制未到齐就等下一个 chunk，避免解析出半个字符。
				if i+4 >= len(rest) {
					return "", false
				}
				code, err := strconv.ParseUint(rest[i+1:i+5], 16, 32)
				if err != nil {
					return "", false
				}
				b.WriteRune(rune(code))
				i += 4
			default:
				// 非法转义（模型常在此写 Windows 路径）：按字面反斜杠处理，
				// 与 fixInvalidJSONEscapes 的容错口径保持一致。
				b.WriteByte('\\')
				b.WriteByte(rest[i])
			}
		default:
			b.WriteByte(ch)
		}
	}
	// 扫到尾都没遇到收尾引号：字符串还没流完。
	return "", false
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
