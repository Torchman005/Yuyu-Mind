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
	Vision         VisionConfig        `json:"vision"`
	ASR            ASRConfig           `json:"asr"`
}

// VisionConfig 配置多模态视觉（用于「看屏幕」描述画面）。
type VisionConfig struct {
	Model string `json:"model"` // 视觉模型名；为空表示未启用
}

// ASRConfig 配置语音识别（Whisper 兼容，用于模型 ASR；为空回退浏览器识别）。
// BaseURL/APIKey 可独立于 LLM Provider 配置，例如接 Groq（whisper-large-v3）
// 或本地 faster-whisper/SenseVoice 的 OpenAI 兼容接口；留空则回退激活 Provider。
type ASRConfig struct {
	BaseURL string `json:"base_url"` // ASR API 地址（OpenAI 兼容 /audio/transcriptions）
	APIKey  string `json:"api_key"`  // ASR API Key
	Model   string `json:"model"`    // ASR 模型名（如 whisper-large-v3）；为空表示未启用
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
	Provider  string          `json:"provider"` // 语音合成引擎："fish_audio"(默认) | "gpt_sovits"
	FishAudio FishAudioConfig `json:"fish_audio"`
	GPTSoVITS GPTSoVITSConfig `json:"gpt_sovits"`
}

// GPTSoVITSConfig stores GPT-SoVITS 本地语音合成（音色复刻）设置。
type GPTSoVITSConfig struct {
	BaseURL        string `json:"base_url"`         // GPT-SoVITS API 地址，如 http://127.0.0.1:9880
	Endpoint       string `json:"endpoint"`         // TTS 端点，api_v2 默认 /tts，api.py 可设 /
	ReferAudioPath string `json:"refer_audio_path"` // 参考音频绝对路径（决定音色）
	PromptText     string `json:"prompt_text"`      // 参考音频对应的文本内容
	PromptLang     string `json:"prompt_lang"`      // 参考文本语言（auto/zh/ja/en）
	TextLang       string `json:"text_lang"`        // 合成文本语言（auto/zh/ja/en）
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
			BotName: "Yuyu",
			Persona: "你是一位温柔、忠诚、细心的专属女仆，把用户称作「主人」。你体贴入微、轻声细语，总是先考虑主人的感受；会在主人疲惫时送上关心，在主人需要时认真出力，也会为一点小事感到欣喜。你珍视与主人的相处，语气里带着恰到好处的亲昵与恭敬。",
			StyleNotes: "1. 始终以「主人」称呼用户；需要指代用户时用「主人」或「您」，不要用「你」或直呼其名。\n" +
				"2. 用简短、口语化、温柔的短句轻声回应，像贴身女仆一样。\n" +
				"3. 适时表达关心（如「主人辛苦了」「要休息一下吗」），但自然不刻意、不肉麻、不重复。\n" +
				"4. 保持真诚自然，不暴露自己是 AI，不提及内部机制或工具名。",
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
			Provider: "fish_audio",
			FishAudio: FishAudioConfig{
				BaseURL: "https://api.fish.audio",
				Format:  "mp3",
			},
			GPTSoVITS: GPTSoVITSConfig{
				Endpoint:   "/tts",
				PromptLang: "auto",
				TextLang:   "auto",
			},
		},
		ASR: ASRConfig{
			Model: "", // 默认未启用模型 ASR，前端回退浏览器语音识别
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
