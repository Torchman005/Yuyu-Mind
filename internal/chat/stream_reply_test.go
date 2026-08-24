package chat

import (
	"reflect"
	"testing"
)

func collectSentences(chunks []string, maxRunes int) []string {
	s := newStreamingSentencer(maxRunes)
	var parts []string
	for _, chunk := range chunks {
		parts = append(parts, s.feed(chunk)...)
	}
	parts = append(parts, s.flush()...)
	return parts
}

func TestStreamingSentencer(t *testing.T) {
	// 逐句按标点切分，跨 chunk 也能正确合并。
	got := collectSentences([]string{"你好", "。今天天气怎么", "样？还不错！"}, 90)
	want := []string{"你好。", "今天天气怎么样？", "还不错！"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sentence split mismatch:\n got=%v\nwant=%v", got, want)
	}

	// 无标点超长句强制按 maxRunes 切分。
	got = collectSentences([]string{"这是一句没有标点而且非常长的句子需要被强制切分"}, 8)
	if len(got) == 0 {
		t.Fatalf("expected forced split parts, got empty")
	}
	for _, part := range got {
		if len([]rune(part)) > 8 {
			t.Fatalf("part exceeds maxRunes: %q (%d runes)", part, len([]rune(part)))
		}
	}
	joined := ""
	for _, part := range got {
		joined += part
	}
	if joined != "这是一句没有标点而且非常长的句子需要被强制切分" {
		t.Fatalf("forced split lost content: %q", joined)
	}

	// 空输入 flush 不产出空串。
	if got := collectSentences([]string{""}, 90); len(got) != 0 {
		t.Fatalf("expected no parts for empty input, got %v", got)
	}

	// 仅强句界（。！？.!?\n）切分：整句作为一个 chunk 交给 GPT-SoVITS（配合 text_split_method=cut1 朗读更稳），
	// 避免应用侧把「主人」「那里」等过短片段单独下发而被打成哼声。
	got = collectSentences([]string{"主人就是您呀~要不，我为您泡杯温热的花茶？"}, 90)
	want = []string{"主人就是您呀~要不，我为您泡杯温热的花茶？"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full-sentence split mismatch:\n got=%v\nwant=%v", got, want)
	}

	// 含逗号的整句作为一个 chunk（不按逗号切分），交由 GPT-SoVITS 朗读。
	got = collectSentences([]string{"这一声主人，早就在心里念过千遍了。"}, 90)
	want = []string{"这一声主人，早就在心里念过千遍了。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("comma-in-sentence split mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestRefineSentenceEmotion(t *testing.T) {
	cases := []struct {
		part         string
		last         string
		want         string
	}{
		{"太好了，谢谢！", "neutral", EmotionHappy},   // 明显正面 → 下发
		{"普通的一句话", "happy", ""},               // 中性且与 last 无关 → 不下发（保持）
		{"有点难过呢", "neutral", EmotionSad},      // 明显负面 → 下发
		{"开心", "happy", ""},                    // 与 last 相同 → 不下发（避免重复）
		{"惊讶", "neutral", EmotionSurprised},    // 惊讶 → 下发
	}
	for _, c := range cases {
		if got := refineSentenceEmotion(c.part, c.last); got != c.want {
			t.Fatalf("refineSentenceEmotion(%q, %q) = %q, want %q", c.part, c.last, got, c.want)
		}
	}
}
