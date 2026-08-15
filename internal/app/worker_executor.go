package app

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/agent"
	pkgTypes "github.com/yuyu-mind/backend/pkg/types"
)

// workerModelAdapter 把 Eino 的 model.ToolCallingChatModel 适配为 agent.ToolCallingModel。
type workerModelAdapter struct {
	inner model.ToolCallingChatModel
}

func (a *workerModelAdapter) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.inner.Generate(ctx, input, opts...)
}

func (a *workerModelAdapter) WithTools(ts []*schema.ToolInfo) (agent.ToolCallingModel, error) {
	m, err := a.inner.WithTools(ts)
	if err != nil {
		return nil, err
	}
	return &workerModelAdapter{inner: m}, nil
}

// workerModelFactory 为 Worker 执行器创建工具调用模型。
// 每次执行新建模型实例（WithTools 返回新实例，天然并发安全）。
func (a *App) workerModelFactory() func(context.Context) (agent.ToolCallingModel, error) {
	return func(ctx context.Context) (agent.ToolCallingModel, error) {
		providerCfg, err := a.cfg.GetActiveProviderConfig()
		if err != nil {
			return nil, err
		}
		baseModel, err := a.providerReg.Create(ctx, pkgTypes.ProviderConfig{
			ID:      a.cfg.ActiveProvider.ProviderID,
			Name:    providerCfg.Name,
			BaseURL: providerCfg.BaseURL,
			APIKey:  providerCfg.APIKey,
			Model:   providerCfg.Model,
		})
		if err != nil {
			return nil, err
		}
		tcModel, ok := baseModel.(model.ToolCallingChatModel)
		if !ok {
			return nil, fmt.Errorf("active model %q does not support tool calling", providerCfg.Model)
		}
		return &workerModelAdapter{inner: tcModel}, nil
	}
}
