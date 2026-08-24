package chat

import "testing"

func TestDialogStreamParser_NonStreamingWrapper(t *testing.T) {
	// 非流式（单 chunk 完整包装）→ 解包 dialog 数组为多个 item。
	p := newDialogStreamParser()
	items := p.feed(`{"dialog":[{"speech":"主人呀～","emotion":"happy","mood":"cheer","energy":0.9},{"speech":"我在这呢","emotion":"neutral"}]}`)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	if items[0].Speech != "主人呀～" || items[0].Emotion != EmotionHappy || items[0].Mood != "cheer" {
		t.Fatalf("item0 mismatch: %+v", items[0])
	}
	if p.yieldedItems != 2 {
		t.Fatalf("expected yielded=2, got %d", p.yieldedItems)
	}
}

func TestDialogStreamParser_StreamingItems(t *testing.T) {
	// 流式：内部对象逐个闭合，逐条 yield。
	p := newDialogStreamParser()
	var all []DialogItem
	chunks := []string{`{"dialog":[{"speech":"第一句","emotion":"happy"}`, `,{"speech":"第二句","emotion":"sad"`, `}]}`}
	for _, c := range chunks {
		all = append(all, p.feed(c)...)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(all), all)
	}
	if all[0].Speech != "第一句" || all[0].Emotion != EmotionHappy {
		t.Fatalf("item0 mismatch: %+v", all[0])
	}
	if all[1].Speech != "第二句" || all[1].Emotion != EmotionSad {
		t.Fatalf("item1 mismatch: %+v", all[1])
	}
}

func TestDialogStreamParser_FlatTextFallback(t *testing.T) {
	// 模型直接给 flat text：解析不出对象，accumulated 保留原文供回退。
	p := newDialogStreamParser()
	items := p.feed("主人就是您呀～")
	if len(items) != 0 {
		t.Fatalf("expected 0 items for flat text, got %+v", items)
	}
	if p.accumulated != "主人就是您呀～" {
		t.Fatalf("accumulated mismatch: %q", p.accumulated)
	}
	if p.yieldedItems != 0 {
		t.Fatalf("expected yielded=0, got %d", p.yieldedItems)
	}
}

func TestDialogStreamParser_Malformed(t *testing.T) {
	p := newDialogStreamParser()
	items := p.feed(`{"dialog":[{"speech":"x"`) // 内部对象未闭合 → 无完整对象
	if len(items) != 0 {
		t.Fatalf("expected 0 items for incomplete json, got %+v", items)
	}
	if p.parseFailure != 0 {
		t.Fatalf("expected no parse failure for incomplete buffer, got %d", p.parseFailure)
	}
}

func TestCompleteJSONObjectSpan(t *testing.T) {
	start, end, ok := completeJSONObjectSpan(`{"a":{"b":1}} tail`)
	if !ok || start != 0 || end != 13 {
		t.Fatalf("span mismatch: start=%d end=%d ok=%v", start, end, ok)
	}
	if start, end, ok := completeJSONObjectSpan(`no json`); ok || start != 0 || end != 0 {
		t.Fatalf("expected no span, got start=%d end=%d ok=%v", start, end, ok)
	}
}
