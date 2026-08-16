package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

type registered struct {
	mu       sync.Mutex
	plugin   Plugin
	manifest Manifest
	enabled  bool
	started  bool
	actions  map[string]ActionHandler
}

// PluginStatus 是 List 返回的运行时状态。
type PluginStatus struct {
	Manifest Manifest
	Enabled  bool
	Actions  []string // 已注册（可调用）的动作名
}

// Manager 管理插件注册、生命周期与动作派发。
type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]*registered
	toolRegistrar func(name string, t tool.BaseTool) error
	configStore   ConfigStore
	logger        *slog.Logger
}

// NewManager 创建插件管理器。toolRegistrar 可为 nil（此时插件不能注册工具）。
func NewManager(toolRegistrar func(name string, t tool.BaseTool) error, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		plugins:       make(map[string]*registered),
		toolRegistrar: toolRegistrar,
		logger:        logger,
	}
}

// SetConfigStore 注入插件配置存储（可选；未注入时配置不持久化）。
func (m *Manager) SetConfigStore(store ConfigStore) {
	m.configStore = store
}

// GetConfig 返回插件配置；无存储或未配置时返回空 map。
func (m *Manager) GetConfig(ctx context.Context, pluginID string) (map[string]any, error) {
	if m.configStore == nil {
		return map[string]any{}, nil
	}
	return m.configStore.Get(ctx, pluginID)
}

// SetConfig 保存插件配置。
func (m *Manager) SetConfig(ctx context.Context, pluginID string, config map[string]any) error {
	if m.configStore == nil {
		return fmt.Errorf("plugin config store is not configured")
	}
	return m.configStore.Set(ctx, pluginID, config)
}

// Register 初始化并启用一个插件。默认即启用；后续可用 Disable 停用。
func (m *Manager) Register(ctx context.Context, p Plugin) error {
	manifest := p.Manifest()
	if manifest.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = "1.0"
	}

	reg := &registered{
		plugin:   p,
		manifest: manifest,
		enabled:  true,
		actions:  make(map[string]ActionHandler),
	}

	host := &Host{
		RegisterTool: func(name string, t tool.BaseTool) error {
			if m.toolRegistrar == nil {
				return fmt.Errorf("tool registrar is not available")
			}
			return m.toolRegistrar(name, t)
		},
		RegisterAction: func(name string, h ActionHandler) error {
			if name == "" {
				return fmt.Errorf("action name is required")
			}
			reg.mu.Lock()
			reg.actions[name] = h
			reg.mu.Unlock()
			return nil
		},
		Config: func() map[string]any {
			cfg, err := m.GetConfig(ctx, manifest.Name)
			if err != nil {
				cfg = map[string]any{}
			}
			return cfg
		},
		Logf: func(format string, args ...any) {
			m.logger.Info(fmt.Sprintf(format, args...), "plugin", manifest.Name)
		},
	}

	if err := p.Init(ctx, host); err != nil {
		return fmt.Errorf("init plugin %q: %w", manifest.Name, err)
	}
	// Init 可能协商了更完整的 manifest（如 sidecar），重新读取一次。
	if negotiated := p.Manifest(); negotiated.Name != "" {
		if negotiated.SchemaVersion == "" {
			negotiated.SchemaVersion = manifest.SchemaVersion
		}
		reg.manifest = negotiated
		manifest = negotiated
	}
	if err := p.Start(ctx); err != nil {
		return fmt.Errorf("start plugin %q: %w", manifest.Name, err)
	}
	reg.mu.Lock()
	reg.started = true
	reg.mu.Unlock()

	m.mu.Lock()
	m.plugins[manifest.Name] = reg
	m.mu.Unlock()
	m.logger.Info("plugin registered", "plugin", manifest.Name, "version", manifest.Version)
	return nil
}

// Enable 启用一个已注册的插件。
func (m *Manager) Enable(ctx context.Context, id string) error {
	reg, ok := m.lookup(id)
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.enabled {
		return nil
	}
	if err := reg.plugin.Start(ctx); err != nil {
		return fmt.Errorf("start plugin %q: %w", id, err)
	}
	reg.enabled = true
	reg.started = true
	m.logger.Info("plugin enabled", "plugin", id)
	return nil
}

// Disable 停用一个插件，但不卸载其动作定义。
func (m *Manager) Disable(ctx context.Context, id string) error {
	reg, ok := m.lookup(id)
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if !reg.enabled {
		return nil
	}
	if reg.started {
		if err := reg.plugin.Stop(ctx); err != nil {
			return fmt.Errorf("stop plugin %q: %w", id, err)
		}
		reg.started = false
	}
	reg.enabled = false
	m.logger.Info("plugin disabled", "plugin", id)
	return nil
}

// List 返回所有插件的运行时状态，按名称排序。
func (m *Manager) List() []PluginStatus {
	m.mu.RLock()
	regs := make([]*registered, 0, len(m.plugins))
	for _, reg := range m.plugins {
		regs = append(regs, reg)
	}
	m.mu.RUnlock()

	statuses := make([]PluginStatus, 0, len(regs))
	for _, reg := range regs {
		reg.mu.Lock()
		status := PluginStatus{
			Manifest: reg.manifest,
			Enabled:  reg.enabled,
			Actions:  make([]string, 0, len(reg.actions)),
		}
		for name := range reg.actions {
			status.Actions = append(status.Actions, name)
		}
		reg.mu.Unlock()
		sort.Strings(status.Actions)
		statuses = append(statuses, status)
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Manifest.Name < statuses[j].Manifest.Name
	})
	return statuses
}

// InvokeAction 调用指定插件的动作。仅启用中的插件可被调用。
func (m *Manager) InvokeAction(ctx context.Context, id, action string, input map[string]any) (map[string]any, error) {
	reg, ok := m.lookup(id)
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", id)
	}

	reg.mu.Lock()
	if !reg.enabled {
		reg.mu.Unlock()
		return nil, fmt.Errorf("plugin %q is disabled", id)
	}
	handler, ok := reg.actions[action]
	reg.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("action %q not found in plugin %q", action, id)
	}
	if input == nil {
		input = map[string]any{}
	}
	return handler(ctx, input)
}

// StopAll 停用所有插件（宿主退出时调用）。
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.RLock()
	regs := make([]*registered, 0, len(m.plugins))
	for _, reg := range m.plugins {
		regs = append(regs, reg)
	}
	m.mu.RUnlock()

	for _, reg := range regs {
		reg.mu.Lock()
		if reg.started {
			_ = reg.plugin.Stop(ctx)
			reg.started = false
			reg.enabled = false
		}
		reg.mu.Unlock()
	}
}

func (m *Manager) lookup(id string) (*registered, bool) {
	m.mu.RLock()
	reg, ok := m.plugins[id]
	m.mu.RUnlock()
	return reg, ok
}
