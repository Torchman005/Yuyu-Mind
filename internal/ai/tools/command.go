package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultCommandTimeout = 30 * time.Second
	maxCommandTimeout     = 120 * time.Second
	maxCommandOutput      = 32 * 1024
)

// CommandInput 是 execute_command 的参数。
type CommandInput struct {
	Command   string   `json:"command" jsonschema:"description=Executable program name, e.g. git, go, cmd."`
	Args      []string `json:"args" jsonschema:"description=Arguments passed to the program."`
	WorkDir   string   `json:"work_dir" jsonschema:"description=Relative directory under the workspace to run in. Empty means workspace root."`
	TimeoutMS int      `json:"timeout_ms" jsonschema:"description=Timeout in milliseconds, default 30000, max 120000."`
}

// CommandTool 在工作区内执行命令（危险：默认应加入审批清单）。
type CommandTool struct {
	ws      *Workspace
	timeout time.Duration
}

// NewCommandTool 创建 execute_command 工具。
func NewCommandTool(ws *Workspace) *CommandTool {
	return &CommandTool{ws: ws, timeout: defaultCommandTimeout}
}

// Info 返回工具元数据。
func (t *CommandTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return attachSchema(&schema.ToolInfo{
		Name: "execute_command",
		Desc: "Run a command inside the workspace and return its combined output. Use only with explicit user approval.",
	}, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "Executable program name, e.g. git, go, cmd."},
			"args":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Arguments passed to the program."},
			"work_dir":   map[string]any{"type": "string", "description": "Relative directory under the workspace to run in. Empty means workspace root."},
			"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds, default 30000, max 120000."},
		},
		"required": []string{"command"},
	}), nil
}

// InvokableRun 在工作区目录内执行命令并返回合并输出。
func (t *CommandTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in CommandInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse execute_command arguments: %w", err)
	}
	if strings.TrimSpace(in.Command) == "" {
		return "", fmt.Errorf("command is required")
	}

	workDir := t.ws.Root()
	if strings.TrimSpace(in.WorkDir) != "" {
		resolved, err := t.ws.Resolve(in.WorkDir)
		if err != nil {
			return "", err
		}
		workDir = resolved
	}

	timeout := t.timeout
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	if timeout > maxCommandTimeout {
		timeout = maxCommandTimeout
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, in.Command, in.Args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()

	if cmdCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s", timeout)
	}

	truncated := false
	if len(output) > maxCommandOutput {
		output = output[:maxCommandOutput]
		truncated = true
	}

	result := struct {
		Command   string `json:"command"`
		WorkDir   string `json:"workDir"`
		ExitCode  int    `json:"exitCode"`
		Output    string `json:"output"`
		Truncated bool   `json:"truncated,omitempty"`
	}{
		Command:   in.Command,
		WorkDir:   relPath(t.ws.Root(), workDir),
		Output:    string(output),
		Truncated: truncated,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("run command: %w", err)
		}
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal execute_command result: %w", err)
	}
	return string(raw), nil
}
