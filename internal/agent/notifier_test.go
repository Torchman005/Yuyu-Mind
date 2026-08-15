package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yuyu-mind/backend/internal/db"
)

type fakeNotifier struct {
	mu      sync.Mutex
	taskIDs map[string]bool
}

func (n *fakeNotifier) NotifyTaskChanged(taskID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.taskIDs == nil {
		n.taskIDs = make(map[string]bool)
	}
	n.taskIDs[taskID] = true
}

func (n *fakeNotifier) sawTask(id string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.taskIDs[id]
}

func TestNotifierOnTaskLifecycle(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "notifier.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	svc := NewService(database, NewDefaultExecutor(), slog.Default())
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	svc.Start(context.Background())
	defer svc.Stop()

	task, err := svc.SubmitTask(context.Background(), TaskSpec{
		Title:        "测试任务",
		Goal:         "完成一个测试任务",
		Instructions: "执行前校验并通过。",
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		current, getErr := svc.GetTask(context.Background(), task.ID)
		if getErr != nil {
			t.Fatalf("GetTask: %v", getErr)
		}
		if current != nil && (current.Status == TaskStatusCompleted || current.Status == TaskStatusFailed || current.Status == TaskStatusCancelled) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s did not reach terminal state", task.ID)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !notifier.sawTask(task.ID) {
		t.Fatalf("notifier did not observe task %s", task.ID)
	}
}
