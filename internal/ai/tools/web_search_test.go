package tools

import (
	"context"
	"strings"
	"testing"
)

type fakeSearchProvider struct {
	results []SearchResult
	err     error
}

func (p *fakeSearchProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	return p.results, p.err
}

func TestWebSearchToolWithProvider(t *testing.T) {
	tool := NewWebSearchToolWithProvider(&fakeSearchProvider{
		results: []SearchResult{{Title: "标题", URL: "https://example.com/x", Snippet: "摘要内容"}},
	})
	out, err := tool.InvokableRun(context.Background(), `{"query":"hello"}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "摘要内容") || !strings.Contains(out, "https://example.com/x") {
		t.Fatalf("unexpected output %s", out)
	}

	if _, err := tool.InvokableRun(context.Background(), `{"query":""}`); err == nil {
		t.Fatalf("expected error for empty query")
	}
}

func TestParseDDGResponse(t *testing.T) {
	body := &ddgResponse{
		AbstractText: "The answer",
		AbstractURL:  "https://example.com/a",
		Heading:      "Answer heading",
		RelatedTopics: []ddgTopic{
			{Text: "topic one", FirstURL: "https://example.com/1", Topics: []ddgTopic{
				{Text: "nested topic", FirstURL: "https://example.com/n"},
			}},
			{Text: "topic two", FirstURL: "https://example.com/2"},
		},
	}
	results := parseDDGResponse(body)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d: %v", len(results), results)
	}
	if results[0].Title != "Answer heading" || results[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected first result %+v", results[0])
	}

	if len(parseDDGResponse(nil)) != 0 {
		t.Fatalf("expected empty for nil body")
	}
}
