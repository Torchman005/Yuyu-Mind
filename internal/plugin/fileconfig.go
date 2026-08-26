package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// fileFormat 标识元数据 / 配置文件格式。
type fileFormat string

const (
	formatJSON fileFormat = "json"
	formatYAML fileFormat = "yaml"
	formatTOML fileFormat = "toml"
)

var (
	manifestCandidates = []string{"plugin.json", "plugin.yaml", "plugin.yml", "plugin.toml"}
	configCandidates   = []string{"config.json", "config.yaml", "config.yml", "config.toml"}
)

// formatForExt 根据文件扩展名推断格式（未知一律按 JSON 处理）。
func formatForExt(path string) fileFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return formatYAML
	case ".toml":
		return formatTOML
	default:
		return formatJSON
	}
}

// findPluginFile 依次尝试候选文件名，返回第一个存在的普通文件；全部不存在时报错。
func findPluginFile(dir string, candidates []string) (string, error) {
	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no %s file found in %s (expected one of %s)",
		strings.TrimSuffix(candidates[0], filepath.Ext(candidates[0])), dir, strings.Join(candidates, ", "))
}

// ---- manifest ----

// parseManifest 按格式解析元数据。
func parseManifest(data []byte, format fileFormat) (Manifest, error) {
	var m Manifest
	var err error
	switch format {
	case formatYAML:
		err = yaml.Unmarshal(data, &m)
	case formatTOML:
		err = toml.Unmarshal(data, &m)
	default:
		err = json.Unmarshal(data, &m)
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("parse manifest as %s: %w", format, err)
	}
	return m, nil
}

// readManifest 读取插件目录内的元数据文件（plugin.json/.yaml/.yml/.toml）。
func readManifest(dir string) (Manifest, error) {
	path, err := findPluginFile(dir, manifestCandidates)
	if err != nil {
		return Manifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	m, err := parseManifest(data, formatForExt(path))
	if err != nil {
		return Manifest{}, err
	}
	if strings.TrimSpace(m.Name) == "" {
		return Manifest{}, fmt.Errorf("manifest %s has empty name", path)
	}
	return m, nil
}

// ---- config 文件 ----

// readConfigFile 读取插件目录内的配置文件；不存在时返回空配置。
func readConfigFile(dir string) (map[string]any, error) {
	path, err := findPluginFile(dir, configCandidates)
	if err != nil {
		// 无配置文件：返回空 map，由插件按自身默认值运行。
		return map[string]any{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg map[string]any
	switch formatForExt(path) {
	case formatYAML:
		err = yaml.Unmarshal(data, &cfg)
	case formatTOML:
		err = toml.Unmarshal(data, &cfg)
	default:
		err = json.Unmarshal(data, &cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// writeConfigFile 写出插件配置文件到插件目录。沿用已有配置文件格式；
// 不存在时默认写 config.json。TOML 会先剔除 nil 值（TOML 不支持 null）。
func writeConfigFile(dir string, cfg map[string]any) error {
	path, err := findPluginFile(dir, configCandidates)
	if err != nil {
		path = filepath.Join(dir, "config.json")
	}
	format := formatForExt(path)
	var data []byte
	switch format {
	case formatYAML:
		data, err = yaml.Marshal(cfg)
	case formatTOML:
		data, err = toml.Marshal(withoutNils(cfg))
	default:
		data, err = json.MarshalIndent(cfg, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("encode config as %s: %w", format, err)
	}
	return os.WriteFile(path, data, 0o644)
}

// withoutNils 递归删除 map 中值为 nil 的键、slice 中的 nil，用于规避 TOML 不支持 null。
func withoutNils(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if val == nil {
				continue
			}
			out[k] = withoutNils(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, val := range v {
			if val == nil {
				continue
			}
			out = append(out, withoutNils(val))
		}
		return out
	default:
		return value
	}
}

// FileConfigStore 把插件配置持久化到插件目录内的 config.<fmt> 文件。
// dirs 映射 pluginID → 插件目录；未知插件（如内置插件）不做持久化。
type FileConfigStore struct {
	mu   sync.RWMutex
	dirs map[string]string
}

// NewFileConfigStore 创建文件配置存储。
func NewFileConfigStore(dirs map[string]string) *FileConfigStore {
	if dirs == nil {
		dirs = map[string]string{}
	}
	return &FileConfigStore{dirs: dirs}
}

// SetDir 设置（或更新）插件目录映射（用于热加载/刷新）。
func (s *FileConfigStore) SetDir(pluginID, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs[pluginID] = dir
}

// RemoveDir 删除插件目录映射（用于插件卸载）。
func (s *FileConfigStore) RemoveDir(pluginID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dirs, pluginID)
}

// LookupDir 返回插件目录；不存在时第二个返回值为 false。
func (s *FileConfigStore) LookupDir(pluginID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.dirs[pluginID]
	return d, ok
}

// Get 返回插件配置；未知插件或无配置文件时返回空 map。
func (s *FileConfigStore) Get(ctx context.Context, pluginID string) (map[string]any, error) {
	s.mu.RLock()
	dir, ok := s.dirs[pluginID]
	s.mu.RUnlock()
	if !ok {
		return map[string]any{}, nil
	}
	cfg, err := readConfigFile(dir)
	if err != nil {
		return nil, fmt.Errorf("read config for plugin %q: %w", pluginID, err)
	}
	return cfg, nil
}

// Set 写出插件配置到其目录内的配置文件。
func (s *FileConfigStore) Set(ctx context.Context, pluginID string, config map[string]any) error {
	s.mu.RLock()
	dir, ok := s.dirs[pluginID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("plugin %q is not a directory plugin", pluginID)
	}
	if err := writeConfigFile(dir, config); err != nil {
		return fmt.Errorf("write config for plugin %q: %w", pluginID, err)
	}
	return nil
}

// CompositeConfigStore 优先走文件存储，未知插件回退到 settings 键值表。
// 便于目录插件与内置插件混用同一 manager。
type CompositeConfigStore struct {
	file     *FileConfigStore
	fallback ConfigStore
}

// NewCompositeConfigStore 创建复合配置存储。
func NewCompositeConfigStore(file *FileConfigStore, fallback ConfigStore) *CompositeConfigStore {
	return &CompositeConfigStore{file: file, fallback: fallback}
}

// Get 按「先文件、后回退」的顺序取配置。
func (s *CompositeConfigStore) Get(ctx context.Context, pluginID string) (map[string]any, error) {
	if s.file != nil {
		if _, ok := s.fileDirs()[pluginID]; ok {
			return s.file.Get(ctx, pluginID)
		}
	}
	if s.fallback != nil {
		return s.fallback.Get(ctx, pluginID)
	}
	return map[string]any{}, nil
}

// Set 按「先文件、后回退」的顺序写配置。
func (s *CompositeConfigStore) Set(ctx context.Context, pluginID string, config map[string]any) error {
	if s.file != nil {
		if _, ok := s.fileDirs()[pluginID]; ok {
			return s.file.Set(ctx, pluginID, config)
		}
	}
	if s.fallback != nil {
		return s.fallback.Set(ctx, pluginID, config)
	}
	return nil
}

func (s *CompositeConfigStore) fileDirs() map[string]string {
	s.file.mu.RLock()
	defer s.file.mu.RUnlock()
	cp := make(map[string]string, len(s.file.dirs))
	for k, v := range s.file.dirs {
		cp[k] = v
	}
	return cp
}
