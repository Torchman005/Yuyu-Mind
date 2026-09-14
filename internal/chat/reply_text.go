package chat

import (
	"regexp"
	"strings"
)

// 本文件集中放置「回复文本后处理」的纯函数：清洗模型输出、按句切分、长度截断。
// 它们同时被流式回复路径（stream_reply.go）与测试使用，因此独立于任何 Service 类型。
//
// 历史上这些函数与 SendService.SendGuidedReply 放在一起；该同步发送路径已被
// streamReply（边生成边 emit + 持久化）取代并不再被调用，故一并移除，
// 仅保留这些仍被复用的纯函数。

// 舞台指示（动作/心理/神态描写）清理。
//
// 设计取舍（见 docs/REALISM-ANALYSIS.md P0-3）：原先用一个宽泛的行内规则删除**所有**括号内容，
// 会把真人口语里的插入语（「那个（我是说）…」「（大概）三点吧」）一并删掉，令回复过度书面化。
// 现改为保守策略，只清理明确是"舞台提示"的部分：
//   - 整行只有括号内容（单独一行「（笑）」）→ 删整行；
//   - 行首的括号提示（「（叹气）算了」）→ 去提示、留正文；
//   - 行内括号保留，避免误删口语插入语。
var stageLinePattern = regexp.MustCompile(`(?m)^\s*[\(（\[【][^\n]{0,80}[\)）\]】]\s*$`)
var leadingStagePattern = regexp.MustCompile(`^\s*[\(（\[【][^\)）\]】\n]{0,20}[\)）\]】]\s*`)

// postprocessReply 清洗模型输出：去掉行首/整行的舞台指示，规整换行与空白；
// 但保留行内括号（口语插入语）。这是「只输出说出来的话」这一约束的最后一道保障。
func postprocessReply(raw string) string {
	text := strings.TrimSpace(raw)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = stageLinePattern.ReplaceAllString(text, "")

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 行首舞台提示可能连续出现多次，如「（叹气）（歪头）算了」。
		for {
			stripped := leadingStagePattern.ReplaceAllString(trimmed, "")
			if stripped == trimmed {
				break
			}
			trimmed = strings.TrimSpace(stripped)
		}
		lines[i] = trimmed
	}
	return strings.TrimSpace(strings.Join(nonEmpty(lines), "\n"))
}

// splitReply 按句子边界把长回复切成多条短消息；无标点时按 maxRunes 强制切分。
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

// trimSentencePart 去掉片段末尾的逗号类字符，避免 GPT-SoVITS 对「带尾随逗号的短片段」过度切分而哼声
// （如「这一声主人，」会被哼成轻哼；去掉尾随逗号成「这一声主人」则能正常读出）。
func trimSentencePart(part string) string {
	return strings.TrimRight(strings.TrimSpace(part), "，、；,;")
}

// nonEmpty 去掉空白项并 trim 每项。
func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

// —— 口语冗余 / 口误自纠正（见 docs/REALISM-ANALYSIS.md P1）——
//
// 为什么要做：模型生成的文本永远"干净、完整、有条理"，这正是机器感的重要来源。
// 真人说话有填充词（"嗯""那个"）、会重复字、偶尔说错再更正。语音场景下这比"错别字"更贴切——
// 用户**听**到的不是文字，而是停顿、语气和口误。
//
// 落地方式：用代码注入而不是指望模型自觉（频率可控、可测试）。
// 概率刻意很低：偶尔出现才像真人，每句都有就变成另一种机械感。

const (
	// disfluencyFillerChance 是"句首插入填充词"的概率。
	disfluencyFillerChance = 0.12
	// disfluencyRepeatChance 是"首个字重复一次"的概率（"我我"这类）。
	disfluencyRepeatChance = 0.06
)

// disfluencyFillers 是自然的口语填充词。带省略号/逗号，让 TTS 自然产生一点停顿。
var disfluencyFillers = []string{"嗯…", "诶，", "那个…", "唔…", "啊，", "嘛，"}

// applyDisfluency 按概率给一句话注入口语特征。
//
// r1/r2 是 [0,1) 的随机数，由调用方注入以便测试确定性。以下情况不注入：
//   - 句子过短（加填充词会很怪，如"嗯…好"）；
//   - 已经以标点/填充词开头（避免叠成"嗯…嗯…"）。
func applyDisfluency(sentence string, r1, r2 float64) string {
	trimmed := strings.TrimSpace(sentence)
	if trimmed == "" {
		return sentence
	}
	runes := []rune(trimmed)
	if len(runes) < 4 {
		return sentence
	}
	// 已带前置标点/省略号时不再加，避免连续停顿。
	if strings.ContainsAny(string(runes[0]), "，,。.…!！?？~～") {
		return sentence
	}

	out := trimmed
	fillerUsed := false

	if r1 < disfluencyFillerChance {
		idx := int(r2 * float64(len(disfluencyFillers)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(disfluencyFillers) {
			idx = len(disfluencyFillers) - 1
		}
		out = disfluencyFillers[idx] + out
		fillerUsed = true
	}

	// 重复首个字（"我我"）——仅在没插填充词、且首字是常见人称/指示/判断词时做，
	// 以免破坏语义（例如"算法"重复成"算算法"）。
	if !fillerUsed && r2 < disfluencyRepeatChance && len(runes) >= 6 {
		switch runes[0] {
		case '我', '你', '他', '她', '这', '那', '就', '是':
			out = string(runes[0]) + out
		}
	}
	return out
}
