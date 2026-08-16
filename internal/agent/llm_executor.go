package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ToolCallingModel 是 Worker 执行器依赖的最小工具调用模型接口。
// 宿主（app 层）负责把 Eino 的 model.ToolCallingChatModel 适配进来。
type ToolCallingModel interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
	WithTools(tools []*schema.ToolInfo) (ToolCallingModel, error)
}

// LLMExecutor 用工具调用模型循环执行结构化任务包。
// 它是 DefaultExecutor 的「真实」替代：真正调用模型 + 工具完成任务。
type LLMExecutor struct {
	modelFactory  func(ctx context.Context) (ToolCallingModel, error)
	toolProvider  func() []tool.BaseTool
	maxIterations int
}

// NewLLMExecutor 创建真实 Worker 执行器。
// modelFactory 每次执行时创建一个工具调用模型；toolProvider 每次执行时返回当前可用工具集
// （允许插件在运行期注册新工具），实际可用工具会按 task.AllowedActions 白名单过滤。
func NewLLMExecutor(modelFactory func(ctx context.Context) (ToolCallingModel, error), toolProvider func() []tool.BaseTool) *LLMExecutor {
	return &LLMExecutor{
		modelFactory:  modelFactory,
		toolProvider:  toolProvider,
		maxIterations: 12,
	}
}

// Execute 执行任务包，返回结构化结果。
func (e *LLMExecutor) Execute(ctx context.Context, task TaskSpec, runtime Runtime) (*TaskResult, error) {
	if err := runtime.CheckCancelled(ctx); err != nil {
		return nil, err
	}

	instructions := strings.TrimSpace(task.Instructions)
	if instructions == "" {
		answer, err := runtime.WaitForInput(ctx, QuestionPayload{
			Question: "底层执行任务缺少明确执行说明，需要补充要做什么。",
			Reason:   "instructions 为空；Worker 不读取长期记忆，也不直接向用户追问。",
		})
		if err != nil {
			return nil, err
		}
		instructions = answer
	}

	if err := runtime.CheckCancelled(ctx); err != nil {
		return nil, err
	}

	chatModel, err := e.modelFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("create worker model: %w", err)
	}

	var availableTools []tool.BaseTool
	if e.toolProvider != nil {
		availableTools = e.toolProvider()
	}
	allowedTools := filterToolsByActions(availableTools, task.AllowedActions)
	toolInfos, err := collectToolInfos(ctx, allowedTools)
	if err != nil {
		return nil, err
	}
	toolModel, err := chatModel.WithTools(toolInfos)
	if err != nil {
		return nil, fmt.Errorf("bind tools: %w", err)
	}

	if err := runtime.Emit(ctx, EventTypeProgress, "info", "Worker 开始执行任务包。", map[string]any{
		"allowed_actions": task.AllowedActions,
		"workspace":       task.Workspace,
		"tool_count":      len(toolInfos),
	}); err != nil {
		return nil, err
	}

	messages := buildTaskMessages(task, instructions)

	approvedForRun := false
	for i := 0; i < e.maxIterations; i++ {
		if err := runtime.CheckCancelled(ctx); err != nil {
			return nil, err
		}

		reply, err := toolModel.Generate(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("worker generate: %w", err)
		}

		if len(reply.ToolCalls) == 0 {
			summary := strings.TrimSpace(reply.Content)
			if summary == "" {
				summary = "任务已执行完成。"
			}
			if err := runtime.LogOperation(ctx, "llm_executor", task.Workspace, "Worker 完成任务。", "completed", task); err != nil {
				return nil, err
			}
			return &TaskResult{
				Summary:   summary,
				Artifacts: nil,
				Metadata:  map[string]any{"executor": "llm", "iterations": i + 1},
			}, nil
		}

		if !approvedForRun {
			if err := ensureDangerousApproval(ctx, reply, runtime); err != nil {
				return nil, err
			}
			approvedForRun = true
		}

		toolMessages, err := executeWorkerToolCalls(ctx, reply, allowedTools)
		if err != nil {
			return nil, err
		}
		for _, tm := range toolMessages {
			if tm.Role == schema.Tool {
				_ = runtime.Emit(ctx, EventTypeProgress, "info", "Worker 调用工具。", map[string]any{
					"tool_call_id": tm.ToolCallID,
					"result":       truncateForLog(tm.Content, 2000),
				})
			}
		}
		messages = append(messages, toolMessages...)
	}

	return nil, fmt.Errorf("task reached max tool iterations %d", e.maxIterations)
}

// approvalRequiredTools 是需要用户审批才允许执行的危险工具。
var approvalRequiredTools = map[string]bool{
	"write_file":      true,
	"execute_command": true,
	"send_input":      true,
	"screen_capture":  true,
}

// ensureDangerousApproval 在存在危险工具调用时请求审批（每轮一次）。
func ensureDangerousApproval(ctx context.Context, reply *schema.Message, runtime Runtime) error {
	for _, tc := range reply.ToolCalls {
		if !approvalRequiredTools[tc.Function.Name] {
			continue
		}
		approved, err := runtime.RequestApproval(ctx, ApprovalRequest{
			Action: tc.Function.Name,
			Target: approvalTarget(tc.Function.Name, tc.Function.Arguments),
			Reason: "该操作会修改工作区内的文件，需要用户批准。",
		})
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("operation %q was rejected by the user", tc.Function.Name)
		}
	}
	return nil
}

// approvalTarget 从工具参数里提取一个人类可读的操作对象（用于审批提示）。
func approvalTarget(name, argumentsJSON string) string {
	args, err := decodeJSON[map[string]any](argumentsJSON)
	if err != nil || args == nil {
		return ""
	}
	if path, ok := args["path"].(string); ok {
		return path
	}
	return ""
}

// buildTaskMessages 把任务包组装成模型输入消息。
func buildTaskMessages(task TaskSpec, instructions string) []*schema.Message {
	constraints := strings.Join(task.Constraints, "\n- ")
	if constraints == "" {
		constraints = "(none)"
	}
	allowed := strings.Join(task.AllowedActions, ", ")
	if allowed == "" {
		allowed = "(none)"
	}

	system := strings.TrimSpace(fmt.Sprintf(`You are a worker agent that executes a structured task package handed down by a top-level agent.
Follow the rules strictly:
- Do NOT read the user's long-term memory and do NOT ask the user questions directly.
- Work inside the given workspace only.
- Use only the tools listed in allowed actions. If a needed action is not allowed, report it in your final summary instead of doing it.
- When the task is done, stop calling tools and reply with a concise summary of what you did and what you produced.

Goal: %s
Workspace: %s
Constraints:
- %s
Allowed actions: %s`,
		nonEmpty(task.Goal, "(none)"),
		nonEmpty(task.Workspace, "(none)"),
		constraints,
		allowed,
	))

	user := strings.TrimSpace(fmt.Sprintf(`Instructions:
%s

Execute the task now.`, instructions))

	return []*schema.Message{
		{Role: schema.System, Content: system},
		{Role: schema.User, Content: user},
	}
}

// filterToolsByActions 按 allowed_actions 白名单过滤工具。
// allowed 为空时返回空集（安全默认：不静默放行任何工具）。
func filterToolsByActions(tools []tool.BaseTool, allowed []string) []tool.BaseTool {
	if len(allowed) == 0 {
		return nil
	}
	allow := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allow[strings.TrimSpace(name)] = true
	}

	// 这里需要工具名，Info 需 ctx；用 context.Background() 只读元数据是安全的。
	ctx := context.Background()
	result := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if allow[info.Name] {
			result = append(result, t)
		}
	}
	sortToolsByName(ctx, result)
	return result
}

func collectToolInfos(ctx context.Context, tools []tool.BaseTool) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("get tool info: %w", err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// executeWorkerToolCalls 执行模型返回的工具调用，返回追加到会话的工具消息。
func executeWorkerToolCalls(ctx context.Context, msg *schema.Message, tools []tool.BaseTool) ([]*schema.Message, error) {
	toolMap := make(map[string]tool.BaseTool, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}

	results := []*schema.Message{msg}
	for _, tc := range msg.ToolCalls {
		t, ok := toolMap[tc.Function.Name]
		if !ok {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				Content:    fmt.Sprintf("Error: tool %q is not allowed", tc.Function.Name),
				ToolCallID: tc.ID,
			})
			continue
		}
		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				Content:    fmt.Sprintf("Error: tool %q is not invokable", tc.Function.Name),
				ToolCallID: tc.ID,
			})
			continue
		}
		result, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
		if err != nil {
			result = fmt.Sprintf("Error executing tool %q: %v", tc.Function.Name, err)
		}
		results = append(results, &schema.Message{
			Role:       schema.Tool,
			Content:    result,
			ToolCallID: tc.ID,
		})
	}
	return results, nil
}

func sortToolsByName(ctx context.Context, tools []tool.BaseTool) {
	names := make(map[tool.BaseTool]string, len(tools))
	for _, t := range tools {
		if info, err := t.Info(ctx); err == nil {
			names[t] = info.Name
		}
	}
	sort.SliceStable(tools, func(i, j int) bool {
		return names[tools[i]] < names[tools[j]]
	})
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func truncateForLog(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}
