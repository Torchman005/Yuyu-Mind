package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/yuyu-mind/backend/pkg/types"
)

// Factory 根据 ProviderConfig 创建 ChatModel 实例。
type Factory func(ctx context.Context, cfg types.ProviderConfig) (model.ChatModel, error)

// Registry 管理模型供应商工厂。
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry 创建供应商注册表。
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register 注册供应商工厂。
func (r *Registry) Register(providerID string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[providerID] = factory
}

// Create 使用已注册的工厂创建指定供应商的 ChatModel。
func (r *Registry) Create(ctx context.Context, cfg types.ProviderConfig) (model.ChatModel, error) {
	r.mu.RLock()
	factory, ok := r.factories[cfg.ID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not registered", cfg.ID)
	}
	return factory(ctx, cfg)
}

// ListProviders 返回所有已注册的供应商 ID。
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.factories))
	for id := range r.factories {
		ids = append(ids, id)
	}
	return ids
}
