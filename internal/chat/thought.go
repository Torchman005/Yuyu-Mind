package chat

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

// 本文件实现「内心独白」（见 docs/REALISM-ANALYSIS.md，P3）。
//
// 背景：此前的角色只有**说出口的话**这一条通道——她说的每一句都是对用户说的。
// 真人不是这样：听话的时候心里会闪过吐槽、走神、犹豫、"我是不是说错了"。
// 这些念头**没有出口**，但它们的存在本身就让人相信"她有个内心"。
// Neuro-sama 的"自言自语"之所以有效，正是因为它不是回答，而是回答之外的一层。
//
// 三条设计红线：
//
//  1. **不朗读、不进 TTS 队列**。独白经独立事件通道（EventTypeThought）下发，
//     前端用与台词完全不同的视觉层呈现。若混进 token 通道会被念出来，
//     那就不是"内心"而是"第二段台词"，效果立刻反向。
//  2. **不落库、不进对话历史**。独白一旦成为历史，模型下一轮就会把它当成自己说过的话，
//     开始"回应自己的心声"，同时也会污染记忆抽取。它必须是**即兴且无痕**的。
//     代价是重启后不留痕——这恰好符合"念头本来就不留痕"的语义。
//  3. **偶发**。每轮都说 = 新的口癖。模型侧自评"有没有真的闪过念头"，
//     服务侧再用冷却窗口与概率二次克制（见 shouldSurfaceThought）。

const (
	// thoughtMinRunes / thoughtMaxRunes 是独白的长度区间。
	// 上限刻意压得很短：独白是"一闪而过"，写成长段就变成了内心戏表演。
	thoughtMinRunes = 2
	thoughtMaxRunes = 34

	// monologueMinGap 是两次独白之间的最小间隔：说话太密的角色不像有内心，像有旁白。
	monologueMinGap = 20 * time.Second

	// monologueChance 是"模型给出了独白"之后仍要再掷一次的通过概率。
	// 模型已经自评过一轮，这里只做二次克制，所以取值不宜太低（否则独白几乎不出现）。
	monologueChance = 0.6
)

// monologueGate 按会话记录上次显示独白的时间，实现"冷却 + 概率"的二次克制。
//
// 状态放在进程内存：独白本身不落库（见文件头第 2 条），因此它的节流状态
// 也不需要跨重启存活——重启后第一句话就出现独白是可以接受的。
type monologueGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newMonologueGate() *monologueGate {
	return &monologueGate{last: make(map[string]time.Time)}
}

// Allow 判断此刻能否显示一句内心独白；允许时顺带记录时间戳（因此它是有副作用的判定）。
//
// allow 来自配置开关；r 是 [0,1) 的随机数，由调用方注入以便测试确定性；
// 传入负值表示"强制通过"（与情绪门控 shouldAdvanceMood 的约定一致）。
func (g *monologueGate) Allow(conversationID string, allow bool, r float64, now time.Time) bool {
	if g == nil {
		// 未装配门控（例如只构造了部分字段的轻量测试）时不拦截。
		return allow
	}
	if now.IsZero() {
		now = time.Now()
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	prev, seen := g.last[conversationID]
	if !shouldSurfaceThought(allow, now.Sub(prev), seen, r) {
		return false
	}
	g.last[conversationID] = now
	return true
}

// allowMonologue 判断此刻能否下发内心独白：配置开关 + 冷却/概率门控的组合入口。
//
// nil 安全：未装配门控（如只构造了部分字段的轻量测试）时按配置开关直通，
// 这样"关掉开关就一句都不出现"这一断言在任何构造方式下都成立。
func (s *Service) allowMonologue(conversationID string, allow bool, r float64, now time.Time) bool {
	if s == nil || s.monologue == nil {
		return allow
	}
	return s.monologue.Allow(conversationID, allow, r, now)
}

// shouldSurfaceThought 是独白门控的纯函数形式（便于表驱动测试）。
//
// sinceLast/hasLast 描述"距上次独白过了多久"；hasLast=false 表示本次会话还没有过独白。
func shouldSurfaceThought(allow bool, sinceLast time.Duration, hasLast bool, r float64) bool {
	if !allow {
		return false
	}
	// 冷却窗口优先于概率：刚说过就不再显示，无论随机数多低。
	if hasLast && sinceLast < monologueMinGap {
		return false
	}
	if r < 0 {
		// 显式强制通过（测试/诊断用），避免依赖"随机数恰好小于概率"的边界假设。
		return true
	}
	if r >= 1 {
		// 越界随机数折回 [0,1)，保持行为可预期（与 ThinkingPause 同样处理）。
		r = r - float64(int(r))
	}
	return r < monologueChance
}

// normalizeThought 清洗模型给出的内心独白；返回空串表示"这句不该显示"。
//
// 纯函数，不依赖随机数与时间，因此可以被表驱动测试完整覆盖。
//
// ⚠️ 这里**不能**复用 postprocessReply（台词用的舞台提示清洗器），原因很具体：
// 它的 stageLinePattern 会把「整行都是括号内容」的行直接删掉，而模型极爱把心里话写成
// 「（他今天话好少，是不是累了）」——那正是一行完整的括号文本，会被整句删空。
// 也就是说复用清洗器会让**大多数独白静默消失**，且只在"模型恰好写了括号"时发生，极难排查。
// 因此独白走自己的清洗：只剥**成对的外层**括号，保留里面的内容。
//
// 三件事：
//  1. 压平成单行（独白不该是多行段落）；
//  2. 剥掉外层包裹——模型极爱把心里话写成「（…）」或加引号，
//     这层括号是书面约定，真到画面上反而显得做作；
//  3. 长度护栏：过短（无信息量）丢弃，过长则截断——独白一旦变长就成了内心戏。
func normalizeThought(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}

	// 1) 压平：先去掉换行，再把连续空白折叠为单个空格。
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}

	// 剥掉模型可能添加的字段名前缀（"thought: …"「心声：…」）。
	text = strings.TrimSpace(trimThoughtLabel(text))
	if text == "" {
		return ""
	}

	// 2) 剥外层包裹（可能嵌套：「（嗯…）」）。
	text = strings.TrimSpace(trimThoughtWrapper(text))
	text = strings.TrimSpace(trimThoughtLabel(text))

	// 占位值：模型有时会用 none/null 之类填一个"可选但我不想写"的字段。
	switch strings.ToLower(text) {
	case "none", "null", "nil", "n/a", "na", "-", "—", "无", "没有", "空", "略", "…", "...", "。。":
		return ""
	}

	// 3) 内容与长度护栏。
	// 只剩标点/空白的输出（如 "" 「……」「（）」）不是念头 —— 模型用它们填"可选但不想写"的字段。
	if !hasThoughtContent(text) {
		return ""
	}
	runes := []rune(text)
	if len(runes) < thoughtMinRunes {
		return ""
	}
	if len(runes) > thoughtMaxRunes {
		// 按字符截断（非字节）以免切断多字节中文；截断处补省略号提示"话没说完"。
		text = strings.TrimRight(string(runes[:thoughtMaxRunes]), "，,。.、；;：:!！?？ \t") + "…"
	}
	return text
}

// hasThoughtContent 判断文本里是否存在真正的"内容字符"（文字或数字）。
// CJK 由 unicode.IsLetter 覆盖，因此中文念头会被正确保留。
func hasThoughtContent(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// trimThoughtWrapper 反复剥掉成对的外层括号/引号。
// 只处理**首尾配对**的情况，因此不会破坏"（笑）嗯…刚才那个"里真正的语气补充。
func trimThoughtWrapper(text string) string {
	pairs := [][2]string{
		{"（", "）"}, {"(", ")"},
		{"「", "」"}, {"『", "』"},
		{"“", "”"}, {"‘", "’"},
		{"《", "》"}, {"【", "】"},
		{`"`, `"`}, {"'", "'"},
	}
	// 上限只是防御性的：正常输入最多嵌套两三层。
	for i := 0; i < 4; i++ {
		runes := []rune(strings.TrimSpace(text))
		if len(runes) < 2 {
			return strings.TrimSpace(text)
		}
		head, tail := string(runes[0]), string(runes[len(runes)-1])
		matched := false
		for _, pair := range pairs {
			// head/tail 相同的情况（引号）必须长度 > 2，否则「"」这种单字符会被误剥成空。
			if head == pair[0] && tail == pair[1] && (pair[0] != pair[1] || len(runes) > 2) {
				text = strings.TrimSpace(string(runes[1 : len(runes)-1]))
				matched = true
				break
			}
		}
		if !matched {
			return strings.TrimSpace(text)
		}
	}
	return strings.TrimSpace(text)
}

// trimThoughtLabel 剥掉开头的字段名/角色名标签（如 "thought:"「独白：」）。
func trimThoughtLabel(text string) string {
	lowered := strings.ToLower(text)
	for _, label := range []string{
		"thought:", "thought：", "inner monologue:", "inner:",
		"内心独白：", "内心独白:", "独白：", "独白:", "心声：", "心声:",
		"心里想：", "心里想:", "想法：", "想法:", "心理活动：", "心理活动:",
	} {
		if strings.HasPrefix(lowered, label) {
			return string([]rune(text)[len([]rune(label)):])
		}
	}
	return text
}
