package agent

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

var (
	errTaskCancelled        = errors.New("task cancelled")
	errTaskWaitingInput     = errors.New("task waiting for input")
	errTaskWaitingApproval  = errors.New("task waiting for approval")
)

type taskRuntime struct {
	service *Service
	taskID  string
}

func (r *taskRuntime) Emit(ctx context.Context, eventType, level, message string, payload any) error {
	return r.service.addEvent(ctx, r.taskID, eventType, level, message, payload)
}

func (r *taskRuntime) LogOperation(ctx context.Context, kind, target, summary, status string, payload any) error {
	if err := r.service.db.AgentEvents.AddOperation(ctx, &db.AgentOperationLog{
		ID:        uuid.New().String(),
		TaskID:    r.taskID,
		Kind:      kind,
		Target:    target,
		Summary:   summary,
		Status:    status,
		Payload:   encodeJSON(payload),
		CreatedAt: time.Now(),
	}); err != nil {
		return err
	}
	return r.Emit(ctx, EventTypeOperation, "info", summary, map[string]any{
		"kind":   kind,
		"target": target,
		"status": status,
	})
}

func (r *taskRuntime) WaitForInput(ctx context.Context, question QuestionPayload) (string, error) {
	answer, ok, err := r.consumeInputControl(ctx)
	if err != nil {
		return "", err
	}
	if ok {
		_ = r.Emit(ctx, EventTypeControl, "info", "Worker 已读取顶层补充信息，继续执行。", InputPayload{Answer: answer})
		return answer, nil
	}

	if err := r.service.addEvent(ctx, r.taskID, EventTypeQuestion, "warn", question.Question, question); err != nil {
		return "", err
	}
	if err := r.service.db.AgentTasks.UpdateStatus(ctx, r.taskID, TaskStatusWaitingForInput, "", "", nil, nil); err != nil {
		return "", err
	}
	r.service.notifyChanged(r.taskID)
	return "", errTaskWaitingInput
}

func (r *taskRuntime) CheckCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	controls, err := r.service.db.AgentEvents.PendingControls(ctx, r.taskID)
	if err != nil {
		return err
	}
	for _, control := range controls {
		if control.Type == ControlTypeCancel {
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			return errTaskCancelled
		}
	}
	return nil
}

// RequestApproval 请求用户审批一个危险操作。
// 返回 (approved, nil)：已批准/已拒绝；返回 (false, errTaskWaitingApproval)：已挂起等待审批。
func (r *taskRuntime) RequestApproval(ctx context.Context, request ApprovalRequest) (bool, error) {
	// 先消费已存在的 approve/reject 控制（任务被重新调度后的第二次进入）。
	controls, err := r.service.db.AgentEvents.PendingControls(ctx, r.taskID)
	if err != nil {
		return false, err
	}
	for _, control := range controls {
		switch control.Type {
		case ControlTypeApprove:
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			_ = r.Emit(ctx, EventTypeControl, "info", "Worker 已读取审批结果：批准。", nil)
			return true, nil
		case ControlTypeReject:
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			_ = r.Emit(ctx, EventTypeControl, "warn", "Worker 已读取审批结果：拒绝。", nil)
			return false, nil
		case ControlTypeCancel:
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			return false, errTaskCancelled
		}
	}

	// 尚无决定：写问题事件并挂起。
	question := request.Action
	if request.Target != "" {
		question += " " + request.Target
	}
	if request.Reason != "" {
		question += "：" + request.Reason
	}
	if err := r.service.addEvent(ctx, r.taskID, EventTypeQuestion, "warn", "需要审批："+question, request); err != nil {
		return false, err
	}
	if err := r.service.db.AgentTasks.UpdateStatus(ctx, r.taskID, TaskStatusWaitingForApproval, "", "", nil, nil); err != nil {
		return false, err
	}
	r.service.notifyChanged(r.taskID)
	return false, errTaskWaitingApproval
}

func (r *taskRuntime) consumeInputControl(ctx context.Context) (string, bool, error) {
	controls, err := r.service.db.AgentEvents.PendingControls(ctx, r.taskID)
	if err != nil {
		return "", false, err
	}
	for _, control := range controls {
		switch control.Type {
		case ControlTypeCancel:
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			return "", false, errTaskCancelled
		case ControlTypeInput:
			payload, err := decodeJSON[InputPayload](control.Payload)
			if err != nil {
				return "", false, err
			}
			_ = r.service.db.AgentEvents.MarkControlApplied(ctx, control.ID)
			return payload.Answer, true, nil
		}
	}
	return "", false, nil
}
