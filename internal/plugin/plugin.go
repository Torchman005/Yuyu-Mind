// Package plugin 定义桌宠的进程内插件系统。
//
// 设计目标：先打通「稳定接口 + 生命周期 + 权限声明 + 动作派发」的正确性，
// 再考虑第三方二进制热插拔（后续通过子进程 sidecar 补齐，见 docs/DEVELOPMENT-NOTES.md 难点 2）。
package plugin

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

// ActionHandler 是插件暴露给前端调用的动作处理器。
// 入参是前端传来的 JSON 对象，返回可 JSON 序列化的结果。
type ActionHandler func(ctx context.Context, input map[string]any) (map[string]any, error)

// Action 描述插件暴露的一个可调用动作（供前端发现与调用）。
type Action struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// Manifest 描述插件元数据、权限与能力，是插件系统的对外契约。
// 与前端 app.PluginInfo（companion.go）字段一一对应。
type Manifest struct {
	SchemaVersion string         `json:"schemaVersion"`
	Name          string         `json:"name"`
	DisplayName   string         `json:"displayName"`
	Description   string         `json:"description"`
	Version       string         `json:"version"`
	Author        string         `json:"author"`
	Entry         string         `json:"entry"`
	Permissions   []string       `json:"permissions"`
	ConfigSchema  map[string]any `json:"configSchema,omitempty"`
	Actions       []Action       `json:"actions,omitempty"`
}

// Host 是宿主在插件 Init 期间提供的服务集合。
// 采用函数字段而非接口，便于宿主按需注入闭包，避免插件包反向依赖 app 包。
type Host struct {
	// RegisterTool 把 Eino 工具注册进宿主的工具注册表（供 Planner 调用）。
	RegisterTool func(name string, t tool.BaseTool) error
	// RegisterAction 注册一个可被前端调用的动作。
	RegisterAction func(name string, h ActionHandler) error
	// Logf 通过宿主统一日志输出。
	Logf func(format string, args ...any)
}

// Plugin 是进程内插件接口。所有内置/同进程插件实现此接口。
type Plugin interface {
	// Manifest 返回插件静态元数据。
	Manifest() Manifest
	// Init 在插件被注册时调用，可在此注册工具与动作。
	Init(ctx context.Context, host *Host) error
	// Start 在插件启用时调用。
	Start(ctx context.Context) error
	// Stop 在插件停用或宿主退出时调用。
	Stop(ctx context.Context) error
}
