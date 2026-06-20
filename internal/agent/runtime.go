package agent

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

var (
	errTaskCancelled    = errors.New("task cancelled")
	errTaskWaitingInput = errors.New("task waiting for input")
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
