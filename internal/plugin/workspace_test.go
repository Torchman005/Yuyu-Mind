package plugin

import (
	"context"
	"testing"

	"github.com/yuyu-mind/backend/internal/ai/tools"
)

func TestWorkspacePluginRoundTrip(t *testing.T) {
	ws, err := tools.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	p := NewWorkspacePlugin(ws)
	ctx := context.Background()

	writeOut, err := p.write(ctx, map[string]any{"path": "sub/notes.md", "content": "hello"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if writeOut["written"] != true {
		t.Fatalf("unexpected write result %v", writeOut)
	}

	readOut, err := p.read(ctx, map[string]any{"path": "sub/notes.md"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if readOut["content"] != "hello" {
		t.Fatalf("unexpected read result %v", readOut)
	}

	listOut, err := p.list(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	items, _ := listOut["items"].([]map[string]any)
	if len(items) != 1 || items[0]["name"] != "sub" {
		t.Fatalf("unexpected list result %v", listOut)
	}

	if _, err := p.read(ctx, map[string]any{"path": "../secret.txt"}); err == nil {
		t.Fatalf("expected read escape error")
	}
	if _, err := p.write(ctx, map[string]any{"path": "../escape.txt", "content": "x"}); err == nil {
		t.Fatalf("expected write escape error")
	}
}

func TestWorkspacePluginViaManager(t *testing.T) {
	ws, err := tools.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	m := NewManager(nil, nil)
	if err := m.Register(context.Background(), NewWorkspacePlugin(ws)); err != nil {
		t.Fatalf("register: %v", err)
	}
	out, err := m.InvokeAction(context.Background(), "workspace", "list", map[string]any{})
	if err != nil {
		t.Fatalf("invoke list: %v", err)
	}
	if out["items"] == nil {
		t.Fatalf("unexpected result %v", out)
	}
}
