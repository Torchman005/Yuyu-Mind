package chat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/config"
	"github.com/yuyu-mind/backend/internal/db"
)

// ---- 纯函数：normalizeThought ----

// TestNormalizeThought 覆盖内心独白的清洗规则。这些规则的存在理由都是"实测中模型真的会这么写"：
// 心里话被写成括号、被加上引号、被压成多行、或者干脆用 none 填一个不想写的可选字段。
func TestNormalizeThought(t *testing.T) {
	longInput := strings.Repeat("啊", 40)

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"空串不产生独白", "", ""},
		{"纯空白不产生独白", "   \n  ", ""},
		{"剥掉中文圆括号", "（其实我有点困了）", "其实我有点困了"},
		{"剥掉直角引号", "「主人今天怪怪的」", "主人今天怪怪的"},
		{"剥掉英文双引号", `"嗯…他到底想说什么"`, "嗯…他到底想说什么"},
		{"嵌套括号全部剥掉", "（（唔，好像说错了））", "唔，好像说错了"},
		{"剥掉字段名前缀", "thought: 这个方案不太行", "这个方案不太行"},
		{"剥掉中文标签", "心声：这条裙子挺好看的", "这条裙子挺好看的"},
		{"标签与括号叠加", "（thought: 先别急）", "先别急"},
		{"多行压平成一行", "嗯…\n他好像\n有点累", "嗯… 他好像 有点累"},
		{"行内的括号保留（不是包裹）", "（笑）嗯…刚才那个", "（笑）嗯…刚才那个"},

		// 占位值：模型把"可选但不想写"的字段用这些词填满。
		{"none 占位", "none", ""},
		{"null 占位", "NULL", ""},
		{"中文无占位", "无", ""},
		{"纯标点占位（省略号）", "……", ""},
		{"纯标点占位（空引号）", `""`, ""},
		{"纯标点占位（括号）", "（）", ""},
		{"单字过短", "困", ""},
		{"未配对的左括号（无内容）", "（", ""},

		{"超长截断并补省略号", longInput, strings.Repeat("啊", thoughtMaxRunes) + "…"},
		{"正常独白原样保留", "唔，好像答非所问了", "唔，好像答非所问了"},
	}

	for _, c := range cases {
		if got := normalizeThought(c.raw); got != c.want {
			t.Fatalf("%s: normalizeThought(%q) = %q, want %q", c.name, c.raw, got, c.want)
		}
	}

	// 截断后总长度必须仍在护栏内（多出来的省略号不能把长度顶超）。
	got := normalizeThought(longInput)
	if n := len([]rune(got)); n != thoughtMaxRunes+1 {
		t.Fatalf("截断后长度 = %d, want %d", n, thoughtMaxRunes+1)
	}
	// 幂等：清洗结果再清洗一次不应继续变化（避免前端/后端二次处理产生差异）。
	if again := normalizeThought(got); again != got {
		t.Fatalf("normalizeThought 非幂等: %q -> %q", got, again)
	}
}

// TestNormalizeThoughtMustNotReuseSpeechCleaner 锁住一个很容易踩、且后果隐蔽的坑：
// 独白不能复用台词的清洗器 postprocessReply。
//
// 台词的清洗器会把「整行都是括号内容」的行删掉（那是为了去掉「（笑）」这类舞台提示），
// 而模型极爱把心里话写成「（…）」——一旦复用，绝大多数独白会被**静默删空**，
// 且只在"模型恰好写了括号"时发生，排查成本极高。这个测试同时断言两侧行为，
// 让后来者想"顺手复用一下"时立刻看到代价。
func TestNormalizeThoughtMustNotReuseSpeechCleaner(t *testing.T) {
	const wrapped = "（他今天话好少，是不是累了）"

	if got := postprocessReply(wrapped); got != "" {
		// 前提变了：清洗器不再删整行括号。此时可以重新评估是否需要两条清洗路径，
		// 但不代表可以盲目合并——仍然要保留"独白不朗读、不进历史"的语义。
		t.Fatalf("前提变化：postprocessReply(%q) = %q，本测试需重新评估", wrapped, got)
	}
	if got := normalizeThought(wrapped); got != "他今天话好少，是不是累了" {
		t.Fatalf("独白被误删或未剥括号: %q", got)
	}
}

// ---- 纯函数：shouldSurfaceThought ----

// TestShouldSurfaceThought 覆盖独白门控：配置开关、冷却窗口、概率。
func TestShouldSurfaceThought(t *testing.T) {
	cases := []struct {
		name      string
		allow     bool
		sinceLast time.Duration
		hasLast   bool
		r         float64
		want      bool
	}{
		{"开关关闭时一律不显示", false, time.Hour, true, 0, false},
		{"开关关闭时即使首次也不显示", false, time.Hour, false, 0, false},
		{"首次且随机数低", true, time.Hour, false, 0.1, true},
		{"首次但随机数高", true, time.Hour, false, 0.9, false},
		{"刚说过就不再说（冷却优先于概率）", true, 5 * time.Second, true, 0, false},
		{"刚好到达冷却边界", true, monologueMinGap, true, 0, true},
		{"超过冷却窗口", true, 10 * time.Minute, true, 0.5, true},
		{"强制通过（负随机数）", true, time.Hour, true, -1, true},
		{"强制通过也不能越过冷却", true, time.Second, true, -1, false},
		{"越界随机数折回区间内", true, time.Hour, true, 1.5, true},
		{"概率边界：刚好等于阈值", true, time.Hour, true, monologueChance, false},
		{"概率边界：略低于阈值", true, time.Hour, true, monologueChance - 0.001, true},
	}
	for _, c := range cases {
		got := shouldSurfaceThought(c.allow, c.sinceLast, c.hasLast, c.r)
		if got != c.want {
			t.Fatalf("%s: shouldSurfaceThought(allow=%v, since=%v, hasLast=%v, r=%.3f) = %v, want %v",
				c.name, c.allow, c.sinceLast, c.hasLast, c.r, got, c.want)
		}
	}
}

// ---- 门控：monologueGate / Service.allowMonologue ----

func TestMonologueGateCooldownAndRecording(t *testing.T) {
	g := newMonologueGate()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	const conv = "c-1"

	// 首次：随机数低 → 通过，并记录时间戳。
	if !g.Allow(conv, true, 0.1, now) {
		t.Fatalf("首次独白应被允许")
	}
	// 紧随其后（仍在冷却窗口内）：即使随机数更低也必须被挡。
	if g.Allow(conv, true, 0, now.Add(5*time.Second)) {
		t.Fatalf("冷却窗口内不应再次显示独白")
	}
	// 越过冷却窗口：允许。
	if !g.Allow(conv, true, 0.1, now.Add(monologueMinGap+time.Second)) {
		t.Fatalf("冷却窗口之后应可再次显示独白")
	}
	// 不同会话互不影响（门控按会话隔离）。
	if !g.Allow("c-2", true, 0.1, now.Add(monologueMinGap+2*time.Second)) {
		t.Fatalf("另一个会话不应受本会话冷却影响")
	}

	// 开关关闭时：既不显示，也不占用冷却（后续开启不应被"幽灵记录"挡住）。
	if g.Allow(conv, false, 0.1, now.Add(time.Hour)) {
		t.Fatalf("开关关闭时不应显示独白")
	}
	if !g.Allow(conv, true, 0.1, now.Add(time.Hour+time.Second)) {
		t.Fatalf("被开关挡下的那一次不应占用冷却窗口")
	}
}

// TestServiceAllowMonologueNilSafety 验证未装配门控（轻量构造的 Service）时的降级语义：
// 完全由配置开关决定，这样"关掉开关就一句都不会出现"在任何构造方式下都成立。
func TestServiceAllowMonologueNilSafety(t *testing.T) {
	now := time.Now()
	var nilService *Service
	if nilService.allowMonologue("c", true, 0.1, now) != true {
		t.Fatalf("nil Service + 开关开启 应放行")
	}
	if nilService.allowMonologue("c", false, 0.1, now) != false {
		t.Fatalf("nil Service + 开关关闭 应拦截")
	}

	light := &Service{} // 未调用 NewService：monologue 为 nil
	if !light.allowMonologue("c", true, 0.1, now) {
		t.Fatalf("未装配门控时开关开启应放行")
	}
	if light.allowMonologue("c", false, 0.1, now) {
		t.Fatalf("未装配门控时开关关闭应拦截")
	}
}

// ---- 流式抽取：extractThoughtField ----

func TestExtractThoughtField(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		want         string
		wantComplete bool
	}{
		{"正常提取", `{"thought":"嗯，好像不太对","dialog":[]}`, "嗯，好像不太对", true},
		{"key 与冒号间有空格", `{"thought" : "先这样吧"}`, "先这样吧", true},
		{"字符串未闭合", `{"thought":"还没说完`, "", false},
		{"完全没有该字段", `{"dialog":[{"speech":"你好"}]}`, "", false},
		{"值为非字符串", `{"thought":null}`, "", false},
		{"转义引号", `{"thought":"他说\"算了\"就走了"}`, `他说"算了"就走了`, true},
		{"转义换行", `{"thought":"第一行\n第二行"}`, "第一行\n第二行", true},
		{"unicode 转义", `{"thought":"\u4e2d\u6587"}`, "中文", true},
		{"unicode 转义未到齐", `{"thought":"\u4e2`, "", false},
		{"非法转义按字面保留", `{"thought":"D:\itJinYu\myproj"}`, `D:\itJinYu\myproj`, true},
		{"尾部转义被切断", `{"thought":"结尾反斜杠\`, "", false},
		{"反斜杠与斜杠转义", `{"thought":"a\\b\/c"}`, `a\b/c`, true},
	}
	for _, c := range cases {
		got, complete := extractThoughtField(c.text)
		if complete != c.wantComplete || got != c.want {
			t.Fatalf("%s: extractThoughtField(%q) = (%q, %v), want (%q, %v)",
				c.name, c.text, got, complete, c.want, c.wantComplete)
		}
	}
}

// ---- 流式解析：dialogStreamParser 同时给出独白与台词 ----

// TestDialogStreamParserCapturesThoughtBeforeSpeech 是这一通道的核心语义断言：
// 独白必须在**包装前缀被丢弃之前**被捕获，否则流式路径下它会永久丢失。
func TestDialogStreamParserCapturesThoughtBeforeSpeech(t *testing.T) {
	p := newDialogStreamParser()

	// 第 1 片：thought 还没闭合 → 不应产生独白。
	items := p.feed(`{"thought":"嗯…他好像`)
	if got := p.takeThought(); got != "" {
		t.Fatalf("独白未闭合却被提取: %q", got)
	}
	if len(items) != 0 {
		t.Fatalf("此时不应有台词: %v", items)
	}

	// 第 2 片：thought 闭合，同时第一句台词完整。
	items = p.feed(`有点累","dialog":[{"speech":"怎么了呀？","emotion":"happy"}`)
	got := p.takeThought()
	if got != "嗯…他好像有点累" {
		t.Fatalf("独白提取失败: %q", got)
	}
	if len(items) != 1 || items[0].Speech != "怎么了呀？" {
		t.Fatalf("台词解析异常: %+v", items)
	}

	// 第 3 片：收尾的 ]} 不应重复产出独白。
	p.feed(`]}`)
	if again := p.takeThought(); again != "" {
		t.Fatalf("独白被重复产出: %q", again)
	}
}

// TestDialogStreamParserThoughtScenarios 覆盖单 chunk、无独白、独白在台词之后三种情形。
func TestDialogStreamParserThoughtScenarios(t *testing.T) {
	// 单 chunk（模型一次吐出全部 JSON）：包装对象一次性闭合，仍应拿到独白与两句台词。
	p := newDialogStreamParser()
	items := p.feed(`{"thought":"（其实我有点困了）","dialog":[{"speech":"第一句"},{"speech":"第二句"}]}`)
	if got := p.takeThought(); got != "其实我有点困了" {
		t.Fatalf("单 chunk 独白 = %q, want %q", got, "其实我有点困了")
	}
	if len(items) != 2 {
		t.Fatalf("单 chunk 台词条数 = %d, want 2", len(items))
	}

	// 没有独白字段：takeThought 必须返回空串（不能凭空造一句）。
	p = newDialogStreamParser()
	p.feed(`{"dialog":[{"speech":"只有台词"}]}`)
	if got := p.takeThought(); got != "" {
		t.Fatalf("无独白字段却产出了 %q", got)
	}

	// 模型把 thought 写在 dialog 之后（违反提示词要求）：仍应被提取，只是时序靠后。
	p = newDialogStreamParser()
	p.feed(`{"dialog":[{"speech":"先说出口的话"}],"thought":"这句反而来晚了"}`)
	if got := p.takeThought(); got != "这句反而来晚了" {
		t.Fatalf("后置独白 = %q", got)
	}

	// 占位值（模型用 none 填可选字段）不应被当成独白。
	p = newDialogStreamParser()
	p.feed(`{"thought":"none","dialog":[{"speech":"嗯"}]}`)
	if got := p.takeThought(); got != "" {
		t.Fatalf("占位值被当成独白: %q", got)
	}
}

// ---- 端到端：streamReply 下发独白事件 ----

// recordingEmitter 收集所有下发的 ChatEvent，供断言事件类型与顺序。
type recordingEmitter struct {
	events []ChatEvent
}

func (e *recordingEmitter) Emit(event ChatEvent) {
	e.events = append(e.events, event)
}

func (e *recordingEmitter) firstIndex(eventType ChatEventType) int {
	for i, ev := range e.events {
		if ev.Type == eventType {
			return i
		}
	}
	return -1
}

// chunkedStreamModel 按预设切片逐片流式返回，用于驱动 streamReply 的流式路径。
type chunkedStreamModel struct {
	chunks []string
}

func (m *chunkedStreamModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: strings.Join(m.chunks, "")}, nil
}

func (m *chunkedStreamModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 容量取够，保证发送端不会因缓冲满而阻塞（否则测试会死锁）。
	reader, writer := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer writer.Close()
		for _, chunk := range m.chunks {
			if writer.Send(&schema.Message{Role: schema.Assistant, Content: chunk}, nil) {
				return
			}
		}
	}()
	return reader, nil
}

// newThoughtService 构造一个可跑 streamReply 的最小 Service：
// 关掉口语冗余与反应停顿，使产物完全由脚本化的模型输出决定（否则随机注入会让断言不稳定）。
func newThoughtService(t *testing.T, allowMonologue bool, withGate bool) (*Service, string, *config.Config) {
	t.Helper()

	database, err := db.New(filepath.Join(t.TempDir(), "chat-thought.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	conv := &db.Conversation{
		ID:        "c-thought",
		Title:     "T",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.Conversations.Create(context.Background(), conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Chat.AllowTypoSimulation = false
	cfg.Chat.ThinkingPauseMinMs = 0
	cfg.Chat.ThinkingPauseMaxMs = 0
	cfg.Chat.AllowInnerMonologue = allowMonologue

	svc := &Service{cfg: cfg, db: database}
	if withGate {
		svc.monologue = newMonologueGate()
	}
	return svc, conv.ID, cfg
}

// TestStreamReplyEmitsThoughtBeforeSpeech 验证整条通道的接线：
//   - thought 走独立事件类型，且**排在台词之前**（先有念头，再开口）；
//   - thought 不会被混进 token（否则会被 TTS 念出来，"内心"就变成了第二段台词）；
//   - thought 不落库（否则下一轮会被当成自己说过的话喂回模型）。
func TestStreamReplyEmitsThoughtBeforeSpeech(t *testing.T) {
	// withGate=false：门控为 nil 时完全由配置决定，因此本用例是确定性的（不依赖随机数）。
	s, convID, cfg := newThoughtService(t, true, false)

	mdl := &chunkedStreamModel{chunks: []string{
		`{"thought":"（他今天话好少，`,
		`是不是累了）","dialog":[{"speech":"主人今天怎么啦？","emotion":"thinking"},`,
		`{"speech":"要不要休息一下。","emotion":"happy"}]}`,
	}}
	replyer := NewReplyerAgent(mdl, cfg.Chat)
	snapshot := TurnSnapshot{
		ConversationID: convID,
		Target:         NormalizedMessage{ID: "m1", ConversationID: convID, Content: "在吗"},
	}
	emitter := &recordingEmitter{}

	parts, err := s.streamReply(context.Background(), replyer, snapshot, PlannerDecision{Emotion: "neutral"}, nil, nil, emitter, &ConversationRuntime{})
	if err != nil {
		t.Fatalf("streamReply: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("台词条数 = %d (%v), want 2", len(parts), parts)
	}

	thoughtIdx := emitter.firstIndex(EventTypeThought)
	if thoughtIdx < 0 {
		t.Fatalf("未下发任何 thought 事件: %+v", emitter.events)
	}
	thought := emitter.events[thoughtIdx].Content
	if thought != "他今天话好少，是不是累了" {
		t.Fatalf("独白内容 = %q", thought)
	}
	tokenIdx := emitter.firstIndex(EventTypeToken)
	if tokenIdx < 0 || thoughtIdx > tokenIdx {
		t.Fatalf("独白必须排在台词之前: thought@%d token@%d", thoughtIdx, tokenIdx)
	}

	// 独白绝不能出现在任何一句被朗读的台词里（否则会被念出来）。
	for _, part := range parts {
		if strings.Contains(part, "累了") || strings.Contains(part, "话好少") {
			t.Fatalf("独白混进了台词: %q", part)
		}
	}
	for _, ev := range emitter.events {
		if ev.Type == EventTypeToken && strings.Contains(ev.Content, "话好少") {
			t.Fatalf("独白混进了 token 事件（会被 TTS 朗读）: %+v", ev)
		}
	}

	// 独白不落库：消息表里只应有说出口的两句。
	rows, err := s.db.Messages.ListByConversation(context.Background(), convID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("落库消息数 = %d, want 2（独白不应入库）", len(rows))
	}
	for _, row := range rows {
		if strings.Contains(row.Content, "话好少") {
			t.Fatalf("独白被写入了历史: %+v", row)
		}
		if row.SourceKind != "guided_reply" {
			t.Fatalf("台词 source_kind = %q, want guided_reply", row.SourceKind)
		}
	}
}

// TestStreamReplyThoughtDisabledByConfig 是"配置项必须有消费点"的回归测试。
//
// 本项目出现过 AllowTypoSimulation 定义了、赋值了、但全仓库无人读取的静默失效；
// 这里用真实调用链证明 AllowInnerMonologue=false 时确实一句独白都不会下发。
func TestStreamReplyThoughtDisabledByConfig(t *testing.T) {
	s, convID, cfg := newThoughtService(t, false, true)

	mdl := &chunkedStreamModel{chunks: []string{
		`{"thought":"这句绝不能被显示","dialog":[{"speech":"只有这句该出现","emotion":"neutral"}]}`,
	}}
	replyer := NewReplyerAgent(mdl, cfg.Chat)
	snapshot := TurnSnapshot{
		ConversationID: convID,
		Target:         NormalizedMessage{ID: "m1", ConversationID: convID, Content: "在吗"},
	}
	emitter := &recordingEmitter{}

	if _, err := s.streamReply(context.Background(), replyer, snapshot, PlannerDecision{}, nil, nil, emitter, &ConversationRuntime{}); err != nil {
		t.Fatalf("streamReply: %v", err)
	}
	if idx := emitter.firstIndex(EventTypeThought); idx >= 0 {
		t.Fatalf("开关关闭时不应下发 thought 事件: %+v", emitter.events[idx])
	}
	if idx := emitter.firstIndex(EventTypeToken); idx < 0 {
		t.Fatalf("开关关闭不应影响正常台词下发: %+v", emitter.events)
	}
}

// TestStreamReplyWithoutThoughtKeepsBehaviour 回归：模型不给独白时，旧行为必须完全不变。
func TestStreamReplyWithoutThoughtKeepsBehaviour(t *testing.T) {
	s, convID, cfg := newThoughtService(t, true, true)

	mdl := &chunkedStreamModel{chunks: []string{`{"dialog":[{"speech":"普通回复","emotion":"neutral"}]}`}}
	replyer := NewReplyerAgent(mdl, cfg.Chat)
	snapshot := TurnSnapshot{
		ConversationID: convID,
		Target:         NormalizedMessage{ID: "m1", ConversationID: convID, Content: "在吗"},
	}
	emitter := &recordingEmitter{}

	parts, err := s.streamReply(context.Background(), replyer, snapshot, PlannerDecision{}, nil, nil, emitter, &ConversationRuntime{})
	if err != nil {
		t.Fatalf("streamReply: %v", err)
	}
	if len(parts) != 1 || parts[0] != "普通回复" {
		t.Fatalf("台词 = %v, want [普通回复]", parts)
	}
	if idx := emitter.firstIndex(EventTypeThought); idx >= 0 {
		t.Fatalf("无独白时不应下发 thought 事件")
	}
}
