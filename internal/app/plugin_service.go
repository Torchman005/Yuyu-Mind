package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yuyu-mind/backend/internal/db"
	"github.com/yuyu-mind/backend/internal/plugin"
)

var errPluginsUnavailable = errors.New("plugin system is not initialized")

// settingsConfigStore 用 settings 键值表持久化插件配置（key = plugin.config.<id>）。
type settingsConfigStore struct {
	settings *db.SettingsRepo
}

func (s *settingsConfigStore) Get(ctx context.Context, pluginID string) (map[string]any, error) {
	raw, err := s.settings.Get(ctx, "plugin.config."+pluginID)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("decode plugin config: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

func (s *settingsConfigStore) Set(ctx context.Context, pluginID string, config map[string]any) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode plugin config: %w", err)
	}
	return s.settings.Set(ctx, "plugin.config."+pluginID, string(raw))
}

// GetPluginConfig 返回插件配置（无配置返回空对象）。
func (a *App) GetPluginConfig(pluginID string) (map[string]any, error) {
	if a.pluginMgr == nil {
		return nil, errPluginsUnavailable
	}
	return a.pluginMgr.GetConfig(a.ctx, pluginID)
}

// SetPluginConfig 保存插件配置。
func (a *App) SetPluginConfig(pluginID string, config map[string]any) error {
	if a.pluginMgr == nil {
		return errPluginsUnavailable
	}
	return a.pluginMgr.SetConfig(a.ctx, pluginID, config)
}

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
	tools := make([]map[string]any, 0, len(manifest.Tools))
	loadedTools := make([]string, 0, len(manifest.Tools))
	for _, t := range manifest.Tools {
		item := map[string]any{
			"name":        t.Name,
			"description": t.Description,
		}
		if t.InputSchema != nil {
			item["inputSchema"] = t.InputSchema
		}
		tools = append(tools, item)
		loadedTools = append(loadedTools, t.Name)
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
		Tools:         tools,
		LoadedTools:   loadedTools,
	}
}
