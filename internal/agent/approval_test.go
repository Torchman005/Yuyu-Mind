package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/yuyu-mind/backend/internal/db"
)

// approvalExecutor 第一次请求审批并挂起，批准后完成任务。
type approvalExecutor struct {
	executions int
}

func (e *approvalExecutor) Execute(ctx context.Context, task TaskSpec, runtime Runtime) (*TaskResult, error) {
	e.executions++
	approved, err := runtime.RequestApproval(ctx, ApprovalRequest{
		Action: "write_file",
		Target: "notes.md",
		Reason: "写文件",
	})
	if err != nil {
		return nil, err
	}
	if !approved {
		return nil, fmt.Errorf("operation rejected")
	}
	return &TaskResult{Summary: "已写入 notes.md"}, nil
}

func TestApprovalFlow(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	executor := &approvalExecutor{}
	svc := NewService(database, executor, slog.Default())
	svc.Start(context.Background())
	defer svc.Stop()

	task, err := svc.SubmitTask(context.Background(), TaskSpec{
		Title:          "写文件",
		Goal:           "创建 notes.md",
		Instructions:   "写 notes.md",
		AllowedActions: []string{"write_file"},
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}

	waitForTaskStatus(t, svc, task.ID, TaskStatusWaitingForApproval)

	if _, err := svc.AddControl(context.Background(), task.ID, ControlTypeApprove, nil); err != nil {
		t.Fatalf("AddControl approve: %v", err)
	}

	waitForTaskStatus(t, svc, task.ID, TaskStatusCompleted)

	if executor.executions < 2 {
		t.Fatalf("expected at least 2 executions (request + approved run), got %d", executor.executions)
	}
}

func waitForTaskStatus(t *testing.T, svc *Service, taskID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := svc.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if current != nil && current.Status == want {
			return
		}
		if current != nil && (current.Status == TaskStatusFailed || current.Status == TaskStatusCancelled) {
			t.Fatalf("task reached %s (error=%s) while waiting for %s", current.Status, current.Error, want)
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s did not reach status %s (current=%s)", taskID, want, currentStatus(current))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func currentStatus(task *db.AgentTask) string {
	if task == nil {
		return "(nil)"
	}
	return task.Status
}
