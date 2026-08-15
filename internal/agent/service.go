package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

type runningTask struct {
	cancel context.CancelFunc
}

// Notifier 在任务状态或事件发生变化时通知宿主（用于推送到前端）。
type Notifier interface {
	NotifyTaskChanged(taskID string)
}

type Service struct {
	db       *db.DB
	executor Executor
	logger   *slog.Logger
	notifier Notifier

	cancel context.CancelFunc
	wg     sync.WaitGroup

	wakeup      chan struct{}
	workerSlots chan struct{}

	runningMu sync.Mutex
	running   map[string]runningTask
}

func NewService(database *db.DB, executor Executor, logger *slog.Logger) *Service {
	if executor == nil {
		executor = NewDefaultExecutor()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:          database,
		executor:    executor,
		logger:      logger,
		wakeup:      make(chan struct{}, 1),
		workerSlots: make(chan struct{}, 4),
		running:     make(map[string]runningTask),
	}
}

// SetNotifier 注入任务变更通知器（可选；未注入时不推送）。
func (s *Service) SetNotifier(notifier Notifier) {
	s.notifier = notifier
}

func (s *Service) notifyChanged(taskID string) {
	if s.notifier != nil && taskID != "" {
		s.notifier.NotifyTaskChanged(taskID)
	}
}

func (s *Service) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go s.loop(runCtx)
	s.notifyScheduler()
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}

	s.runningMu.Lock()
	for _, task := range s.running {
		task.cancel()
	}
	s.runningMu.Unlock()

	s.wg.Wait()
}

func (s *Service) SubmitTask(ctx context.Context, spec TaskSpec) (*db.AgentTask, error) {
	if strings.TrimSpace(spec.Goal) == "" {
		return nil, fmt.Errorf("goal is required")
	}
	if strings.TrimSpace(spec.Title) == "" {
		spec.Title = spec.Goal
	}

	now := time.Now()
	task := &db.AgentTask{
		ID:             uuid.New().String(),
		ConversationID: spec.ConversationID,
		ParentTaskID:   spec.ParentTaskID,
		Title:          spec.Title,
		Goal:           spec.Goal,
		Status:         TaskStatusQueued,
		Priority:       spec.Priority,
		SpecJSON:       encodeJSON(spec),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.db.AgentTasks.Create(ctx, task); err != nil {
		return nil, err
	}
	_ = s.addEvent(ctx, task.ID, EventTypeAudit, "info", "顶层 Agent 已创建异步任务。", spec)
	s.notifyScheduler()
	return task, nil
}

func (s *Service) ListTasks(ctx context.Context, conversationID string, limit int) ([]*db.AgentTask, error) {
	return s.db.AgentTasks.List(ctx, conversationID, limit)
}

func (s *Service) GetTask(ctx context.Context, taskID string) (*db.AgentTask, error) {
	return s.db.AgentTasks.Get(ctx, taskID)
}

func (s *Service) ListEvents(ctx context.Context, taskID string) ([]*db.AgentTaskEvent, error) {
	return s.db.AgentEvents.ListEvents(ctx, taskID)
}

func (s *Service) AddControl(ctx context.Context, taskID, controlType string, payload any) (*db.AgentTaskControl, error) {
	control := &db.AgentTaskControl{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Type:      controlType,
		Payload:   encodeJSON(payload),
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	if err := s.db.AgentEvents.AddControl(ctx, control); err != nil {
		return nil, err
	}
	_ = s.addEvent(ctx, taskID, EventTypeControl, "info", "顶层 Agent 已下发控制消息。", control)

	switch controlType {
	case ControlTypeCancel:
		if !s.cancelRunningTask(taskID) {
			_ = s.cancelNonRunningTask(ctx, taskID, control.ID)
		}
	case ControlTypeInput, ControlTypeApprove, ControlTypeReject, ControlTypeRevise:
		_ = s.requeueWaitingTask(ctx, taskID)
		s.notifyScheduler()
	}

	return control, nil
}

func (s *Service) AnswerTaskQuestion(ctx context.Context, taskID, answer string) (*db.AgentTaskControl, error) {
	return s.AddControl(ctx, taskID, ControlTypeInput, InputPayload{Answer: answer})
}

func (s *Service) CancelTask(ctx context.Context, taskID string) (*db.AgentTaskControl, error) {
	return s.AddControl(ctx, taskID, ControlTypeCancel, nil)
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wakeup:
			s.drainQueue(ctx)
		case <-ticker.C:
			s.drainQueue(ctx)
		}
	}
}

func (s *Service) drainQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case s.workerSlots <- struct{}{}:
			task, err := s.db.AgentTasks.ClaimNextQueued(ctx)
			if err != nil {
				<-s.workerSlots
				s.logger.Warn("claim agent task failed", "error", err)
				return
			}
			if task == nil {
				<-s.workerSlots
				return
			}

			taskCtx, cancel := context.WithCancel(ctx)
			s.registerRunningTask(task.ID, cancel)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer s.notifyScheduler()
				defer func() { <-s.workerSlots }()
				defer s.unregisterRunningTask(task.ID)
				defer cancel()
				s.runTask(taskCtx, task)
			}()
		default:
			return
		}
	}
}

func (s *Service) runTask(ctx context.Context, task *db.AgentTask) {
	_ = s.addEvent(ctx, task.ID, EventTypeProgress, "info", "Worker 开始执行任务。", nil)

	spec, err := decodeJSON[TaskSpec](task.SpecJSON)
	if err != nil {
		s.failTask(ctx, task.ID, fmt.Errorf("decode task spec: %w", err))
		return
	}

	runtime := &taskRuntime{service: s, taskID: task.ID}
	result, err := s.executor.Execute(ctx, spec, runtime)
	if err != nil {
		switch {
		case errors.Is(err, errTaskCancelled), errors.Is(err, context.Canceled):
			completedAt := time.Now()
			_ = s.db.AgentTasks.UpdateStatus(context.Background(), task.ID, TaskStatusCancelled, err.Error(), "", nil, &completedAt)
			_ = s.addEvent(context.Background(), task.ID, EventTypeAudit, "warn", "任务已取消。", nil)
		case errors.Is(err, errTaskWaitingInput):
			_ = s.addEvent(context.Background(), task.ID, EventTypeAudit, "info", "任务已挂起，等待顶层 Agent 补充信息。", nil)
		case errors.Is(err, errTaskWaitingApproval):
			_ = s.addEvent(context.Background(), task.ID, EventTypeAudit, "info", "任务已挂起，等待审批。", nil)
		default:
			s.failTask(context.Background(), task.ID, err)
		}
		return
	}

	completedAt := time.Now()
	resultJSON := encodeJSON(result)
	if err := s.db.AgentTasks.UpdateStatus(ctx, task.ID, TaskStatusCompleted, "", resultJSON, nil, &completedAt); err != nil {
		s.logger.Warn("complete agent task failed", "task_id", task.ID, "error", err)
		return
	}
	_ = s.addEvent(ctx, task.ID, EventTypeResult, "info", "Worker 完成任务。", result)
}

func (s *Service) failTask(ctx context.Context, taskID string, err error) {
	completedAt := time.Now()
	_ = s.db.AgentTasks.UpdateStatus(ctx, taskID, TaskStatusFailed, err.Error(), "", nil, &completedAt)
	_ = s.addEvent(ctx, taskID, EventTypeError, "error", err.Error(), nil)
}

func (s *Service) addEvent(ctx context.Context, taskID, eventType, level, message string, payload any) error {
	err := s.db.AgentEvents.AddEvent(ctx, &db.AgentTaskEvent{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Type:      eventType,
		Level:     level,
		Message:   message,
		Payload:   encodeJSON(payload),
		CreatedAt: time.Now(),
	})
	if err == nil {
		s.notifyChanged(taskID)
	}
	return err
}

func (s *Service) notifyScheduler() {
	select {
	case s.wakeup <- struct{}{}:
	default:
	}
}

func (s *Service) registerRunningTask(taskID string, cancel context.CancelFunc) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	s.running[taskID] = runningTask{cancel: cancel}
}

func (s *Service) unregisterRunningTask(taskID string) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	delete(s.running, taskID)
}

func (s *Service) cancelRunningTask(taskID string) bool {
	s.runningMu.Lock()
	task, ok := s.running[taskID]
	s.runningMu.Unlock()
	if ok {
		task.cancel()
	}
	return ok
}

func (s *Service) cancelNonRunningTask(ctx context.Context, taskID, controlID string) error {
	task, err := s.db.AgentTasks.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", taskID)
	}

	switch task.Status {
	case TaskStatusQueued, TaskStatusWaitingForInput, TaskStatusWaitingForApproval:
		completedAt := time.Now()
		if err := s.db.AgentTasks.UpdateStatus(ctx, taskID, TaskStatusCancelled, "task cancelled", "", nil, &completedAt); err != nil {
			return err
		}
		_ = s.db.AgentEvents.MarkControlApplied(ctx, controlID)
		return s.addEvent(ctx, taskID, EventTypeAudit, "warn", "任务已取消。", nil)
	default:
		return nil
	}
}

func (s *Service) requeueWaitingTask(ctx context.Context, taskID string) error {
	task, err := s.db.AgentTasks.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", taskID)
	}
	if task.Status != TaskStatusWaitingForInput && task.Status != TaskStatusWaitingForApproval {
		return nil
	}
	if err := s.db.AgentTasks.UpdateStatus(ctx, taskID, TaskStatusQueued, "", "", nil, nil); err != nil {
		return err
	}
	return s.addEvent(ctx, taskID, EventTypeControl, "info", "任务已收到补充信息，重新进入队列。", nil)
}
