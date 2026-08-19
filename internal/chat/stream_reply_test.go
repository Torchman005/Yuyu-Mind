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
}
