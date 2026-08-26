package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/yuyu-mind/backend/internal/config"
)

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", `{"action":"reply"}`, `{"action":"reply"}`},
		{"markdown-fence", "```json\n{\"action\":\"reply\"}\n```", `{"action":"reply"}`},
		{"surrounding-text", "text before {\"action\":\"task\",\"task\":{}} text after", `{"action":"task","task":{}}`},
	}
	for _, c := range cases {
		if got := extractJSONObject(c.in); got != c.want {
			t.Fatalf("%s: extractJSONObject(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestPostprocessReply(t *testing.T) {
	in := "（思考中）\n你好，世界。\n(这是舞台指示)\n"
	got := postprocessReply(in)
	if strings.Contains(got, "舞台指示") || strings.Contains(got, "思考中") {
		t.Fatalf("stage directions not removed: %q", got)
	}
	if !strings.Contains(got, "你好，世界。") {
		t.Fatalf("content lost: %q", got)
	}
}

func TestSplitReply(t *testing.T) {
	parts := splitReply("你好。世界。", 3)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
	}
	if joined := strings.Join(parts, ""); joined != "你好。世界。" {
		t.Fatalf("split+join = %q", joined)
	}
}

func TestLooksLikeQuestionOrRequest(t *testing.T) {
	if !looksLikeQuestionOrRequest("你能帮我吗？") {
		t.Fatalf("expected question marker")
	}
	if !looksLikeQuestionOrRequest("怎么配置模型") {
		t.Fatalf("expected request marker")
	}
	if looksLikeQuestionOrRequest("好的") {
		t.Fatalf("expected not a question")
	}
}

func TestEmotionNormalization(t *testing.T) {
	if NormalizeEmotion("happy") != "happy" {
		t.Fatalf("valid emotion changed")
	}
	if NormalizeEmotion("bogus") != EmotionNeutral {
		t.Fatalf("invalid emotion should fall back to neutral")
	}
	if NormalizeMood("cheer") != "cheer" {
		t.Fatalf("valid mood changed")
	}
	if NormalizeMood("") != MoodCalm {
		t.Fatalf("empty mood should fall back to calm")
	}
	if ClampEnergy(1.5) != 1 || ClampEnergy(-0.5) != 0 || ClampEnergy(0.4) != 0.4 {
		t.Fatalf("energy clamping incorrect")
	}
	if ClampValence(1.5) != 1 || ClampValence(-1.5) != -1 || ClampValence(0.4) != 0.4 {
		t.Fatalf("valence clamping incorrect")
	}
	if ClampDominance(2) != 1 || ClampDominance(-2) != -1 || ClampDominance(-0.3) != -0.3 {
		t.Fatalf("dominance clamping incorrect")
	}
}

func TestPlannerDecisionEmotionInfo(t *testing.T) {
	d := PlannerDecision{Emotion: "happy", Mood: "cheer", Energy: 0.8, Valence: 0.6, Dominance: 0.2, Gesture: "bounce", Hand: "left"}
	info := d.EmotionInfo()
	if info.Emotion != "happy" || info.Mood != "cheer" || info.Energy != 0.8 || info.Valence != 0.6 || info.Dominance != 0.2 || info.Gesture != "bounce" || info.Hand != "left" {
		t.Fatalf("EmotionInfo mismatch: %+v", info)
	}
}

func TestParsePlannerDecision(t *testing.T) {
	d, err := parsePlannerDecision(`{"action":"reply","emotion":"happy","mood":"cheer","energy":0.9,"valence":0.8,"dominance":0.3}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Action != "reply" || d.Emotion != "happy" || d.Mood != "cheer" || d.Energy != 0.9 || d.Valence != 0.8 || d.Dominance != 0.3 {
		t.Fatalf("unexpected decision: %+v", d)
	}

	// 围栏包裹 + 非法情绪归一化。
	d, err = parsePlannerDecision("```json\n{\"action\":\"task\",\"emotion\":\"bogus\",\"energy\":2,\"valence\":3,\"dominance\":-2}\n```")
	if err != nil {
		t.Fatalf("parse fenced: %v", err)
	}
	if d.Emotion != EmotionNeutral || d.Energy != 1 || d.Valence != 1 || d.Dominance != -1 {
		t.Fatalf("normalization failed: %+v", d)
	}

	// 空 action 不应报错（由 Plan 兜底为 reply）。
	d, err = parsePlannerDecision(`{"reason":"no action"}`)
	if err != nil {
		t.Fatalf("parse empty action: %v", err)
	}
	if d.Action != "" {
		t.Fatalf("expected empty action, got %q", d.Action)
	}

	// 非法 JSON 报错。
	if _, err := parsePlannerDecision("not json"); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestTurnGateEvaluate(t *testing.T) {
	gate := NewTurnGate(config.ChatConfig{ReplyThreshold: 0.45, ReplyFrequency: 1})
	now := time.Now()
	snapshot := TurnSnapshot{
		Target: NormalizedMessage{ID: "m1", Content: "你能帮我写个文件吗？", CreatedAt: now},
		Now:    now,
	}
	decision := gate.Evaluate(snapshot)
	if !decision.ShouldPlan {
		t.Fatalf("expected ShouldPlan for question, got score=%f threshold=%f", decision.Score, decision.Threshold)
	}

	weak := gate.Evaluate(TurnSnapshot{
		Target:    NormalizedMessage{ID: "m2", Content: "好的", CreatedAt: now},
		LastBotAt: now,
		BotStreak: 2,
		Now:       now,
	})
	if weak.ShouldPlan {
		t.Fatalf("expected weak backchannel to be gated, got score=%f", weak.Score)
	}

	// 普通陈述句（非弱回撤）即使紧接着上一条 bot 回复、且已有连续回复 streak，也应得到回复。
	statement := gate.Evaluate(TurnSnapshot{
		Target:    NormalizedMessage{ID: "m3", Content: "今天好累啊", CreatedAt: now},
		LastBotAt: now,
		BotStreak: 3,
		Now:       now,
	})
	if !statement.ShouldPlan {
		t.Fatalf("expected statement to get reply, got score=%f", statement.Score)
	}
}

func TestParsePlannerDecisionWindowsPath(t *testing.T) {
	// 模型常把 Windows 路径原样写进 JSON（含 \i \A 等非法转义），应被修正后仍能解析。
	raw := `{"action":"task","task":{"goal":"init react","workspace":"D:\itJinYu_toolkit\AI-pet"}}`
	decision, err := parsePlannerDecision(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if decision.Action != "task" || decision.Task == nil || decision.Task.Goal != "init react" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if decision.Task.Workspace != "D:\\itJinYu_toolkit\\AI-pet" {
		t.Fatalf("workspace not fixed: %q", decision.Task.Workspace)
	}
}
