package tools

import (
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// Registry 管理工具实例。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]tool.BaseTool
}

// NewRegistry 创建工具注册表。
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]tool.BaseTool),
	}
}

// Register 注册工具。
func (r *Registry) Register(name string, t tool.BaseTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = t
}

// Get 按名称获取工具。
func (r *Registry) Get(name string) (tool.BaseTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

// GetAll 返回所有已注册工具。
func (r *Registry) GetAll() []tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]tool.BaseTool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ListNames 返回所有已注册工具名称。
func (r *Registry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
