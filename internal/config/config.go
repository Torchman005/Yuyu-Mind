package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config 保存应用的全部配置。
type Config struct {
	mu       sync.RWMutex
	filePath string

	ActiveProvider ActiveProvider      `json:"active_provider"`
	Providers      map[string]Provider `json:"providers"`
	App            AppConfig           `json:"app"`
	Memory         MemoryConfig        `json:"memory"`
}

// ActiveProvider 标识当前选中的模型供应商和模型。
type ActiveProvider struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// Provider 保存模型供应商的凭据和端点信息。
type Provider struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"` // 该供应商的默认模型
	Disabled bool   `json:"disabled"`
}

// AppConfig 保存应用通用配置。
type AppConfig struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
	MaxTurns int    `json:"max_turns"` // 上下文中保留的最大对话轮数
	DBPath   string `json:"db_path"`   // 数据库绝对路径；为空时使用默认路径
}

// MemoryConfig 保存会话记忆配置。
type MemoryConfig struct {
	MaxTurns     int `json:"max_turns"`      // 最大历史轮数，0 表示不限制
	MaxTokensEst int `json:"max_tokens_est"` // 历史消息粗略 token 预算，0 表示不限制
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		ActiveProvider: ActiveProvider{
			ProviderID: "openai",
			Model:      "gpt-4o",
		},
		Providers: map[string]Provider{
			"openai": {
				Name:    "OpenAI",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "",
				Model:   "gpt-4o",
			},
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "",
				Model:   "deepseek-chat",
			},
			"moonshot": {
				Name:    "Moonshot",
				BaseURL: "https://api.moonshot.cn/v1",
				APIKey:  "",
				Model:   "moonshot-v1-8k",
			},
			"ollama": {
				Name:    "Ollama",
				BaseURL: "http://localhost:11434/v1",
				APIKey:  "",
				Model:   "llama3",
			},
		},
		App: AppConfig{
			Theme:    "system",
			Language: "zh-CN",
			MaxTurns: 20,
		},
		Memory: MemoryConfig{
			MaxTurns:     20,
			MaxTokensEst: 0,
		},
	}
}

// configDir 返回当前系统下的 Yuyu Mind 配置目录。
func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}
	return filepath.Join(dir, "Yuyu-Mind"), nil
}

// defaultDBPath 返回默认数据库文件路径。
func defaultDBPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yuyu-mind.db"), nil
}

// Load 从磁盘读取配置；配置不存在时创建默认配置。
func Load() (*Config, error) {
	dir, err := configDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}

	filePath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.filePath = filePath
			// 设置默认数据库路径。
			if cfg.App.DBPath == "" {
				dbPath, _ := defaultDBPath()
				cfg.App.DBPath = dbPath
			}
			// 保存默认配置。
			if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
				return nil, fmt.Errorf("create config dir: %w", mkdirErr)
			}
			if saveErr := cfg.Save(); saveErr != nil {
				return nil, fmt.Errorf("save default config: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	cfg.filePath = filePath
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.App.DBPath == "" {
		dbPath, _ := defaultDBPath()
		cfg.App.DBPath = dbPath
	}
	return cfg, nil
}

// Save 将当前配置写入磁盘。
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.filePath == "" {
		dir, err := configDir()
		if err != nil {
			return fmt.Errorf("config dir: %w", err)
		}
		c.filePath = filepath.Join(dir, "config.json")
	}

	if err := os.MkdirAll(filepath.Dir(c.filePath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(c.filePath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// GetActiveProviderConfig 返回当前激活供应商的配置。
func (c *Config) GetActiveProviderConfig() (Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.Providers[c.ActiveProvider.ProviderID]
	if !ok {
		return Provider{}, fmt.Errorf("provider %q not found", c.ActiveProvider.ProviderID)
	}
	// 如果 ActiveProvider 指定了模型，则覆盖供应商默认模型。
	if c.ActiveProvider.Model != "" {
		p.Model = c.ActiveProvider.Model
	}
	return p, nil
}

// SetActiveProvider 更新当前激活的供应商和模型。
func (c *Config) SetActiveProvider(providerID, model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.Providers[providerID]; !ok {
		return fmt.Errorf("provider %q not found", providerID)
	}
	c.ActiveProvider = ActiveProvider{
		ProviderID: providerID,
		Model:      model,
	}
	return nil
}

// UpdateProvider 更新供应商配置。
func (c *Config) UpdateProvider(id string, p Provider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Providers[id] = p
	return nil
}
