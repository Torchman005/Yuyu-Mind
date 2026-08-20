package chat

import "testing"

func TestShouldSkipPlanner(t *testing.T) {
	cases := map[string]bool{
		"你好":                true,  // 简单问候
		"今天好累啊":             true,  // 简单情绪表达
		"哈哈，太逗了":            true,  // 简单闲聊
		"帮我写一个 Hello World": false, // 任务
		"你还记得我之前说的吗":        false, // 记忆
		"查一下今天的天气":          false, // 工具查询
		"生成一份项目总结文档":        false, // 任务
		"":                  false, // 空消息不走快速通道
	}
	for content, want := range cases {
		if got := shouldSkipPlanner(content); got != want {
			t.Fatalf("shouldSkipPlanner(%q) = %v, want %v", content, got, want)
		}
	}

	// 超长消息（>24 字符）即使无信号也保守走 Planner。
	long := "这是一个非常长的消息，包含了很多内容和上下文信息，为了安全起见应该走 Planner 完整决策流程"
	if shouldSkipPlanner(long) {
		t.Fatalf("long message should not skip planner")
	}
}

func TestInferEmotionFromText(t *testing.T) {
	cases := map[string]string{
		"太好了，谢谢！": EmotionHappy,
		"有点难过":    EmotionSad,
		"居然是这样！":  EmotionSurprised,
		"报错了":     EmotionThinking,
		"普通的一句话":  EmotionNeutral,
	}
	for text, want := range cases {
		if got := InferEmotionFromText(text); got != want {
			t.Fatalf("InferEmotionFromText(%q) = %q, want %q", text, got, want)
		}
	}
}
