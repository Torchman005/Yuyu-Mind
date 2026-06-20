package usage

import (
	"sync"

	"github.com/cloudwego/eino/schema"
)

// Snapshot 是一次请求内所有模型调用的 token 聚合结果。
type Snapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ModelCalls       int
}

// Collector 线程安全地收集一次请求内的模型 token 用量。
type Collector struct {
	mu       sync.Mutex
	snapshot Snapshot
}

// NewCollector 创建 token 用量收集器。
func NewCollector() *Collector {
	return &Collector{}
}

// AddMessage 从 Eino 消息的 ResponseMeta 中提取 token 用量并累计。
func (c *Collector) AddMessage(msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}

	usage := msg.ResponseMeta.Usage
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot.PromptTokens += usage.PromptTokens
	c.snapshot.CompletionTokens += usage.CompletionTokens
	c.snapshot.TotalTokens += totalTokens
	c.snapshot.ModelCalls++
}

// Snapshot 返回当前累计结果的副本。
func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot
}
