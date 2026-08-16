package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// runSidecarProcess 在被 re-exec 的子进程中扮演 sidecar：读 stdin 的 JSON-RPC，写 stdout 响应。
func runSidecarProcess() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		resp := map[string]any{"id": req.ID}
		switch req.Method {
		case "manifest":
			resp["result"] = map[string]any{
				"schemaVersion": "1.0",
				"name":          "demo",
				"displayName":   "Demo Sidecar",
				"description":   "test sidecar",
				"version":       "0.1.0",
				"author":        "test",
				"entry":         "sidecar",
				"permissions":   []string{},
				"actions":       []map[string]any{{"name": "echo", "description": "echo input.text"}},
			}
		case "start", "stop":
			resp["result"] = map[string]any{"ok": true}
		case "invoke_action":
			input, _ := req.Params["input"].(map[string]any)
			resp["result"] = map[string]any{"echo": input["text"]}
		default:
			resp["error"] = map[string]any{"message": "unknown method " + req.Method}
		}
		data, _ := json.Marshal(resp)
		fmt.Fprintln(os.Stdout, string(data))
	}
	os.Exit(0)
}

func TestSidecarPluginLifecycle(t *testing.T) {
	if os.Getenv("YUYU_SIDECAR") == "1" {
		runSidecarProcess()
		return
	}

	spec := SidecarSpec{
		Name:    "demo",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestSidecarPluginLifecycle"},
		Env:     []string{"YUYU_SIDECAR=1"},
	}

	m := NewManager(nil, nil)
	if err := m.Register(context.Background(), NewSidecarPlugin(spec)); err != nil {
		t.Fatalf("register sidecar: %v", err)
	}
	defer m.StopAll(context.Background())

	statuses := m.List()
	if len(statuses) != 1 || statuses[0].Manifest.Name != "demo" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	out, err := m.InvokeAction(context.Background(), "demo", "echo", map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("invoke echo: %v", err)
	}
	if out["echo"] != "hi" {
		t.Fatalf("unexpected result %v", out)
	}
}
