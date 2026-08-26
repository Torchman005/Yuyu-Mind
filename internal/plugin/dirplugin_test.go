package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// fakeHost 记录插件在 Init 阶段注册的工具与动作。
type fakeHost struct {
	tools   []string
	actions []string
}

func (f *fakeHost) RegisterTool(name string, t tool.BaseTool) error {
	f.tools = append(f.tools, name)
	return nil
}
func (f *fakeHost) RegisterAction(name string, h ActionHandler) error {
	f.actions = append(f.actions, name)
	return nil
}
func (f *fakeHost) Config() map[string]any          { return map[string]any{} }
func (f *fakeHost) Logf(format string, args ...any) {}

func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestDirPluginInitRegistersStubs(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{
		"schemaVersion":"1.0",
		"name":"demo",
		"displayName":"演示",
		"entry":"main.js",
		"runtime":"node",
		"actions":[{"name":"ping","description":"heartbeat"}],
		"tools":[{"name":"shout","description":"uppercase"}]
	}`)

	p, err := NewDirPlugin(dir)
	if err != nil {
		t.Fatalf("new dir plugin: %v", err)
	}
	if p.Manifest().Name != "demo" {
		t.Fatalf("unexpected name: %s", p.Manifest().Name)
	}

	host := &fakeHost{}
	if err := p.Init(context.Background(), &Host{
		RegisterTool:   host.RegisterTool,
		RegisterAction: host.RegisterAction,
		Config:         host.Config,
		Logf:           host.Logf,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(host.actions) != 1 || host.actions[0] != "ping" {
		t.Fatalf("actions not registered: %v", host.actions)
	}
	if len(host.tools) != 1 || host.tools[0] != "shout" {
		t.Fatalf("tools not registered: %v", host.tools)
	}
}

func TestDirPluginEnsureClientWithoutEntry(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{"schemaVersion":"1.0","name":"noentry","actions":[{"name":"x"}]}`)
	p, err := NewDirPlugin(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := p.ensureClient(context.Background()); err == nil {
		t.Fatalf("expected error when no entry is declared")
	}
}

func TestDiscoverPluginDirs(t *testing.T) {
	root := t.TempDir()
	hello := filepath.Join(root, "hello")
	if err := os.MkdirAll(hello, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeManifest(t, hello, `{"schemaVersion":"1.0","name":"hello","entry":"main.js"}`)
	// 一个没有元数据文件的目录，应被跳过。
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	plugins, errs := DiscoverPluginDirs(root)
	if len(plugins) != 1 || plugins[0].Manifest().Name != "hello" {
		t.Fatalf("unexpected discovered plugins: %+v", plugins)
	}
	_ = errs
}

// TestDiscoverShipPlugins 校验仓库自带的两个示例插件目录能被正确发现并解析。
func TestDiscoverShipPlugins(t *testing.T) {
	root := filepath.Join("..", "..", "plugins")
	plugins, errs := DiscoverPluginDirs(root)
	if len(errs) > 0 {
		t.Fatalf("discover ship plugins: %v", errs)
	}
	got := map[string]bool{}
	for _, p := range plugins {
		got[p.Manifest().Name] = true
	}
	for _, want := range []string{"hello", "code-assistant"} {
		if !got[want] {
			t.Fatalf("ship plugin %q not discovered; got %v", want, got)
		}
	}
	// code-assistant 应声明 1 个工具 + 6 个动作（show/list/open diff、accept/reject、open_workspace）。
	for _, p := range plugins {
		if p.Manifest().Name == "code-assistant" {
			if len(p.Manifest().Tools) != 1 || len(p.Manifest().Actions) != 6 {
				t.Fatalf("code-assistant manifest wrong: tools=%d actions=%d", len(p.Manifest().Tools), len(p.Manifest().Actions))
			}
		}
	}
}

func TestDirPluginNodeRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed; skipping sidecar round-trip")
	}
	dir := filepath.Join("..", "..", "plugins", "hello")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "main.js")); err != nil {
		t.Skip("hello plugin present? skipping round-trip")
	}

	p, err := NewDirPlugin(abs)
	if err != nil {
		t.Fatalf("new dir plugin: %v", err)
	}
	defer p.Stop(context.Background())

	// 动作 round-trip。
	res, err := p.invoke(context.Background(), "invoke_action", map[string]any{
		"action": "hello",
		"input":  map[string]any{},
	})
	if err != nil {
		t.Fatalf("invoke hello action: %v", err)
	}
	if msg, _ := res["message"].(string); msg == "" {
		t.Fatalf("expected message, got %+v", res)
	}

	// 工具 round-trip。
	tres, err := p.invoke(context.Background(), "invoke_tool", map[string]any{
		"tool":      "shout",
		"arguments": `{"text":"hi"}`,
	})
	if err != nil {
		t.Fatalf("invoke shout tool: %v", err)
	}
	if out, _ := tres["result"].(string); out != "HI" {
		t.Fatalf("expected HI, got %+v", tres)
	}
}
