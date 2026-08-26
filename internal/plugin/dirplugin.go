package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/ai/tools"
)

// DirPlugin 是一个「目录即插件」的实现：一个目录内存放元数据文件（plugin.json/.yaml/.yml/.toml）、
// 配置文件（config.<fmt>）和入口脚本/可执行文件（entry）。挂载时宿主仅读取文件即可知晓插件能力，
// 真正执行时才按需拉起其入口进程（sidecar，stdio JSON-RPC）。
type DirPlugin struct {
	mu       sync.Mutex
	dir      string
	manifest Manifest
	client   *sidecarClient
	baseDir  string // 可选：宿主工作区根目录，注入为 YUYU_WORKSPACE 供侧车默认落位
	logf     func(format string, args ...any)
}

// NewDirPlugin 从目录读取元数据并创建目录插件。目录必须含 plugin.json/.yaml/.yml/.toml 之一。
func NewDirPlugin(dir string) (*DirPlugin, error) {
	m, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	return &DirPlugin{dir: dir, manifest: m}, nil
}

// SetBaseDir 设置宿主工作区根目录，注入给 sidecar 作为默认工作目录（YUYU_WORKSPACE）。
func (p *DirPlugin) SetBaseDir(dir string) { p.baseDir = dir }

// DiscoverPluginDirs 扫描 root 下每个子目录，把含元数据文件的目录解析为目录插件。
// 不含元数据文件（或解析失败）的目录被跳过，错误收集到返回值第二项供调用方记录。
func DiscoverPluginDirs(root string) ([]*DirPlugin, []error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []error{err}
	}
	var plugins []*DirPlugin
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		p, err := NewDirPlugin(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("plugin dir %s: %w", dir, err))
			continue
		}
		plugins = append(plugins, p)
	}
	return plugins, errs
}

// Manifest 返回从元数据文件解析出的插件契约。
func (p *DirPlugin) Manifest() Manifest { return p.manifest }

// Dir 返回插件目录绝对路径。
func (p *DirPlugin) Dir() string { return p.dir }

// Init 读取元数据并把 tools/actions 注册成「转发到 sidecar 的桩」。此阶段不拉起进程。
func (p *DirPlugin) Init(ctx context.Context, host *Host) error {
	p.logf = host.Logf
	for _, def := range p.manifest.Tools {
		def := def
		if err := host.RegisterTool(def.Name, &dirTool{plugin: p, def: def}); err != nil {
			return err
		}
	}
	for _, action := range p.manifest.Actions {
		name := action.Name
		if err := host.RegisterAction(name, func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if p.logf != nil {
				p.logf("plugin action invoked: %s", name)
			}
			result, err := p.invoke(ctx, "invoke_action", map[string]any{"action": name, "input": input})
			if err != nil {
				if p.logf != nil {
					p.logf("plugin action error: %s: %v", name, err)
				}
				return nil, err
			}
			if p.logf != nil {
				if data, merr := json.Marshal(result); merr == nil {
					p.logf("plugin action result (%s): %s", name, truncateForLog(string(data), 3000))
				}
			}
			return result, nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Start 采用懒加载：注册阶段不拉起进程，首次调用 tools/actions 时才启动。
func (p *DirPlugin) Start(ctx context.Context) error { return nil }

// Stop 关闭 sidecar 进程。
func (p *DirPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		p.client.close()
		p.client = nil
	}
	return nil
}

// invoke 自动确保 sidecar 进程在跑，然后发起一次 JSON-RPC 调用。
func (p *DirPlugin) invoke(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	client, err := p.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.call(method, params)
}

// ensureClient 按需启动入口进程并缓存客户端。
func (p *DirPlugin) ensureClient(ctx context.Context) (*sidecarClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return p.client, nil
	}

	entry := strings.TrimSpace(p.manifest.Entry)
	if entry == "" {
		return nil, fmt.Errorf("plugin %q declares no entry; cannot run", p.manifest.Name)
	}
	entryPath := entry
	if !filepath.IsAbs(entryPath) {
		entryPath = filepath.Join(p.dir, entry)
	}
	command, args := p.commandFor(entryPath)

	// 把当前配置以 JSON 注入环境，sidecar 无需自带 YAML/TOML 解析器即可读取。
	cfg, _ := readConfigFile(p.dir)
	cfgJSON, _ := json.Marshal(cfg)

	env := append(os.Environ(),
		"YUYU_PLUGIN_DIR="+p.dir,
		"YUYU_PLUGIN_CONFIG="+string(cfgJSON),
	)
	if strings.TrimSpace(p.baseDir) != "" {
		env = append(env, "YUYU_WORKSPACE="+p.baseDir)
	}

	client, err := newSidecarClient(SidecarSpec{
		Name:    p.manifest.Name,
		Command: command,
		Args:    args,
		Env:     env,
	})
	if err != nil {
		return nil, fmt.Errorf("start plugin %q entry %q: %w", p.manifest.Name, entryPath, err)
	}
	p.client = client
	return client, nil
}

// commandFor 根据 runtime 字段或入口扩展名确定可执行命令与参数。
func (p *DirPlugin) commandFor(entryPath string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(p.manifest.Runtime)) {
	case "node":
		return "node", []string{entryPath}
	case "python":
		return "python", []string{entryPath}
	}
	switch strings.ToLower(filepath.Ext(entryPath)) {
	case ".js":
		return "node", []string{entryPath}
	case ".py":
		return "python", []string{entryPath}
	default:
		// 直接可执行文件（.exe/.cmd/.bat/.ps1 或本平台可执行文件）。
		return entryPath, nil
	}
}

// dirTool 是「转发到 sidecar 的 Eino 工具桩」。
type dirTool struct {
	plugin *DirPlugin
	def    Tool
}

// Info 返回工具元数据（来自 manifest，并带上参数 JSON Schema，供 Planner/Worker 正确构造参数）。
func (t *dirTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info := &schema.ToolInfo{Name: t.def.Name, Desc: t.def.Description}
	if raw := t.def.InputSchema; raw != nil {
		if p, err := tools.BuildToolParams(raw); err == nil && p != nil {
			info.ParamsOneOf = p
		}
	}
	return info, nil
}

// InvokableRun 把参数转发给 sidecar 的 invoke_tool，并返回其 result 字符串。
func (t *dirTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	if t.plugin.logf != nil {
		t.plugin.logf("plugin tool invoked: %s", t.def.Name)
	}
	res, err := t.plugin.invoke(ctx, "invoke_tool", map[string]any{
		"tool":      t.def.Name,
		"arguments": argumentsJSON,
	})
	if err != nil {
		if t.plugin.logf != nil {
			t.plugin.logf("plugin tool error: %s: %v", t.def.Name, err)
		}
		return "", err
	}
	out := ""
	if s, ok := res["result"].(string); ok {
		out = s
	} else {
		if data, merr := json.Marshal(res); merr == nil {
			out = string(data)
		}
	}
	if t.plugin.logf != nil {
		t.plugin.logf("plugin tool result (%s): %s", t.def.Name, truncateForLog(out, 3000))
	}
	return out, nil
}

// truncateForLog 截断日志内容（按 rune 计数），避免插件的超长输出刷屏。
func truncateForLog(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "\n...(截断)"
}
