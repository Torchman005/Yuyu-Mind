package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/memory"
	"github.com/yuyu-mind/backend/internal/usage"
)

// 本文件实现「对话后自动抽取记忆」（见 docs/REALISM-ANALYSIS.md P2）。
//
// 背景：此前长期记忆只有**手动**写入路径（Wails 导出的 UpsertMemory/AddMemoryCandidate），
// 对话过程里发生的事完全不落库。结果是角色"读得到、写不进"——你告诉它的事实与偏好，
// 下一轮就没了，跨会话更像第一次认识。
//
// 抽取策略：
//   - 用**一次低 token 的 LLM 调用**判断"这段对话里有没有值得长期记住的信息"，
//     没有就返回空（大多数闲聊都是空），避免往记忆里灌噪声；
//   - 只记**关于用户**的稳定信息（偏好/事实/长期要求），不记寒暄与一次性任务；
//   - 用 key 做幂等：同一个偏好再次出现时覆盖旧值，而不是堆一堆近似重复的记忆。
//
// 成本与延迟：抽取在回复之后同步执行，但 max_tokens 很小且只喂最近两条消息，
// 开销远低于主回复；失败只记日志，绝不影响回复本身。
//
// 写入走 Service.longMemory（memory.ServiceMemory），未接线时自动关闭。

// extractedMemory 是模型返回的单条记忆。
type extractedMemory struct {
	Kind  string `json:"kind"`  // preference | fact | instruction
	Key   string `json:"key"`   // 稳定标识，用于覆盖更新（如 "drink"、"job"）
	Text  string `json:"text"`  // 一句中文陈述
	Value string `json:"value"` // 可选结构化值
}

const (
	// memoryExtractMaxItems 单轮最多写入几条记忆，防止一次灌太多。
	memoryExtractMaxItems = 3
	// memoryExtractMinRunes 用户消息过短时不值得抽取（如"嗯""好的"）。
	memoryExtractMinRunes = 6
)

// ExtractAndStoreMemories 从一轮对话中抽取值得长期记住的信息并写入长期记忆。
//
// 返回写入的条数（0 表示本轮没有值得记的内容，属常见情况）。
// 任何失败都只记日志、不返回错误——记忆是增强项，不该影响对话主流程。
func (s *Service) ExtractAndStoreMemories(ctx context.Context, conversationID, userText, assistantText string) int {
	if s == nil || s.longMemory == nil {
		return 0
	}
	trimmed := strings.TrimSpace(userText)
	if len([]rune(trimmed)) < memoryExtractMinRunes {
		return 0
	}

	collector := usage.NewCollector()
	trackedModel, providerID, modelName, err := s.createTrackedModel(ctx, collector)
	if err != nil {
		slog.Warn("memory extract: create model failed", "err", err)
		return 0
	}

	items, err := s.extractMemories(ctx, trackedModel, userText, assistantText)
	if err != nil {
		slog.Warn("memory extract: model call failed", "err", err)
		return 0
	}
	if len(items) == 0 {
		return 0
	}

	stored := 0
	for _, item := range items {
		if err := s.storeExtractedMemory(ctx, item, conversationID); err != nil {
			slog.Warn("memory extract: store failed", "err", err, "key", item.Key)
			continue
		}
		stored++
	}
	if stored > 0 {
		s.persistTokenUsage(ctx, conversationID, providerID, modelName, "memory_extract", collector, 0, "success", nil)
		slog.Info("[chat] memories extracted", "count", stored, "conversation", conversationID)
	}
	return stored
}

// extractMemories 调用模型抽取记忆。返回空切片表示"本轮没有值得记的内容"。
func (s *Service) extractMemories(
	ctx context.Context,
	m model.BaseChatModel,
	userText, assistantText string,
) ([]extractedMemory, error) {
	system := &schema.Message{
		Role: schema.System,
		Content: `你负责从一段对话里挑出**值得长期记住的用户信息**。

只记录关于"用户本人"的稳定信息：
- preference：偏好（喜欢/讨厌什么、习惯）
- fact：事实（职业、住址、家人、在做的事）
- instruction：用户对未来互动的长期要求（"以后都用中文"、"别叫我老板"）

不要记录：
- 寒暄、情绪化的临时状态（"我今天好累"不是长期事实）
- 一次性的任务与请求（"帮我改个文件"）
- 助手的发言内容
- 任何你不确定的推测

若没有值得记的内容，返回 {"memories": []}。

输出**严格 JSON**（不要 markdown 代码块、不要解释）：
{"memories":[{"kind":"preference|fact|instruction","key":"简短英文标识","text":"一句中文陈述"}]}

要求：
- key 要稳定可复用（同一件事下次出现应使用同一个 key，以便覆盖更新），如 "drink"、"job"、"language"。
- text 用第三人称陈述用户，例如"用户喜欢喝美式咖啡"。
- 最多 3 条；宁缺毋滥。`,
	}
	user := &schema.Message{
		Role: schema.User,
		Content: fmt.Sprintf("用户说：%s\n助手回复：%s\n\n请输出 JSON。",
			truncateForTopic(userText, 300), truncateForTopic(assistantText, 200)),
	}

	result, err := m.Generate(ctx, []*schema.Message{system, user}, model.WithMaxTokens(300))
	if err != nil {
		return nil, fmt.Errorf("extract memories: %w", err)
	}
	return parseExtractedMemories(result.Content), nil
}

// parseExtractedMemories 解析模型输出（纯函数，便于测试）。
// 容忍 markdown 代码块与多余文本；无法解析时返回空切片而不是报错。
func parseExtractedMemories(raw string) []extractedMemory {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil
	}
	var payload struct {
		Memories []extractedMemory `json:"memories"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		slog.Debug("memory extract: parse failed", "err", err)
		return nil
	}

	out := make([]extractedMemory, 0, len(payload.Memories))
	for _, item := range payload.Memories {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		text := strings.TrimSpace(item.Text)
		key := strings.TrimSpace(item.Key)
		if text == "" || key == "" {
			continue
		}
		switch kind {
		case memory.MemoryKindPreference, memory.MemoryKindFact, memory.MemoryKindInstruction:
		default:
			continue // 只接受这三类；其它（如 episode）交给摘要路径
		}
		out = append(out, extractedMemory{Kind: kind, Key: key, Text: text, Value: item.Value})
		if len(out) >= memoryExtractMaxItems {
			break
		}
	}
	return out
}

// storeExtractedMemory 把一条抽取结果写入长期记忆（按 key 覆盖更新）。
func (s *Service) storeExtractedMemory(ctx context.Context, item extractedMemory, conversationID string) error {
	value := any(item.Text)
	if strings.TrimSpace(item.Value) != "" {
		value = item.Value
	}
	_, err := s.longMemory.UpsertMemory(ctx, memory.UpsertMemoryRequest{
		Scope:      memory.MemoryScopeUser,
		Kind:       item.Kind,
		Key:        item.Key,
		Value:      value,
		Text:       item.Text,
		Confidence: 0.7, // 模型抽取：置信度中等，便于日后人工复核
		Source:     memory.MemorySourceInferred,
	})
	return err
}
