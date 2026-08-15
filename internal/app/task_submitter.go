package app

import (
	"context"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuyu-mind/backend/internal/agent"
	"github.com/yuyu-mind/backend/internal/db"
)

// taskSubmitter 把 agent.Service 适配为 chat.TaskSubmitter，
// 并在提交时填充默认工作区与安全的默认允许动作（只读，不含 write_file）。
type taskSubmitter struct {
	svc            *agent.Service
	workspace      string
	defaultActions []string
}

func (t *taskSubmitter) SubmitTask(ctx context.Context, spec agent.TaskSpec) (*db.AgentTask, error) {
	if strings.TrimSpace(spec.Workspace) == "" {
		spec.Workspace = t.workspace
	}
	if len(spec.AllowedActions) == 0 {
		spec.AllowedActions = append([]string(nil), t.defaultActions...)
	}
	return t.svc.SubmitTask(ctx, spec)
}

// taskNotifier 把 agent 的任务变更事件推送到前端（Wails 事件）。
type taskNotifier struct {
	app *App
}

func (n *taskNotifier) NotifyTaskChanged(taskID string) {
	if n.app != nil && n.app.ctx != nil {
		runtime.EventsEmit(n.app.ctx, "agent:task:changed", map[string]any{"taskId": taskID})
	}
}
