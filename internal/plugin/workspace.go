package plugin

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuyu-mind/backend/internal/ai/tools"
)

// WorkspacePlugin 是内置示例插件：把「工作区文件操作」暴露为插件动作，
// 供前端插件面板调用（用户主动触发，区别于 Worker 的 LLM 工具调用 + 审批流）。
type WorkspacePlugin struct {
	ws *tools.Workspace
}

// NewWorkspacePlugin 创建工作区文件插件。ws 为 nil 时仅返回 manifest、动作不可用。
func NewWorkspacePlugin(ws *tools.Workspace) *WorkspacePlugin {
	return &WorkspacePlugin{ws: ws}
}

// Manifest 返回插件元数据。
func (p *WorkspacePlugin) Manifest() Manifest {
	return Manifest{
		SchemaVersion: "1.0",
		Name:          "workspace",
		DisplayName:   "工作区文件",
		Description:   "内置插件：浏览、读取、写入工作区内的文件。",
		Version:       "0.1.0",
		Author:        "Yuyu Mind",
		Entry:         "builtin",
		Permissions:   []string{"workspace.read", "workspace.write"},
		Actions: []Action{
			{Name: "list", Description: "列出工作区目录内容（input: path 可选）。"},
			{Name: "read", Description: "读取工作区文件内容（input: path）。"},
			{Name: "write", Description: "写入工作区文件（input: path, content）。"},
		},
	}
}

// Init 注册工作区动作。
func (p *WorkspacePlugin) Init(ctx context.Context, host *Host) error {
	if err := host.RegisterAction("list", p.list); err != nil {
		return err
	}
	if err := host.RegisterAction("read", p.read); err != nil {
		return err
	}
	return host.RegisterAction("write", p.write)
}

// Start 无副作用。
func (p *WorkspacePlugin) Start(ctx context.Context) error { return nil }

// Stop 无副作用。
func (p *WorkspacePlugin) Stop(ctx context.Context) error { return nil }

func (p *WorkspacePlugin) list(ctx context.Context, input map[string]any) (map[string]any, error) {
	if p.ws == nil {
		return nil, errWorkspaceUnavailable
	}
	dir := p.ws.Root()
	if path := stringField(input, "path"); strings.TrimSpace(path) != "" {
		resolved, err := p.ws.Resolve(path)
		if err != nil {
			return nil, err
		}
		dir = resolved
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, map[string]any{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["name"].(string) < items[j]["name"].(string)
	})
	return map[string]any{"path": relFromRoot(p.ws.Root(), dir), "items": items}, nil
}

func (p *WorkspacePlugin) read(ctx context.Context, input map[string]any) (map[string]any, error) {
	if p.ws == nil {
		return nil, errWorkspaceUnavailable
	}
	path := strings.TrimSpace(stringField(input, "path"))
	if path == "" {
		return nil, errPathRequired
	}
	resolved, err := p.ws.Resolve(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "content": string(data)}, nil
}

func (p *WorkspacePlugin) write(ctx context.Context, input map[string]any) (map[string]any, error) {
	if p.ws == nil {
		return nil, errWorkspaceUnavailable
	}
	path := strings.TrimSpace(stringField(input, "path"))
	if path == "" {
		return nil, errPathRequired
	}
	content := stringField(input, "content")
	resolved, err := p.ws.Resolve(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "bytes": len(content), "written": true}, nil
}

var (
	errWorkspaceUnavailable = &pluginError{msg: "workspace is not available"}
	errPathRequired         = &pluginError{msg: "path is required"}
)

type pluginError struct{ msg string }

func (e *pluginError) Error() string { return e.msg }

func stringField(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key].(string)
	if !ok {
		return ""
	}
	return value
}

func relFromRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}
