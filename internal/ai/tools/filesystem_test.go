package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceResolve(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}

	// 相对路径应落在工作区内。
	got, err := ws.Resolve("a/b.txt")
	if err != nil {
		t.Fatalf("resolve relative: %v", err)
	}
	if want := filepath.Join(ws.Root(), "a", "b.txt"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// 工作区内的绝对路径应通过。
	if _, err := ws.Resolve(filepath.Join(ws.Root(), "x.txt")); err != nil {
		t.Fatalf("resolve abs inside: %v", err)
	}

	// `..` 逃逸应被拒绝。
	if _, err := ws.Resolve("../outside.txt"); err == nil {
		t.Fatalf("expected escape error for ../")
	}

	// 工作区外的绝对路径应被拒绝。
	outside := t.TempDir()
	if _, err := ws.Resolve(outside); err == nil {
		t.Fatalf("expected escape error for outside absolute path")
	}
}

func TestWorkspaceResolveSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if _, err := ws.Resolve("link"); err == nil {
		t.Fatalf("expected symlink escape error")
	}
}

func TestFileToolsRoundTrip(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	ctx := context.Background()

	write := NewWriteFileTool(ws)
	writeOut, err := write.InvokableRun(ctx, `{"path":"sub/notes.md","content":"hello 世界"}`)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(writeOut, "written") {
		t.Fatalf("unexpected write result %s", writeOut)
	}

	read := NewReadFileTool(ws)
	readOut, err := read.InvokableRun(ctx, `{"path":"sub/notes.md"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(readOut, "hello 世界") {
		t.Fatalf("unexpected read result %s", readOut)
	}

	list := NewListFilesTool(ws)
	listOut, err := list.InvokableRun(ctx, `{"path":"","recursive":true}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listOut, "notes.md") {
		t.Fatalf("unexpected list result %s", listOut)
	}

	// 越界写应被拒绝。
	if _, err := write.InvokableRun(ctx, `{"path":"../escape.txt","content":"x"}`); err == nil {
		t.Fatalf("expected write escape error")
	}
}
