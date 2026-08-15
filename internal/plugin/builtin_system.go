package plugin

import (
	"context"
	"time"
)

// SystemPlugin 是内置示例插件，用于证明插件生命周期与动作派发可用。
// 后续「电脑工具」「游戏」「任务」等能力都可以按同样方式挂载为插件。
type SystemPlugin struct{}

// NewSystemPlugin 创建系统信息插件。
func NewSystemPlugin() *SystemPlugin {
	return &SystemPlugin{}
}

// Manifest 返回系统插件元数据。
func (p *SystemPlugin) Manifest() Manifest {
	return Manifest{
		SchemaVersion: "1.0",
		Name:          "system",
		DisplayName:   "系统信息",
		Description:   "内置插件：提供宿主机心跳与版本信息。",
		Version:       "0.1.0",
		Author:        "Yuyu Mind",
		Entry:         "builtin",
		Permissions:   []string{"read:system"},
		Actions: []Action{
			{Name: "ping", Description: "返回宿主心跳与当前时间。"},
			{Name: "version", Description: "返回应用与插件 Schema 版本信息。"},
		},
	}
}

// Init 注册系统插件可调用的动作。
func (p *SystemPlugin) Init(ctx context.Context, host *Host) error {
	if err := host.RegisterAction("ping", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{
			"ok":   true,
			"time": time.Now().Format(time.RFC3339),
		}, nil
	}); err != nil {
		return err
	}

	if err := host.RegisterAction("version", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{
			"app":           "Yuyu Mind",
			"version":       "0.1.0",
			"schemaVersion": "1.0",
		}, nil
	}); err != nil {
		return err
	}
	return nil
}

// Start 系统插件无副作用。
func (p *SystemPlugin) Start(ctx context.Context) error { return nil }

// Stop 系统插件无副作用。
func (p *SystemPlugin) Stop(ctx context.Context) error { return nil }
