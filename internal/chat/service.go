package chat

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/ai/pipeline"
	aiProvider "github.com/yuyu-mind/backend/internal/ai/provider"
	"github.com/yuyu-mind/backend/internal/ai/template"
	"github.com/yuyu-mind/backend/internal/config"
	"github.com/yuyu-mind/backend/internal/db"
	"github.com/yuyu-mind/backend/internal/memory"
	"github.com/yuyu-mind/backend/internal/usage"
	pkgTypes "github.com/yuyu-mind/backend/pkg/types"
)

// Emitter 负责把流式事件发送给前端。
type Emitter interface {
	Emit(event ChatEvent)
}

// Service 编排聊天请求，连接模型供应商、Pipeline、记忆和数据库。
type Service struct {
	cfg         *config.Config
	db          *db.DB
	providerReg *aiProvider.Registry
	toolReg     interface{ GetAll() []tool.BaseTool }
	memory      memory.Store
}

// NewService 创建聊天服务。
func NewService(
	cfg *config.Config,
	database *db.DB,
	providerReg *aiProvider.Registry,
	toolReg interface{ GetAll() []tool.BaseTool },
	memStore memory.Store,
) *Service {
	return &Service{
		cfg:         cfg,
		db:          database,
		providerReg: providerReg,
		toolReg:     toolReg,
		memory:      memStore,
	}
}

// StreamChat 处理一次聊天请求，并通过 emitter 推送流式事件。
func (s *Service) StreamChat(ctx context.Context, req ChatRequest, emitter Emitter) error {
	startedAt := time.Now()
	mode := "chat"
	if req.UseTools {
		mode = "agent"
	}
	collector := usage.NewCollector()

	// 读取当前激活的供应商配置。
	providerCfg, err := s.cfg.GetActiveProviderConfig()
	if err != nil {
		emitter.Emit(ChatEvent{Type: EventTypeError, Content: err.Error()})
		return fmt.Errorf("get provider config: %w", err)
	}

	providerID := s.cfg.ActiveProvider.ProviderID
	modelName := providerCfg.Model

	// 创建 ChatModel，并包装 token 追踪逻辑。
	chatModel, err := s.providerReg.Create(ctx, pkgTypes.ProviderConfig{
		ID:      providerID,
		Name:    providerCfg.Name,
		BaseURL: providerCfg.BaseURL,
		APIKey:  providerCfg.APIKey,
		Model:   modelName,
	})
	if err != nil {
		emitter.Emit(ChatEvent{Type: EventTypeError, Content: fmt.Sprintf("Failed to create model: %v", err)})
		s.persistTokenUsage(ctx, req.ConversationID, providerID, modelName, mode, collector, time.Since(startedAt), "failed", err)
		return fmt.Errorf("create model: %w", err)
	}
	trackedModel := usage.NewTrackedChatModel(chatModel, collector)

	// 加载会话历史，失败时降级为空上下文。
	history, err := s.memory.GetHistory(ctx, req.ConversationID)
	if err != nil {
		slog.Warn("failed to load history, starting fresh", "error", err)
		history = nil
	}

	// 根据请求选择普通聊天或 Agent 工具调用链路。
	var assistantMsg *schema.Message
	if req.UseTools {
		assistantMsg, err = s.runAgentPipeline(ctx, trackedModel, history, req, emitter)
	} else {
		assistantMsg, err = s.runChatPipeline(ctx, trackedModel, history, req, emitter)
	}

	if err != nil {
		emitter.Emit(ChatEvent{Type: EventTypeError, Content: err.Error()})
		s.persistTokenUsage(ctx, req.ConversationID, providerID, modelName, mode, collector, time.Since(startedAt), "failed", err)
		return err
	}

	// 保存用户消息和助手回复。
	userMsg := &schema.Message{Role: schema.User, Content: req.Content}
	if err := s.memory.AppendMessages(ctx, req.ConversationID, []*schema.Message{userMsg, assistantMsg}); err != nil {
		slog.Error("failed to persist messages", "error", err)
	}
	s.persistTokenUsage(ctx, req.ConversationID, providerID, modelName, mode, collector, time.Since(startedAt), "success", nil)

	// 通知前端本次流式响应结束。
	emitter.Emit(ChatEvent{Type: EventTypeDone})
	return nil
}

// runChatPipeline 执行不带工具调用的普通聊天链路。
func (s *Service) runChatPipeline(
	ctx context.Context,
	chatModel model.ChatModel,
	history []*schema.Message,
	req ChatRequest,
	emitter Emitter,
) (*schema.Message, error) {
	tmpl, err := template.NewChatTemplate("You are a helpful assistant.")
	if err != nil {
		return nil, fmt.Errorf("create chat template: %w", err)
	}

	runnable, err := pipeline.BuildChatChain(chatModel, tmpl)
	if err != nil {
		return nil, fmt.Errorf("build chat chain: %w", err)
	}

	// 流式读取响应，同时保留 chunk 中的 ResponseMeta。
	streamReader, err := runnable.Stream(ctx, pipeline.ChatChainInput{
		Query:   req.Content,
		History: history,
	})
	if err != nil {
		return nil, fmt.Errorf("stream chat: %w", err)
	}
	defer streamReader.Close()

	chunks := make([]*schema.Message, 0, 32)
	for {
		chunk, err := streamReader.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive chat stream: %w", err)
		}
		chunks = append(chunks, chunk)
		if chunk.Content != "" {
			emitter.Emit(ChatEvent{Type: EventTypeToken, Content: chunk.Content})
		}
	}

	if len(chunks) == 0 {
		return &schema.Message{Role: schema.Assistant}, nil
	}

	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, fmt.Errorf("concat chat stream: %w", err)
	}
	if msg.Role == "" {
		msg.Role = schema.Assistant
	}
	return msg, nil
}

// runAgentPipeline 执行带工具调用的 Agent 链路。
func (s *Service) runAgentPipeline(
	ctx context.Context,
	chatModel model.ChatModel,
	history []*schema.Message,
	req ChatRequest,
	emitter Emitter,
) (*schema.Message, error) {
	// 汇总工具描述，写入系统提示词。
	tools := s.toolReg.GetAll()
	toolDescs := make([]string, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		toolDescs = append(toolDescs, fmt.Sprintf("%s: %s", info.Name, info.Desc))
	}

	tmpl, err := template.NewAgentTemplate("You are a helpful assistant.", toolDescs)
	if err != nil {
		return nil, fmt.Errorf("create agent template: %w", err)
	}

	runnable, err := pipeline.BuildAgentGraph(chatModel, tmpl, tools, 10)
	if err != nil {
		return nil, fmt.Errorf("build agent graph: %w", err)
	}

	// 执行 Agent 图。
	result, err := runnable.Invoke(ctx, pipeline.AgentGraphInput{
		Query:   req.Content,
		History: history,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke agent: %w", err)
	}

	// 当前 Agent 图是非流式执行，完成后一次性发送最终回复。
	if result.Content != "" {
		emitter.Emit(ChatEvent{Type: EventTypeToken, Content: result.Content})
	}

	return result, nil
}

// CreateConversation 创建新会话。
func (s *Service) CreateConversation(ctx context.Context, title string) (*db.Conversation, error) {
	providerCfg, _ := s.cfg.GetActiveProviderConfig()
	conv := &db.Conversation{
		ID:        uuid.New().String(),
		Title:     title,
		Provider:  s.cfg.ActiveProvider.ProviderID,
		Model:     providerCfg.Model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.db.Conversations.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

// ListConversations 返回所有会话。
func (s *Service) ListConversations(ctx context.Context) ([]*db.Conversation, error) {
	return s.db.Conversations.List(ctx)
}

// DeleteConversation 删除会话及其关联数据。
func (s *Service) DeleteConversation(ctx context.Context, id string) error {
	return s.db.Conversations.Delete(ctx, id)
}

// GetMessages 返回指定会话的所有消息。
func (s *Service) GetMessages(ctx context.Context, convID string) ([]*db.Message, error) {
	return s.db.Messages.ListByConversation(ctx, convID)
}

// ListTokenUsageByConversation 返回指定会话的 token 用量明细。
func (s *Service) ListTokenUsageByConversation(ctx context.Context, convID string) ([]*db.TokenUsageRecord, error) {
	return s.db.TokenUsage.ListByConversation(ctx, convID)
}

// GetTokenUsageSummary 返回全局 token 用量总览。
func (s *Service) GetTokenUsageSummary(ctx context.Context) (*db.TokenUsageSummary, error) {
	return s.db.TokenUsage.Summary(ctx)
}

// GetTokenUsageSummaryByConversation 返回指定会话的 token 用量总览。
func (s *Service) GetTokenUsageSummaryByConversation(ctx context.Context, convID string) (*db.TokenUsageSummary, error) {
	return s.db.TokenUsage.SummaryByConversation(ctx, convID)
}

// GetTokenUsageByProviderModel 按供应商和模型维度汇总 token 用量。
func (s *Service) GetTokenUsageByProviderModel(ctx context.Context) ([]*db.TokenUsageSummary, error) {
	return s.db.TokenUsage.SummaryByProviderModel(ctx)
}

func (s *Service) persistTokenUsage(
	ctx context.Context,
	conversationID string,
	providerID string,
	modelName string,
	mode string,
	collector *usage.Collector,
	duration time.Duration,
	status string,
	requestErr error,
) {
	snapshot := collector.Snapshot()
	record := &db.TokenUsageRecord{
		ID:               uuid.New().String(),
		ConversationID:   conversationID,
		Provider:         providerID,
		Model:            modelName,
		Mode:             mode,
		PromptTokens:     snapshot.PromptTokens,
		CompletionTokens: snapshot.CompletionTokens,
		TotalTokens:      snapshot.TotalTokens,
		ModelCalls:       snapshot.ModelCalls,
		DurationMS:       duration.Milliseconds(),
		Status:           status,
		CreatedAt:        time.Now(),
	}
	if requestErr != nil {
		record.Error = requestErr.Error()
	}
	if err := s.db.TokenUsage.Create(ctx, record); err != nil {
		slog.Warn("failed to persist token usage", "error", err)
	}
}
