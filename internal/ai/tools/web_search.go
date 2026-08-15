package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SearchResult 是单条网页搜索结果。
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProvider 是 web_search 的搜索后端接口。
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// WebSearchInput 是 web_search 的参数。
type WebSearchInput struct {
	Query string `json:"query" jsonschema:"description=The search query"`
}

// WebSearchTool 执行网页搜索。
type WebSearchTool struct {
	provider SearchProvider
}

// NewWebSearchTool 创建使用 DuckDuckGo 后端的网页搜索工具（无需 API Key）。
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{provider: NewDuckDuckGoProvider()}
}

// NewWebSearchToolWithProvider 使用指定后端创建网页搜索工具（便于测试）。
func NewWebSearchToolWithProvider(provider SearchProvider) *WebSearchTool {
	return &WebSearchTool{provider: provider}
}

// Info 返回提供给模型的工具元数据。
func (t *WebSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the web for information. Use this when you need to find current information, facts, or look things up online.",
	}, nil
}

// InvokableRun 执行网页搜索。
func (t *WebSearchTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var input WebSearchInput
	if err := json.Unmarshal([]byte(argumentsJSON), &input); err != nil {
		return "", fmt.Errorf("parse web_search arguments: %w", err)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	if t.provider == nil {
		return "", fmt.Errorf("search provider is not configured")
	}

	results, err := t.provider.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("web search: %w", err)
	}
	if results == nil {
		results = []SearchResult{}
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("marshal web_search result: %w", err)
	}
	return string(raw), nil
}

// ---- DuckDuckGo provider ----

const ddgDefaultBaseURL = "https://api.duckduckgo.com"

// DuckDuckGoProvider 使用 DuckDuckGo Instant Answer API（无需 Key）搜索。
type DuckDuckGoProvider struct {
	client  *http.Client
	baseURL string
}

// NewDuckDuckGoProvider 创建 DuckDuckGo 搜索后端。
func NewDuckDuckGoProvider() *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: ddgDefaultBaseURL,
	}
}

type ddgResponse struct {
	AbstractText  string     `json:"AbstractText"`
	AbstractURL   string     `json:"AbstractURL"`
	Heading       string     `json:"Heading"`
	RelatedTopics []ddgTopic `json:"RelatedTopics"`
}

type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Topics   []ddgTopic `json:"Topics"`
}

// Search 调用 DuckDuckGo Instant Answer API。
func (p *DuckDuckGoProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	base := p.baseURL
	if base == "" {
		base = ddgDefaultBaseURL
	}
	endpoint := fmt.Sprintf("%s/?q=%s&format=json&no_html=1&skip_disambig=1", strings.TrimRight(base, "/"), url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call DuckDuckGo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DuckDuckGo returned status %s", resp.Status)
	}

	var body ddgResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode DuckDuckGo response: %w", err)
	}
	return parseDDGResponse(&body), nil
}

// parseDDGResponse 把 DuckDuckGo JSON 响应映射为 SearchResult 列表（纯函数，便于测试）。
func parseDDGResponse(body *ddgResponse) []SearchResult {
	results := make([]SearchResult, 0, 8)
	if body == nil {
		return results
	}
	if strings.TrimSpace(body.AbstractText) != "" {
		title := strings.TrimSpace(body.Heading)
		if title == "" {
			title = truncateRunes(body.AbstractText, 60)
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     body.AbstractURL,
			Snippet: body.AbstractText,
		})
	}
	var collect func(t ddgTopic)
	collect = func(t ddgTopic) {
		if strings.TrimSpace(t.Text) != "" {
			results = append(results, SearchResult{
				Title:   truncateRunes(t.Text, 60),
				URL:     t.FirstURL,
				Snippet: t.Text,
			})
		}
		for _, sub := range t.Topics {
			collect(sub)
		}
	}
	for _, topic := range body.RelatedTopics {
		collect(topic)
	}
	return results
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
