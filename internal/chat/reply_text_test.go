package chat

import (
	"strings"
	"testing"
)

// TestPostprocessReplyKeepsColloquial 覆盖 P0-3 引入的新语义：
// 舞台提示（整行括号 / 行首括号）必须清掉，而真人口语插入语（行内括号）必须保留。
//
// 早期实现用宽泛的行内规则删除所有括号内容，会把「那个（我是说）…」这类自然口语一起删掉，
// 导致回复过度书面化（见 docs/REALISM-ANALYSIS.md P0-3）。这个测试是那条取舍的回归防线。
func TestPostprocessReplyKeepsColloquial(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "整行舞台提示删除",
			raw:  "（笑）\n主人今天想做什么？",
			want: "主人今天想做什么？",
		},
		{
			name: "行首舞台提示去掉但保留正文",
			raw:  "（歪头）你在说什么呀",
			want: "你在说什么呀",
		},
		{
			name: "连续行首提示全部去掉",
			raw:  "（叹气）（歪头）算了，不说了",
			want: "算了，不说了",
		},
		{
			name: "行内口语插入语必须保留",
			raw:  "那个（我是说）明天下午三点吧",
			want: "那个（我是说）明天下午三点吧",
		},
		{
			name: "行内近似表达保留",
			raw:  "大概（也许）三点左右",
			want: "大概（也许）三点左右",
		},
		{
			name: "英文方括号行首提示同样清理",
			raw:  "[smile] 你好呀",
			want: "你好呀",
		},
		{
			name: "空行与多余空白规整",
			raw:  "第一句\n\n\n第二句",
			want: "第一句\n第二句",
		},
		{
			name: "纯舞台提示清空后返回空串",
			raw:  "（笑）\n（歪头）",
			want: "",
		},
		{
			name: "普通多句回复不受影响",
			raw:  "嗯，我在。要不要一起想想？",
			want: "嗯，我在。要不要一起想想？",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := postprocessReply(tc.raw); got != tc.want {
				t.Fatalf("postprocessReply(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestSplitReplyTrimsTrailingComma 覆盖按句切分与尾随逗号清理
// （尾随逗号会让 TTS 把短片段哼成轻哼，见 trimSentencePart 注释）。
func TestSplitReplyTrimsTrailingComma(t *testing.T) {
	parts := splitReply("这一声主人，今天过得怎么样？我很想你。", 8)
	if len(parts) < 2 {
		t.Fatalf("长句应被切分为多段，实际 %v", parts)
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			t.Fatalf("切分结果不应包含空片段：%v", parts)
		}
		if trimmed := strings.TrimRight(p, "，、；,;"); trimmed != p {
			t.Fatalf("片段尾随逗号应被去掉：%q", p)
		}
	}
}

// TestApplyDisfluencyInjectsFiller 验证低随机值时注入填充词（口语停顿）。
func TestApplyDisfluencyInjectsFiller(t *testing.T) {
	got := applyDisfluency("我今天去看了那部电影", 0.0, 0.0)
	if got == "我今天去看了那部电影" {
		t.Fatalf("r 极小时应注入填充词，实际未变化：%q", got)
	}
	matched := false
	for _, f := range disfluencyFillers {
		if strings.HasPrefix(got, f) {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("注入的应是已知填充词，实际 %q", got)
	}
	if !strings.Contains(got, "我今天去看了那部电影") {
		t.Fatalf("注入不应破坏原句内容，实际 %q", got)
	}
}

// TestApplyDisfluencyNoInjectionWhenHigh 高随机值时原样返回（不能每句都口语化）。
func TestApplyDisfluencyNoInjectionWhenHigh(t *testing.T) {
	const sentence = "我今天去看了那部电影"
	if got := applyDisfluency(sentence, 0.99, 0.99); got != sentence {
		t.Fatalf("高随机值不应改动句子，实际 %q", got)
	}
}

// TestApplyDisfluencySkipsShortAndPunctuated 短句 / 已带前置标点的句子不注入
// （避免"嗯…好"这种怪异结果，以及"嗯…嗯…"叠成两段停顿）。
func TestApplyDisfluencySkipsShortAndPunctuated(t *testing.T) {
	cases := []string{"好", "嗯嗯", "，今天天气不错", "…然后呢", "。算了"}
	for _, c := range cases {
		if got := applyDisfluency(c, 0.0, 0.0); got != c {
			t.Fatalf("(%q) 不应注入，实际 %q", c, got)
		}
	}
}

// TestApplyDisfluencyRepeatsFirstChar 验证"我我"式重复只作用于安全的首字。
func TestApplyDisfluencyRepeatsFirstChar(t *testing.T) {
	// 首字是"我"且长度足够、且填充词未命中（r1 高、r2 低）→ 重复首字。
	got := applyDisfluency("我觉得这件事有点奇怪", 0.99, 0.0)
	if !strings.HasPrefix(got, "我我") {
		t.Fatalf("应以重复的「我我」开头，实际 %q", got)
	}

	// 首字是普通名词时不应重复（"算算法"会破坏语义）。
	if got := applyDisfluency("算法这块我还得再看看", 0.99, 0.0); strings.HasPrefix(got, "算算") {
		t.Fatalf("不应重复普通名词首字，实际 %q", got)
	}
}
