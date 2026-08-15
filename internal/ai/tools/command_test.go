package tools

import (
	"context"
	"strings"
	"testing"
)

func TestCommandToolRun(t *testing.T) {
	ws, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	tool := NewCommandTool(ws)

	out, err := tool.InvokableRun(context.Background(), `{"command":"go","args":["version"]}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "go1.") {
		t.Fatalf("unexpected output %s", out)
	}
}

func TestCommandToolValidation(t *testing.T) {
	ws, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	tool := NewCommandTool(ws)

	if _, err := tool.InvokableRun(context.Background(), `{"command":""}`); err == nil {
		t.Fatalf("expected error for empty command")
	}
	if _, err := tool.InvokableRun(context.Background(), `{"command":"go","work_dir":"../outside"}`); err == nil {
		t.Fatalf("expected error for escaping work_dir")
	}
}
