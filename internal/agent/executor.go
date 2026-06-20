package agent

import (
	"context"
	"fmt"
	"strings"
)

type Runtime interface {
	Emit(ctx context.Context, eventType, level, message string, payload any) error
	LogOperation(ctx context.Context, kind, target, summary, status string, payload any) error
	WaitForInput(ctx context.Context, question QuestionPayload) (string, error)
	CheckCancelled(ctx context.Context) error
}

type Executor interface {
	Execute(ctx context.Context, task TaskSpec, runtime Runtime) (*TaskResult, error)
}

type DefaultExecutor struct{}

func NewDefaultExecutor() *DefaultExecutor {
	return &DefaultExecutor{}
}

func (e *DefaultExecutor) Execute(ctx context.Context, task TaskSpec, runtime Runtime) (*TaskResult, error) {
	if err := runtime.CheckCancelled(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.Instructions) == "" {
		answer, err := runtime.WaitForInput(ctx, QuestionPayload{
			Question: "底层执行任务缺少明确执行说明，需要补充要做什么。",
			Reason:   "instructions 为空；Worker 不读取长期记忆，也不直接向用户追问。",
		})
		if err != nil {
			return nil, err
		}
		task.Instructions = answer
	}

	if err := runtime.CheckCancelled(ctx); err != nil {
		return nil, err
	}
	if err := runtime.Emit(ctx, EventTypeProgress, "info", "Worker 已接收任务包并完成执行前校验。", map[string]any{
		"allowed_actions": task.AllowedActions,
		"workspace":       task.Workspace,
	}); err != nil {
		return nil, err
	}
	if err := runtime.LogOperation(ctx, "executor", task.Workspace, "默认执行器完成任务包校验，等待接入真实执行 Agent。", "completed", task); err != nil {
		return nil, err
	}

	return &TaskResult{
		Summary: fmt.Sprintf("任务已进入异步执行链路：%s", task.Goal),
		Metadata: map[string]any{
			"executor": "default",
		},
	}, nil
}
