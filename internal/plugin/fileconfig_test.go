package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestJSON(t *testing.T) {
	data := []byte(`{
		"schemaVersion": "1.0",
		"name": "hello",
		"displayName": "你好插件",
		"entry": "main.js",
		"runtime": "node",
		"actions": [{"name": "hi", "description": "greet"}],
		"tools": [{"name": "shout", "description": "uppercase", "inputSchema": {"type":"object"}}]
	}`)
	m, err := parseManifest(data, formatJSON)
	if err != nil {
		t.Fatalf("parse json manifest: %v", err)
	}
	if m.Name != "hello" || m.Entry != "main.js" || m.Runtime != "node" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if len(m.Actions) != 1 || m.Actions[0].Name != "hi" {
		t.Fatalf("actions mismatch: %+v", m.Actions)
	}
	if len(m.Tools) != 1 || m.Tools[0].Name != "shout" {
		t.Fatalf("tools mismatch: %+v", m.Tools)
	}
}

func TestParseManifestYAML(t *testing.T) {
	data := []byte(`schemaVersion: "1.0"
name: hello
displayName: 你好插件
entry: main.js
runtime: node
actions:
  - name: hi
    description: greet
tools:
  - name: shout
    description: uppercase
`)
	m, err := parseManifest(data, formatYAML)
	if err != nil {
		t.Fatalf("parse yaml manifest: %v", err)
	}
	if m.Name != "hello" || m.Runtime != "node" || len(m.Actions) != 1 || len(m.Tools) != 1 {
		t.Fatalf("unexpected yaml manifest: %+v", m)
	}
}

func TestParseManifestTOML(t *testing.T) {
	data := []byte(`schemaVersion = "1.0"
name = "hello"
displayName = "你好插件"
entry = "main.js"
runtime = "node"

[[actions]]
name = "hi"
description = "greet"

[[tools]]
name = "shout"
description = "uppercase"
`)
	m, err := parseManifest(data, formatTOML)
	if err != nil {
		t.Fatalf("parse toml manifest: %v", err)
	}
	if m.Name != "hello" || m.Runtime != "node" || len(m.Actions) != 1 || len(m.Tools) != 1 {
		t.Fatalf("unexpected toml manifest: %+v", m)
	}
}

func TestReadManifestFromDir(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"schemaVersion":"1.0","name":"demo","actions":[{"name":"ping","description":"heartbeat"}]}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Name != "demo" || len(m.Actions) != 1 {
		t.Fatalf("unexpected: %+v", m)
	}
}

func TestReadManifestMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := readManifest(dir); err == nil {
		t.Fatalf("expected error for dir without manifest")
	}
}

func TestFileConfigStoreRoundTripJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewFileConfigStore(map[string]string{"demo": dir})
	ctx := context.Background()

	if err := store.Set(ctx, "demo", map[string]any{"name": "小宇", "count": float64(3)}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	got, err := store.Get(ctx, "demo")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got["name"] != "小宇" {
		t.Fatalf("unexpected config: %+v", got)
	}

	// 未知插件返回空配置，不报错。
	empty, err := store.Get(ctx, "unknown")
	if err != nil {
		t.Fatalf("get unknown: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty config for unknown, got %+v", empty)
	}
}

func TestFileConfigStoreRoundTripTOMLDropNils(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(""), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	store := NewFileConfigStore(map[string]string{"demo": dir})
	ctx := context.Background()

	// nil 值应被剔除（TOML 不支持 null），不至于写出时报错。
	if err := store.Set(ctx, "demo", map[string]any{"a": "b", "skip": nil}); err != nil {
		t.Fatalf("set toml config: %v", err)
	}
	got, err := store.Get(ctx, "demo")
	if err != nil {
		t.Fatalf("get toml config: %v", err)
	}
	if got["a"] != "b" {
		t.Fatalf("unexpected toml config: %+v", got)
	}
	if _, exists := got["skip"]; exists {
		t.Fatalf("nil value should be dropped, got %+v", got)
	}
}
