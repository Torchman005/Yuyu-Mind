package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/yuyu-mind/backend/pkg/types"
)

// NewOpenAICompatFactory 创建 OpenAI 兼容供应商的工厂。
// OpenAI、DeepSeek、Moonshot 等兼容 OpenAI API 的服务都可以复用。
func NewOpenAICompatFactory() Factory {
	return func(ctx context.Context, cfg types.ProviderConfig) (model.BaseChatModel, error) {
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("base_url is required for provider %q", cfg.ID)
		}
		if cfg.Model == "" {
			return nil, fmt.Errorf("model is required for provider %q", cfg.ID)
		}

		chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		})
		if err != nil {
			return nil, fmt.Errorf("create openai compat model (%s): %w", cfg.ID, err)
		}
		return chatModel, nil
	}
}
