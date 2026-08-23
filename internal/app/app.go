package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuyu-mind/backend/internal/agent"
	"github.com/yuyu-mind/backend/internal/ai/callback"
	aiProvider "github.com/yuyu-mind/backend/internal/ai/provider"
	"github.com/yuyu-mind/backend/internal/ai/tools"
	"github.com/yuyu-mind/backend/internal/chat"
	"github.com/yuyu-mind/backend/internal/config"
	"github.com/yuyu-mind/backend/internal/db"
	"github.com/yuyu-mind/backend/internal/memory"
	"github.com/yuyu-mind/backend/internal/plugin"
	pkgTypes "github.com/yuyu-mind/backend/pkg/types"
)

// App 是 Wails 暴露给前端的主应用对象。
type App struct {
	ctx           context.Context
	cfg           *config.Config
	db            *db.DB
	providerReg   *aiProvider.Registry
	toolReg       *tools.Registry
	workerToolReg *tools.Registry
	workspace     *tools.Workspace
	chatSvc       *chat.Service
	agentSvc      *agent.Service
	memorySvc     *memory.ServiceMemory
	memStore      memory.Store
	pluginMgr     *plugin.Manager
}

// New 创建 App 实例。
func New() *App {
	return &App{}
}

// Startup 在 Wails 应用启动时执行。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Configuration Error",
			Message: fmt.Sprintf("Failed to load configuration: %v", err),
		})
		return
	}
	a.cfg = cfg

	callback.Register(slog.Default())

	database, err := db.New(cfg.App.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Database Error",
			Message: fmt.Sprintf("Failed to open database: %v", err),
		})
		return
	}
	a.db = database

	a.providerReg = aiProvider.NewRegistry()
	a.providerReg.Register("openai", aiProvider.NewOpenAICompatFactory())
	a.providerReg.Register("deepseek", aiProvider.NewOpenAICompatFactory())
	a.providerReg.Register("moonshot", aiProvider.NewOpenAICompatFactory())
	a.providerReg.Register("bailian", aiProvider.NewOpenAICompatFactory())
	a.providerReg.Register("ollama", aiProvider.NewOllamaFactory())

	a.toolReg = tools.NewRegistry()
	a.toolReg.Register("web_search", tools.NewWebSearchTool())
	a.toolReg.Register("calculator", tools.NewCalculatorTool())

	// 文件系统工具：只读（list/read）注册进 Planner；write_file 只供 Worker（需审批）。
	workspaceRoot := cfg.App.WorkspaceRoot
	if strings.TrimSpace(workspaceRoot) == "" {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			workspaceRoot = home
		}
	}
	var ws *tools.Workspace
	if workspace, wsErr := tools.NewWorkspace(workspaceRoot); wsErr != nil {
		slog.Warn("failed to create file workspace; file tools disabled", "error", wsErr)
	} else {
		ws = workspace
		a.workspace = workspace
		a.toolReg.Register("list_files", tools.NewListFilesTool(ws))
		a.toolReg.Register("read_file", tools.NewReadFileTool(ws))
	}

	// Worker 工具集（含写文件与命令执行），仅 Worker 可调用；Planner 工具集不含这些危险工具。
	// 用注册表承载，便于插件在运行期向 Worker 注入工具。
	a.workerToolReg = tools.NewRegistry()
	if ws != nil {
		a.workerToolReg.Register("list_files", tools.NewListFilesTool(ws))
		a.workerToolReg.Register("read_file", tools.NewReadFileTool(ws))
		a.workerToolReg.Register("write_file", tools.NewWriteFileTool(ws))
		a.workerToolReg.Register("execute_command", tools.NewCommandTool(ws))
		a.workerToolReg.Register("screen_capture", tools.NewScreenCaptureTool(ws))
	}
	// 键鼠输入合成（仅 Worker，需审批）。
	a.workerToolReg.Register("send_input", tools.NewInputTool())

	a.memStore = memory.NewSQLiteStore(database.Messages, cfg.Memory.MaxTurns)
	a.memorySvc = memory.NewServiceMemory(database.Memories)
	a.chatSvc = chat.NewService(cfg, database, a.providerReg, a.toolReg, a.memStore, a.memorySvc)

	// Worker 执行器：有工作区时用真实 LLM 工具循环，否则回退默认（仅校验）。
	executor := agent.Executor(agent.NewDefaultExecutor())
	if ws != nil {
		executor = agent.NewLLMExecutor(a.workerModelFactory(), a.workerToolReg.GetAll)
	}
	a.agentSvc = agent.NewService(database, executor, slog.Default())
	a.agentSvc.SetNotifier(&taskNotifier{app: a})
	a.agentSvc.Start(ctx)

	// 聊天 → 任务闭环：Planner 识别到任务时经此提交异步任务。
	a.chatSvc.SetTaskSubmitter(&taskSubmitter{
		svc:            a.agentSvc,
		workspace:      workspaceRoot,
		defaultActions: []string{"list_files", "read_file"},
	})

	// 插件系统：插件注册的工具同时进入 Planner 与 Worker 工具集。
	a.pluginMgr = plugin.NewManager(func(name string, t tool.BaseTool) error {
		a.toolReg.Register(name, t)
		if a.workerToolReg != nil {
			a.workerToolReg.Register(name, t)
		}
		return nil
	}, slog.Default())
	a.pluginMgr.SetConfigStore(&settingsConfigStore{settings: database.Settings})
	if err := a.pluginMgr.Register(ctx, plugin.NewSystemPlugin()); err != nil {
		slog.Error("failed to register built-in plugin", "error", err)
	}
	if ws != nil {
		if err := a.pluginMgr.Register(ctx, plugin.NewWorkspacePlugin(ws)); err != nil {
			slog.Error("failed to register workspace plugin", "error", err)
		}
	}

	slog.Info("Yuyu Mind backend started",
		"provider", cfg.ActiveProvider.ProviderID,
		"model", cfg.ActiveProvider.Model,
	)
}

// Shutdown 在 Wails 应用关闭时执行。
func (a *App) Shutdown(ctx context.Context) {
	if a.agentSvc != nil {
		a.agentSvc.Stop()
	}
	if a.pluginMgr != nil {
		a.pluginMgr.StopAll(ctx)
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}
	slog.Info("Yuyu Mind backend stopped")
}

// StreamChat 发送聊天消息，并通过 Wails 事件流式返回响应。
func (a *App) StreamChat(req chat.ChatRequest) error {
	if err := a.ensureCompanionReady(); err != nil {
		return err
	}
	emitter := &wailsEmitter{ctx: a.ctx}
	return a.chatSvc.StreamChat(a.ctx, req, emitter)
}

// CreateConversation 创建新会话。
func (a *App) CreateConversation(title string) (*db.Conversation, error) {
	return a.chatSvc.CreateConversation(a.ctx, title)
}

// ListConversations 返回所有会话。
func (a *App) ListConversations() ([]*db.Conversation, error) {
	return a.chatSvc.ListConversations(a.ctx)
}

// DeleteConversation 删除会话。
func (a *App) DeleteConversation(id string) error {
	return a.chatSvc.DeleteConversation(a.ctx, id)
}

// GetMessages 返回指定会话的消息。
func (a *App) GetMessages(conversationID string) ([]*db.Message, error) {
	return a.chatSvc.GetMessages(a.ctx, conversationID)
}

// UpsertMemory 写入或更新长期记忆。
func (a *App) UpsertMemory(req memory.UpsertMemoryRequest) (*db.UserMemory, error) {
	return a.memorySvc.UpsertMemory(a.ctx, req)
}

// SearchMemories 检索长期记忆。
func (a *App) SearchMemories(scope, kind, query string, limit int) ([]*db.UserMemory, error) {
	return a.memorySvc.SearchMemories(a.ctx, scope, kind, query, limit)
}

// ArchiveMemory 归档长期记忆。
func (a *App) ArchiveMemory(memoryID string) error {
	return a.memorySvc.ArchiveMemory(a.ctx, memoryID)
}

// AddMemoryCandidate 添加候选记忆。
func (a *App) AddMemoryCandidate(req memory.AddCandidateRequest) (*db.MemoryCandidate, error) {
	return a.memorySvc.AddCandidate(a.ctx, req)
}

// ListMemoryCandidates 查询候选记忆。
func (a *App) ListMemoryCandidates(status string, limit int) ([]*db.MemoryCandidate, error) {
	return a.memorySvc.ListCandidates(a.ctx, status, limit)
}

// PromoteMemoryCandidate 将候选记忆转为长期记忆。
func (a *App) PromoteMemoryCandidate(candidateID string) (*db.UserMemory, error) {
	return a.memorySvc.PromoteCandidate(a.ctx, candidateID)
}

// RejectMemoryCandidate 拒绝候选记忆。
func (a *App) RejectMemoryCandidate(candidateID string) error {
	return a.memorySvc.RejectCandidate(a.ctx, candidateID)
}

// UpsertConversationSummary 写入或更新会话摘要。
func (a *App) UpsertConversationSummary(conversationID, summary string, tokenEstimate int) (*db.ConversationSummary, error) {
	return a.memorySvc.UpsertConversationSummary(a.ctx, conversationID, summary, tokenEstimate)
}

// BuildTaskContext 构建给底层 Worker 的任务级记忆投影，并保存快照。
func (a *App) BuildTaskContext(req memory.TaskContextRequest) (*memory.TaskContext, error) {
	return a.memorySvc.BuildTaskContext(a.ctx, req)
}

// SubmitAgentTask 提交异步执行任务，任务包应由顶层 Agent 注入用户偏好和上下文。
func (a *App) SubmitAgentTask(spec agent.TaskSpec) (*db.AgentTask, error) {
	return a.agentSvc.SubmitTask(a.ctx, spec)
}

// ListAgentTasks 查询任务列表。
func (a *App) ListAgentTasks(conversationID string, limit int) ([]*db.AgentTask, error) {
	return a.agentSvc.ListTasks(a.ctx, conversationID, limit)
}

// GetAgentTask 查询单个任务。
func (a *App) GetAgentTask(taskID string) (*db.AgentTask, error) {
	return a.agentSvc.GetTask(a.ctx, taskID)
}

// ListAgentTaskEvents 查询任务事件流。
func (a *App) ListAgentTaskEvents(taskID string) ([]*db.AgentTaskEvent, error) {
	return a.agentSvc.ListEvents(a.ctx, taskID)
}

// SendAgentTaskControl 向运行中任务发送控制消息。
func (a *App) SendAgentTaskControl(taskID, controlType string, payload map[string]any) (*db.AgentTaskControl, error) {
	return a.agentSvc.AddControl(a.ctx, taskID, controlType, payload)
}

// AnswerAgentTaskQuestion 回答 Worker 上报的问题。
func (a *App) AnswerAgentTaskQuestion(taskID, answer string) (*db.AgentTaskControl, error) {
	return a.agentSvc.AnswerTaskQuestion(a.ctx, taskID, answer)
}

// CancelAgentTask 取消任务。
func (a *App) CancelAgentTask(taskID string) (*db.AgentTaskControl, error) {
	return a.agentSvc.CancelTask(a.ctx, taskID)
}

// ListTokenUsageByConversation 返回指定会话的 token 用量明细。
func (a *App) ListTokenUsageByConversation(conversationID string) ([]*db.TokenUsageRecord, error) {
	return a.chatSvc.ListTokenUsageByConversation(a.ctx, conversationID)
}

// GetTokenUsageSummary 返回全局 token 用量总览。
func (a *App) GetTokenUsageSummary() (*db.TokenUsageSummary, error) {
	return a.chatSvc.GetTokenUsageSummary(a.ctx)
}

// GetTokenUsageSummaryByConversation 返回指定会话的 token 用量总览。
func (a *App) GetTokenUsageSummaryByConversation(conversationID string) (*db.TokenUsageSummary, error) {
	return a.chatSvc.GetTokenUsageSummaryByConversation(a.ctx, conversationID)
}

// GetTokenUsageByProviderModel 按供应商和模型维度汇总 token 用量。
func (a *App) GetTokenUsageByProviderModel() ([]*db.TokenUsageSummary, error) {
	return a.chatSvc.GetTokenUsageByProviderModel(a.ctx)
}

// GetActiveProvider 返回当前激活的模型供应商信息。
func (a *App) GetActiveProvider() pkgTypes.ProviderConfig {
	p, _ := a.cfg.GetActiveProviderConfig()
	return pkgTypes.ProviderConfig{
		ID:      a.cfg.ActiveProvider.ProviderID,
		Name:    p.Name,
		BaseURL: p.BaseURL,
		APIKey:  "",
		Model:   p.Model,
	}
}

// SetActiveProvider 切换当前激活的模型供应商和模型。
func (a *App) SetActiveProvider(providerID, model string) error {
	if err := a.cfg.SetActiveProvider(providerID, model); err != nil {
		return err
	}
	return a.cfg.Save()
}

// GetProviders 返回所有已配置的供应商信息，不包含 API Key。
func (a *App) GetProviders() map[string]pkgTypes.ProviderConfig {
	result := make(map[string]pkgTypes.ProviderConfig)
	for id, p := range a.cfg.Providers {
		result[id] = pkgTypes.ProviderConfig{
			ID:      id,
			Name:    p.Name,
			BaseURL: p.BaseURL,
			APIKey:  "",
			Model:   p.Model,
		}
	}
	return result
}

// UpdateProvider 更新供应商配置。
func (a *App) UpdateProvider(id string, cfg pkgTypes.ProviderConfig) error {
	return a.cfg.UpdateProvider(id, config.Provider{
		Name:    cfg.Name,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
}

type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) Emit(event chat.ChatEvent) {
	runtime.EventsEmit(e.ctx, "chat:event", event)
}
