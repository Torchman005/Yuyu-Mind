package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// 本文件实现「关系状态」：让角色与用户的关系随相处时间累积，并影响称呼与语气。
//
// 背景（见 docs/REALISM-ANALYSIS.md P2）：真人聊天对象不是每次都"初次见面"——
// 熟人会更随意、更放松、更愿意多说，也会记得你们的相处历史。此前的实现完全没有这一层，
// 每次对话的语气都像第一次认识。
//
// 与情绪的分工：情绪是**分钟级**的（会衰减），关系是**长期累积**的（不回退）。
// 因此关系状态需要持久化，而情绪不需要。

const (
	relationshipSettingKey = "chat.relationship"
	// relationshipSaveEvery 控制落库频率（每 N 次交互保存一次），避免每轮都写 SQLite。
	relationshipSaveEvery = 5
)

// RelationshipState 是与用户的长期关系状态。
type RelationshipState struct {
	Interactions    int       `json:"interactions"`      // 累计交互次数
	FirstSeenAt     time.Time `json:"first_seen_at"`     // 初次互动时间
	LastSeenAt      time.Time `json:"last_seen_at"`      // 最近互动时间
	FamiliarityScore float64  `json:"familiarity_score"` // 亲密度 0..100
}

// relationshipStore 负责关系状态的读写与更新。
type relationshipStore struct {
	mu      sync.Mutex
	current RelationshipState
	loaded  bool
	// store 为 nil 时只保存在内存（测试或未接线场景）。
	store interface {
		Get(ctx context.Context, key string) (string, error)
		Set(ctx context.Context, key, value string) error
	}
}

func newRelationshipStore(store interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}) *relationshipStore {
	return &relationshipStore{store: store}
}

// load 读取关系状态（惰性、只读一次）。读取失败时退回零值，不阻断对话。
func (r *relationshipStore) load(ctx context.Context) RelationshipState {
	if r.loaded {
		return r.current
	}
	r.loaded = true
	if r.store == nil {
		return r.current
	}
	raw, err := r.store.Get(ctx, relationshipSettingKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return r.current
	}
	var state RelationshipState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		slog.Warn("relationship: parse stored state failed, starting fresh", "err", err)
		return r.current
	}
	r.current = state
	return r.current
}

// save 落库当前状态（失败只记日志，不影响对话）。
func (r *relationshipStore) save(ctx context.Context, state RelationshipState) {
	if r.store == nil {
		return
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := r.store.Set(ctx, relationshipSettingKey, string(payload)); err != nil {
		slog.Warn("relationship: persist failed", "err", err)
	}
}

// Observe 记录一次用户交互并更新关系状态，返回更新后的状态。
//
// 更新用**客观信号**（交互次数 / 消息长度 / 情绪词 / 礼貌用语），不额外调用 LLM：
// 关系是慢变量，没必要为它每轮付一次模型成本。
func (r *relationshipStore) Observe(ctx context.Context, userText string) RelationshipState {
	r.mu.Lock()
	defer r.mu.Unlock()

	state := r.load(ctx)
	now := time.Now()
	if state.FirstSeenAt.IsZero() {
		state.FirstSeenAt = now
	}
	state.LastSeenAt = now
	state.Interactions++
	state.FamiliarityScore = clampFamiliarity(state.FamiliarityScore + familiarityDelta(userText))

	r.current = state
	// 首轮与周期性落库（控制写入频率）。
	if state.Interactions == 1 || state.Interactions%relationshipSaveEvery == 0 {
		r.save(ctx, state)
	}
	return state
}

// Snapshot 返回当前关系状态（不产生副作用）。
func (r *relationshipStore) Snapshot(ctx context.Context) RelationshipState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load(ctx)
}

// familiarityDelta 计算单次交互对亲密度的增量。
//
// 刻意让"聊得越多越熟"但不线性膨胀：普通消息 +1，有实质内容/情绪/礼貌的更多一些。
// 单次上限 3，避免一句话就把关系推到很熟。
func familiarityDelta(userText string) float64 {
	text := strings.TrimSpace(userText)
	if text == "" {
		return 0
	}
	delta := 1.0
	if n := len([]rune(text)); n >= 20 {
		delta += 0.5 // 愿意展开说 = 更亲近
	}
	for _, marker := range []string{
		"谢谢", "辛苦", "麻烦你", "帮我", "拜托", "对不起", "抱歉",
	} {
		if strings.Contains(text, marker) {
			delta += 0.5
			break
		}
	}
	for _, marker := range []string{
		"开心", "难过", "累", "烦", "紧张", "担心", "害怕", "喜欢", "讨厌", "委屈",
	} {
		if strings.Contains(text, marker) {
			delta += 0.5 // 愿意表露情绪 = 信任
			break
		}
	}
	if delta > 3 {
		delta = 3
	}
	return delta
}

func clampFamiliarity(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// relationshipPromptLine 把关系状态转成一句**描述语气而非数值**的提示，注入 Planner。
//
// 同样遵循"模糊措辞"原则（见 emotionPromptLine）：暴露数值会让模型机械执行，
// 而这里要的是"你们是熟人"这种关系感。
//
// 注意：不改变人设规定的称呼（如「主人」）——那属于角色设定，
// 关系只影响**放松程度、话题深度与互动熟稔度**，避免角色前后不一致。
func relationshipPromptLine(state RelationshipState) string {
	if state.Interactions <= 0 {
		return ""
	}
	var stage string
	switch {
	case state.Interactions >= 60 || state.FamiliarityScore >= 60:
		stage = "你和对方已经很熟了，像相处很久的老朋友：说话放松、可以直接开玩笑，也可以自然地提起以前聊过的事。"
	case state.Interactions >= 20 || state.FamiliarityScore >= 25:
		stage = "你和对方比较熟了：语气可以不那么客气，偶尔主动带点玩笑或关心。"
	case state.Interactions >= 5:
		stage = "你和对方已经聊过一阵子了，比刚认识时自然一些。"
	default:
		stage = "你和对方还不太熟，语气客气自然，别表现得太熟络。"
	}
	return stage
}

// RelationshipLine 返回当前关系的中文描述（供注入 prompt）。
// 每个会话共享同一份"与用户的关系"（桌宠是单用户场景）。
func (s *Service) RelationshipLine(ctx context.Context) string {
	if s == nil || s.relations == nil {
		return ""
	}
	return relationshipPromptLine(s.relations.Snapshot(ctx))
}

// ObserveUserTurn 记录一次用户交互（更新关系），并返回关系描述。
// 由 StreamChat 在每轮开始时调用。
func (s *Service) ObserveUserTurn(ctx context.Context, userText string) string {
	if s == nil || s.relations == nil {
		return ""
	}
	state := s.relations.Observe(ctx, userText)
	return relationshipPromptLine(state)
}

// relationSummaryForLog 便于在日志/诊断中查看关系进展。
func relationSummaryForLog(state RelationshipState) string {
	return fmt.Sprintf("interactions=%d familiarity=%.0f", state.Interactions, state.FamiliarityScore)
}
