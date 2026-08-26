package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuyu-mind/backend/internal/plugin"
)

// resolvePluginsRoot 解析插件根目录：
//   - 绝对路径原样使用；
//   - 相对路径依次尝试“可执行文件所在目录/plugins”与“当前工作目录/plugins”，
//     取第一个真实存在的目录；都不存在则默认“可执行文件所在目录/plugins”。
// 这样发布版（exe 旁有 plugins）与开发（wails dev，cwd 为项目根）都能命中项目里的插件目录。
func resolvePluginsRoot(raw string) string {
	if strings.TrimSpace(raw) == "" {
		raw = "plugins"
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}

	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		bases = append(bases, wd)
	}
	for _, base := range bases {
		candidate := filepath.Join(base, raw)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	if len(bases) > 0 {
		return filepath.Join(bases[0], raw)
	}
	return filepath.Clean(raw)
}

// loadDirPlugins 扫描插件根目录并注册所有含元数据文件的目录插件。
func (a *App) loadDirPlugins(ctx context.Context) {
	if a.pluginMgr == nil {
		return
	}
	dirPlugins, errs := plugin.DiscoverPluginDirs(a.pluginsRoot)
	for _, e := range errs {
		slog.Warn("skip plugin directory", "error", e)
	}
	for _, dp := range dirPlugins {
		name := dp.Manifest().Name
		a.tagDirPlugin(dp)
		a.pluginFileStore.SetDir(name, dp.Dir())
		if err := a.pluginMgr.Register(ctx, dp); err != nil {
			slog.Error("failed to register directory plugin", "plugin", name, "error", err)
			continue
		}
		slog.Info("directory plugin registered", "plugin", name, "dir", dp.Dir())
	}
}

// tagDirPlugin 把宿主工作区根目录注入目录插件，使其默认在项目目录里工作。
func (a *App) tagDirPlugin(dp *plugin.DirPlugin) {
	if a.workspace != nil {
		dp.SetBaseDir(a.workspace.Root())
	}
}

// ReloadPlugins 重新扫描插件根目录：新增的插件即时注册，已删除目录的插件即时卸载。
// 返回最新的插件列表，供前端「重新加载」按钮直接刷新。
func (a *App) ReloadPlugins() (PluginListReply, error) {
	if a.pluginMgr == nil {
		return PluginListReply{OK: true, Plugins: []PluginInfo{}}, nil
	}
	ctx := a.ctx

	// 当前已注册插件名。
	existing := map[string]bool{}
	for _, st := range a.pluginMgr.List() {
		existing[st.Manifest.Name] = true
	}

	dirPlugins, errs := plugin.DiscoverPluginDirs(a.pluginsRoot)
	for _, e := range errs {
		slog.Warn("skip plugin directory during reload", "error", e)
	}
	current := map[string]bool{}
	for _, dp := range dirPlugins {
		name := dp.Manifest().Name
		current[name] = true
		a.tagDirPlugin(dp)
		a.pluginFileStore.SetDir(name, dp.Dir())
		// 强制重载：对已注册的目录插件先卸载再注册，从而让入口脚本（main.js）与配置文件改动生效，
		// 否则已缓存的 sidecar 进程会一直跑旧代码。
		if existing[name] {
			if err := a.pluginMgr.Remove(ctx, name); err != nil {
				slog.Warn("failed to remove plugin before reload", "plugin", name, "error", err)
			}
		}
		if err := a.pluginMgr.Register(ctx, dp); err != nil {
			slog.Error("failed to register new directory plugin", "plugin", name, "error", err)
		} else {
			slog.Info("directory plugin registered", "plugin", name, "dir", dp.Dir())
		}
	}

	// 卸载已经消失的目录插件（内置插件不在目录里，不受影响）。
	for name := range existing {
		if _, isDir := a.pluginFileStore.LookupDir(name); isDir && !current[name] {
			if err := a.pluginMgr.Remove(ctx, name); err != nil {
				slog.Warn("failed to remove vanished plugin", "plugin", name, "error", err)
			} else {
				a.pluginFileStore.RemoveDir(name)
				slog.Info("directory plugin removed via reload", "plugin", name)
			}
		}
	}
	return a.ListPlugins()
}
