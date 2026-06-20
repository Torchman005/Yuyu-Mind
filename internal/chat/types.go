package chat

// ChatRequest 是前端发送的一次聊天请求。
type ChatRequest struct {
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	UseTools       bool   `json:"use_tools"` // 是否启用工具调用
}

// ChatEventType 标识流式事件类型。
type ChatEventType string

const (
	EventTypeToken      ChatEventType = "token"       // 流式文本片段
	EventTypeToolCall   ChatEventType = "tool_call"   // 模型决定调用工具
	EventTypeToolResult ChatEventType = "tool_result" // 工具执行结果
	EventTypeDone       ChatEventType = "done"        // 流式响应完成
	EventTypeError      ChatEventType = "error"       // 请求发生错误
)

// ChatEvent 表示发送给前端的流式事件。
type ChatEvent struct {
	Type     ChatEventType `json:"type"`
	Content  string        `json:"content,omitempty"`
	ToolID   string        `json:"tool_id,omitempty"`
	ToolName string        `json:"tool_name,omitempty"`
}

// ConversationInfo 是返回给前端的会话摘要。
type ConversationInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MessageInfo 是返回给前端的单条消息。
type MessageInfo struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCalls  string `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// TokenUsageInfo 是一次请求的 token 用量明细。
type TokenUsageInfo struct {
	ID               string `json:"id"`
	ConversationID   string `json:"conversation_id"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Mode             string `json:"mode"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	ModelCalls       int    `json:"model_calls"`
	DurationMS       int64  `json:"duration_ms"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	CreatedAt        string `json:"created_at"`
}

// TokenUsageSummary 是 token 用量聚合结果。
type TokenUsageSummary struct {
	ConversationID    string `json:"conversation_id,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	RequestCount      int    `json:"request_count"`
	FailedCount       int    `json:"failed_count"`
	ModelCalls        int    `json:"model_calls"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	TotalTokens       int    `json:"total_tokens"`
	TotalDurationMS   int64  `json:"total_duration_ms"`
	AverageDurationMS int64  `json:"average_duration_ms"`
}
