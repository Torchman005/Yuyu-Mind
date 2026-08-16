package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Config struct {
	mu       sync.RWMutex
	filePath string

	ActiveProvider ActiveProvider      `json:"active_provider"`
	Providers      map[string]Provider `json:"providers"`
	App            AppConfig           `json:"app"`
	Chat           ChatConfig          `json:"chat"`
	Memory         MemoryConfig        `json:"memory"`
	Speech         SpeechConfig        `json:"speech"`
}

// ActiveProvider identifies the selected model provider and model.
type ActiveProvider struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// Provider stores model provider credentials and endpoint settings.
type Provider struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	Disabled bool   `json:"disabled"`
}

// AppConfig stores general application settings.
type AppConfig struct {
	Theme         string `json:"theme"`
	Language      string `json:"language"`
	MaxTurns      int    `json:"max_turns"`
	DBPath        string `json:"db_path"`
	WorkspaceRoot string `json:"workspace_root"`
}

// ChatConfig stores backend chat orchestration settings.
type ChatConfig struct {
	BotName                       string  `json:"bot_name"`
	Persona                       string  `json:"persona"`
	StyleNotes                    string  `json:"style_notes"`
	ReplyThreshold                float64 `json:"reply_threshold"`
	ReplyFrequency                float64 `json:"reply_frequency"`
	AverageMessageIntervalSeconds int     `json:"average_message_interval_seconds"`
	MinReplyIntervalSeconds       int     `json:"min_reply_interval_seconds"`
	MaxReplyChars                 int     `json:"max_reply_chars"`
	SplitMaxChars                 int     `json:"split_max_chars"`
	AllowTypoSimulation           bool    `json:"allow_typo_simulation"`
}

// MemoryConfig stores conversation memory settings.
type MemoryConfig struct {
	MaxTurns     int `json:"max_turns"`
	MaxTokensEst int `json:"max_tokens_est"`
}

// SpeechConfig stores speech service settings.
type SpeechConfig struct {
	FishAudio FishAudioConfig `json:"fish_audio"`
}

// FishAudioConfig stores Fish Audio TTS settings.
type FishAudioConfig struct {
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key"`
	ReferenceID string `json:"reference_id"`
	Format      string `json:"format"`
}

// DefaultConfig returns the default application configuration.
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
		Chat: ChatConfig{
			BotName:                       "Yuyu",
			Persona:                       "A warm, attentive private voice companion. Be concise, specific, and natural in one-on-one conversation.",
			StyleNotes:                    "Use short spoken sentences. Do not expose internal reasoning, tool names, or memory records.",
			ReplyThreshold:                0.45,
			ReplyFrequency:                1.0,
			AverageMessageIntervalSeconds: 8,
			MinReplyIntervalSeconds:       0,
			MaxReplyChars:                 500,
			SplitMaxChars:                 90,
			AllowTypoSimulation:           false,
		},
		Memory: MemoryConfig{
			MaxTurns:     20,
			MaxTokensEst: 0,
		},
		Speech: SpeechConfig{
			FishAudio: FishAudioConfig{
				BaseURL: "https://api.fish.audio",
				Format:  "mp3",
			},
		},
	}
}

// configDir returns the Yuyu Mind configuration directory.
// 可用环境变量 YUYU_CONFIG_DIR 覆盖（便于测试与便携配置）。
func configDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("YUYU_CONFIG_DIR")); dir != "" {
		return dir, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}
	return filepath.Join(dir, "Yuyu-Mind"), nil
}

// defaultDBPath returns the default SQLite database path.
func defaultDBPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "yuyu-mind.db"), nil
}

// Load reads config from disk and creates a default config when missing.
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
			if cfg.App.DBPath == "" {
				dbPath, _ := defaultDBPath()
				cfg.App.DBPath = dbPath
			}
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
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.App.DBPath == "" {
		dbPath, _ := defaultDBPath()
		cfg.App.DBPath = dbPath
	}
	return cfg, nil
}

// Save writes the current config to disk.
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

// GetActiveProviderConfig returns the currently active provider settings.
func (c *Config) GetActiveProviderConfig() (Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.Providers[c.ActiveProvider.ProviderID]
	if !ok {
		return Provider{}, fmt.Errorf("provider %q not found", c.ActiveProvider.ProviderID)
	}
	if c.ActiveProvider.Model != "" {
		p.Model = c.ActiveProvider.Model
	}
	return p, nil
}

// SetActiveProvider updates the currently active provider and model.
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

// UpdateProvider updates a provider configuration.
func (c *Config) UpdateProvider(id string, p Provider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Providers[id] = p
	return nil
}
