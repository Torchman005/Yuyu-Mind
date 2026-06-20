package agent

import "encoding/json"

const (
	TaskStatusQueued             = "queued"
	TaskStatusRunning            = "running"
	TaskStatusWaitingForInput    = "waiting_for_input"
	TaskStatusWaitingForApproval = "waiting_for_approval"
	TaskStatusCompleted          = "completed"
	TaskStatusFailed             = "failed"
	TaskStatusCancelled          = "cancelled"
)

const (
	EventTypeProgress  = "progress"
	EventTypeQuestion  = "question"
	EventTypeError     = "error"
	EventTypeResult    = "result"
	EventTypeAudit     = "audit"
	EventTypeControl   = "control"
	EventTypeOperation = "operation"
)

const (
	ControlTypeCancel  = "cancel"
	ControlTypeInput   = "input"
	ControlTypeApprove = "approve"
	ControlTypeReject  = "reject"
	ControlTypeRevise  = "revise"
)

type TaskSpec struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	ParentTaskID   string         `json:"parent_task_id,omitempty"`
	Title          string         `json:"title"`
	Goal           string         `json:"goal"`
	Instructions   string         `json:"instructions"`
	Workspace      string         `json:"workspace"`
	Constraints    []string       `json:"constraints,omitempty"`
	Context        map[string]any `json:"context,omitempty"`
	AllowedActions []string       `json:"allowed_actions,omitempty"`
	Priority       int            `json:"priority,omitempty"`
}

type TaskResult struct {
	Summary    string         `json:"summary"`
	Artifacts  []string       `json:"artifacts,omitempty"`
	NeedReview bool           `json:"need_review,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type QuestionPayload struct {
	Question         string   `json:"question"`
	Reason           string   `json:"reason"`
	SuggestedOptions []string `json:"suggested_options,omitempty"`
}

type InputPayload struct {
	Answer string `json:"answer"`
}

func encodeJSON(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeJSON[T any](raw string) (T, error) {
	var value T
	if raw == "" {
		return value, nil
	}
	err := json.Unmarshal([]byte(raw), &value)
	return value, err
}
