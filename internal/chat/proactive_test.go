package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// scriptedModel 按调用顺序依次返回预设内容，用于验证主动搭话的两段式流程。
// 第 n 次 Generate 返回 replies[n]；超出后返回最后一个（便于断言"只调用了预期次数"）。
type scriptedModel struct {
	replies []string
	errs    []error
	calls   int
	inputs  [][]*schema.Message
}

func (m *scriptedModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	idx := m.calls
	m.calls++
	m.inputs = append(m.inputs, input)
	if idx < len(m.errs) && m.errs[idx] != nil {
		return nil, m.errs[idx]
	}
	if len(m.replies) == 0 {
		return &schema.Message{Role: schema.Assistant, Content: ""}, nil
	}
	if idx >= len(m.replies) {
		idx = len(m.replies) - 1
	}
	return &schema.Message{Role: schema.Assistant, Content: m.replies[idx]}, nil
}

func (m *scriptedModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not used in proactive path")
}

// TestBuildProactiveTopicCandidates 验证候选话题的选取规则：
// 只取用户发言、从最近往前、过短过滤、去重。
func TestBuildProactiveTopicCandidates(t *testing.T) {
	history := []*schema.Message{
		{Role: schema.System, Content: "system"},
		{Role: schema.User, Content: "我最近在学做菜"},          // 较旧
		{Role: schema.Assistant, Content: "要做饭了吗"},       // 助手发言不算话题
		{Role: schema.User, Content: "嗯"},                // 过短
		{Role: schema.User, Content: "今天面试好紧张啊"},         // 最新
		{Role: schema.User, Content: "今天面试好紧张啊"},         // 重复
	}

	got := buildProactiveTopicCandidates(history, 5)
	if len(got) != 2 {
		t.Fatalf("应只取到 2 条有效用户话题，实际 %d：%v", len(got), got)
	}
	// 最新的话题必须排在最前。
	if got[0] != "今天面试好紧张啊" {
		t.Fatalf("最新话题应排第一，实际 %q", got[0])
	}
	if got[1] != "我最近在学做菜" {
		t.Fatalf("次新话题顺序错误，实际 %q", got[1])
	}
	for _, c := range got {
		if c == "要做饭了吗" {
			t.Fatalf("助手自己的发言不应成为话题候选")
		}
	}
}

// TestBuildProactiveTopicCandidatesRespectsLimit 验证数量上限与空历史。
func TestBuildProactiveTopicCandidatesRespectsLimit(t *testing.T) {
	if got := buildProactiveTopicCandidates(nil, 5); len(got) != 0 {
		t.Fatalf("空历史不应产生候选，实际 %v", got)
	}
	history := make([]*schema.Message, 0, 8)
	for i := 0; i < 8; i++ {
		history = append(history, &schema.Message{Role: schema.User, Content: "话题内容" + string(rune('a'+i))})
	}
	if got := buildProactiveTopicCandidates(history, 3); len(got) != 3 {
		t.Fatalf("应遵守 limit=3，实际 %d", len(got))
	}
}

// TestPickProactiveTopicParsesNumber 验证把模型的"编号"答案映射回候选。
func TestPickProactiveTopicParsesNumber(t *testing.T) {
	s := &Service{}
	candidates := []string{"第一件事", "第二件事", "第三件事"}

	m := &scriptedModel{replies: []string{"2"}}
	got, err := s.pickProactiveTopic(context.Background(), m, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "第二件事" {
		t.Fatalf("应选中第二件事，实际 %q", got)
	}
}

// TestPickProactiveTopicHandlesNone 模型认为"不该开口"时返回空串。
func TestPickProactiveTopicHandlesNone(t *testing.T) {
	s := &Service{}
	for _, answer := range []string{"none", "None", "", "NONE"} {
		m := &scriptedModel{replies: []string{answer}}
		got, err := s.pickProactiveTopic(context.Background(), m, []string{"一件事"})
		if err != nil {
			t.Fatalf("answer=%q 不应报错：%v", answer, err)
		}
		if got != "" {
			t.Fatalf("answer=%q 应判定为不开口，实际 %q", answer, got)
		}
	}
}

// TestPickProactiveTopicHandlesGarbage 模型不按格式回答时保守处理（不随机猜话题）。
func TestPickProactiveTopicHandlesGarbage(t *testing.T) {
	s := &Service{}
	for _, answer := range []string{"我觉得第三件比较好", "abc", "99"} {
		m := &scriptedModel{replies: []string{answer}}
		got, err := s.pickProactiveTopic(context.Background(), m, []string{"一", "二", "三"})
		if err != nil {
			t.Fatalf("answer=%q 不应报错：%v", answer, err)
		}
		// 说明：这里不断言必须为空（"我觉得第三件"含数字 3，会被解析），
		// 只保证不会越界、且不会 panic。
		if got != "" {
			found := false
			for _, c := range []string{"一", "二", "三"} {
				if c == got {
					found = true
				}
			}
			if !found {
				t.Fatalf("answer=%q 解析出了不在候选内的结果 %q", answer, got)
			}
		}
	}
}

// TestGenerateProactiveSkipsWhenTooSoon 验证"距上次主动发言太近"时不打扰（且不调用模型）。
func TestGenerateProactiveSkipsWhenTooSoon(t *testing.T) {
	s, convID := newInterruptionService(t)
	// 刚说过话：应直接跳过，不产生任何 LLM 调用。
	got, err := s.GenerateProactive(context.Background(), convID, "pet-idle",
		[]*schema.Message{{Role: schema.User, Content: "今天面试好紧张"}}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Skipped || got.Speech != "" {
		t.Fatalf("距上次太近应跳过，实际 %+v", got)
	}
}

// TestGenerateProactiveSkipsWithoutCandidates 没有可聊素材时跳过，而不是硬凑。
func TestGenerateProactiveSkipsWithoutCandidates(t *testing.T) {
	s, convID := newInterruptionService(t)
	// 历史里只有助手发言/过短内容 → 无候选，应在调用模型前就跳过。
	history := []*schema.Message{
		{Role: schema.Assistant, Content: "我先说点什么"},
		{Role: schema.User, Content: "嗯"},
	}
	got, err := s.GenerateProactive(context.Background(), convID, "pet-idle", history,
		time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Skipped {
		t.Fatalf("无候选话题应跳过，实际 %+v", got)
	}
}

// TestWriteProactiveSpeechUsesPersonaAndCleansOutput 验证生成段会带上人设、
// 并对输出做清洗（去掉可能残留的舞台提示）。
func TestWriteProactiveSpeechUsesPersonaAndCleansOutput(t *testing.T) {
	s, convID := newInterruptionService(t)
	s.cfg.Chat.Persona = "你是一只爱撒娇的猫娘"
	s.cfg.Chat.StyleNotes = "说话简短"
	s.emotions = newEmotionStateStore()

	m := &scriptedModel{replies: []string{"（歪头）诶，你今天是不是有点累？"}}
	speech, emotion, err := s.writeProactiveSpeech(context.Background(), convID, m, "今天面试好紧张")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(speech, "歪头") {
		t.Fatalf("行首舞台提示应被清洗，实际 %q", speech)
	}
	if !strings.Contains(speech, "你今天是不是有点累") {
		t.Fatalf("正文应保留，实际 %q", speech)
	}
	if emotion.Emotion == "" {
		t.Fatalf("应返回一个有效情绪，实际 %+v", emotion)
	}
	// 人设必须进入 prompt，否则主动发言会失去角色感。
	if len(m.inputs) == 0 || !strings.Contains(m.inputs[0][0].Content, "爱撒娇的猫娘") {
		t.Fatalf("prompt 应包含人设，实际 %+v", m.inputs)
	}
	if !strings.Contains(m.inputs[0][0].Content, "今天面试好紧张") {
		t.Fatalf("prompt 应包含选定话题，实际 %q", m.inputs[0][0].Content)
	}
}
