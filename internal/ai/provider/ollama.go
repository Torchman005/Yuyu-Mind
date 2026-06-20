package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/yuyu-mind/backend/pkg/types"
)

// NewOllamaFactory 创建 Ollama 本地模型工厂。
// Ollama 在 /v1 暴露 OpenAI 兼容接口，因此复用 OpenAI 适配器。
func NewOllamaFactory() Factory {
	return func(ctx context.Context, cfg types.ProviderConfig) (model.ChatModel, error) {
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		if cfg.Model == "" {
			return nil, fmt.Errorf("model is required for Ollama")
		}

		chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: baseURL,
			APIKey:  "ollama", // Ollama 不需要真实密钥，但适配器要求该字段非空。
			Model:   cfg.Model,
		})
		if err != nil {
			return nil, fmt.Errorf("create ollama model: %w", err)
		}
		return chatModel, nil
	}
}
