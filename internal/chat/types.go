package chat

type ChatRequest struct {
	ConversationID string `json:"conversation_id"`
	SessionID      string `json:"session_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	SenderID       string `json:"sender_id,omitempty"`
	SenderName     string `json:"sender_name,omitempty"`
	Content        string `json:"content"`
	Mentioned      bool   `json:"mentioned,omitempty"`
	SourceKind     string `json:"source_kind,omitempty"`
	UseTools       bool   `json:"use_tools"`
}

type ChatEventType string

const (
	EventTypeToken      ChatEventType = "token"
	EventTypeToolCall   ChatEventType = "tool_call"
	EventTypeToolResult ChatEventType = "tool_result"
	EventTypeEmotion    ChatEventType = "emotion"
	EventTypeDone       ChatEventType = "done"
	EventTypeError      ChatEventType = "error"
)

type ChatEvent struct {
	Type     ChatEventType `json:"type"`
	Content  string        `json:"content,omitempty"`
	ToolID   string        `json:"tool_id,omitempty"`
	ToolName string        `json:"tool_name,omitempty"`

	// 情绪 Schema（见 emotion.go）。EventTypeEmotion 事件携带完整表演参数。
	Emotion string  `json:"emotion,omitempty"`
	Mood    string  `json:"mood,omitempty"`
	Energy  float64 `json:"energy,omitempty"`
	Gesture string  `json:"gesture,omitempty"`
	Hand    string  `json:"hand,omitempty"`
}

type ConversationInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type MessageInfo struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCalls  string `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	SourceKind string `json:"source_kind,omitempty"`
	CreatedAt  string `json:"created_at"`
}

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
