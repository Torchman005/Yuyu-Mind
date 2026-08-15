package app

import (
	"errors"

	"github.com/yuyu-mind/backend/internal/plugin"
)

var errPluginsUnavailable = errors.New("plugin system is not initialized")

// ListPlugins 返回所有已注册插件的状态。
func (a *App) ListPlugins() (PluginListReply, error) {
	if a.pluginMgr == nil {
		return PluginListReply{OK: true, Plugins: []PluginInfo{}}, nil
	}
	statuses := a.pluginMgr.List()
	plugins := make([]PluginInfo, 0, len(statuses))
	for _, status := range statuses {
		plugins = append(plugins, toPluginInfo(status))
	}
	return PluginListReply{OK: true, Plugins: plugins}, nil
}

// EnablePlugin 启用插件。
func (a *App) EnablePlugin(id string) error {
	if a.pluginMgr == nil {
		return errPluginsUnavailable
	}
	return a.pluginMgr.Enable(a.ctx, id)
}

// DisablePlugin 停用插件。
func (a *App) DisablePlugin(id string) error {
	if a.pluginMgr == nil {
		return errPluginsUnavailable
	}
	return a.pluginMgr.Disable(a.ctx, id)
}

// InvokePluginAction 调用指定插件的动作。
func (a *App) InvokePluginAction(id, action string, input map[string]any) (map[string]any, error) {
	if a.pluginMgr == nil {
		return nil, errPluginsUnavailable
	}
	return a.pluginMgr.InvokeAction(a.ctx, id, action, input)
}

// toPluginInfo 把插件运行时状态转换为前端 DTO。
func toPluginInfo(status plugin.PluginStatus) PluginInfo {
	manifest := status.Manifest
	actions := make([]map[string]any, 0, len(manifest.Actions))
	for _, action := range manifest.Actions {
		item := map[string]any{
			"name":        action.Name,
			"description": action.Description,
		}
		if action.InputSchema != nil {
			item["inputSchema"] = action.InputSchema
		}
		actions = append(actions, item)
	}
	return PluginInfo{
		SchemaVersion: manifest.SchemaVersion,
		Name:          manifest.Name,
		DisplayName:   manifest.DisplayName,
		Description:   manifest.Description,
		Version:       manifest.Version,
		Author:        manifest.Author,
		Enabled:       status.Enabled,
		Entry:         manifest.Entry,
		Permissions:   manifest.Permissions,
		ConfigSchema:  manifest.ConfigSchema,
		Actions:       actions,
		LoadedActions: status.Actions,
	}
}
