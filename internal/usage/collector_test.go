package usage

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCollectorEmpty(t *testing.T) {
	c := NewCollector()
	if s := c.Snapshot(); s.ModelCalls != 0 || s.TotalTokens != 0 {
		t.Fatalf("empty snapshot = %+v", s)
	}
}

func TestCollectorAddMessage(t *testing.T) {
	c := NewCollector()
	c.AddMessage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		},
	})
	c.AddMessage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 20, CompletionTokens: 30, TotalTokens: 50},
		},
	})

	s := c.Snapshot()
	if s.ModelCalls != 2 || s.PromptTokens != 120 || s.CompletionTokens != 80 || s.TotalTokens != 200 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestCollectorTotalTokensFallback(t *testing.T) {
	c := NewCollector()
	c.AddMessage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 40, CompletionTokens: 60},
		},
	})
	if s := c.Snapshot(); s.TotalTokens != 100 {
		t.Fatalf("fallback total = %d", s.TotalTokens)
	}
}

func TestCollectorNilSafe(t *testing.T) {
	c := NewCollector()
	c.AddMessage(nil)
	c.AddMessage(&schema.Message{})
	c.AddMessage(&schema.Message{ResponseMeta: &schema.ResponseMeta{}})
	if s := c.Snapshot(); s.ModelCalls != 0 || s.TotalTokens != 0 {
		t.Fatalf("nil-safe snapshot = %+v", s)
	}
}
