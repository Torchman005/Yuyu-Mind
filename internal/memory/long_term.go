package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/db"
)

const (
	MemoryScopeUser    = "user"
	MemoryScopeProject = "project"

	MemoryKindPreference  = "preference"
	MemoryKindFact        = "fact"
	MemoryKindProject     = "project"
	MemoryKindInstruction = "instruction"
	MemoryKindEpisode     = "episode"

	MemoryStatusActive   = "active"
	MemoryStatusArchived = "archived"
	MemoryStatusRejected = "rejected"

	MemorySourceExplicit = "user_explicit"
	MemorySourceInferred = "model_inferred"
	MemorySourceSystem   = "system"
)

type UpsertMemoryRequest struct {
	Scope           string  `json:"scope"`
	Kind            string  `json:"kind"`
	Key             string  `json:"key"`
	Value           any     `json:"value"`
	Text            string  `json:"text"`
	Confidence      float64 `json:"confidence"`
	Source          string  `json:"source"`
	SourceMessageID string  `json:"source_message_id,omitempty"`
}

type AddCandidateRequest struct {
	Scope         string  `json:"scope"`
	Kind          string  `json:"kind"`
	Key           string  `json:"key"`
	Value         any     `json:"value"`
	Text          string  `json:"text"`
	EvidenceCount int     `json:"evidence_count"`
	Confidence    float64 `json:"confidence"`
}

type TaskContextRequest struct {
	TaskID         string   `json:"task_id,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	Query          string   `json:"query,omitempty"`
}

type TaskContext struct {
	Preferences         map[string]any `json:"preferences"`
	Facts               map[string]any `json:"facts,omitempty"`
	Project             map[string]any `json:"project,omitempty"`
	Instructions        []string       `json:"instructions,omitempty"`
	Episodes            []string       `json:"episodes,omitempty"`
	ConversationSummary string         `json:"conversation_summary,omitempty"`
	MemoryIDs           []string       `json:"memory_ids"`
}

type ServiceMemory struct {
	repo *db.MemoryRepo
}

func NewServiceMemory(repo *db.MemoryRepo) *ServiceMemory {
	return &ServiceMemory{repo: repo}
}

func (s *ServiceMemory) UpsertMemory(ctx context.Context, req UpsertMemoryRequest) (*db.UserMemory, error) {
	if req.Scope == "" {
		req.Scope = MemoryScopeUser
	}
	if req.Kind == "" {
		req.Kind = MemoryKindPreference
	}
	if req.Key == "" {
		return nil, fmt.Errorf("memory key is required")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("memory text is required")
	}
	if req.Confidence <= 0 {
		req.Confidence = 1
	}
	if req.Source == "" {
		req.Source = MemorySourceExplicit
	}

	now := time.Now()
	memory := &db.UserMemory{
		ID:              uuid.New().String(),
		Scope:           req.Scope,
		Kind:            req.Kind,
		Key:             req.Key,
		ValueJSON:       encodeJSON(req.Value),
		Text:            req.Text,
		Confidence:      req.Confidence,
		Source:          req.Source,
		SourceMessageID: req.SourceMessageID,
		Status:          MemoryStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing, err := s.repo.GetMemoryByKey(ctx, req.Scope, req.Kind, req.Key); err == nil && existing != nil {
		memory.ID = existing.ID
		memory.CreatedAt = existing.CreatedAt
		memory.UseCount = existing.UseCount
		memory.LastUsedAt = existing.LastUsedAt
	}

	if err := s.repo.UpsertMemory(ctx, memory); err != nil {
		return nil, err
	}
	_ = s.repo.AddEvent(ctx, &db.MemoryEvent{
		ID:        uuid.New().String(),
		MemoryID:  memory.ID,
		Type:      "upsert",
		Message:   "写入或更新长期记忆。",
		Payload:   encodeJSON(req),
		CreatedAt: now,
	})
	return memory, nil
}

func (s *ServiceMemory) SearchMemories(ctx context.Context, scope, kind, query string, limit int) ([]*db.UserMemory, error) {
	return s.repo.SearchMemories(ctx, scope, kind, query, limit)
}

func (s *ServiceMemory) ArchiveMemory(ctx context.Context, id string) error {
	if err := s.repo.ArchiveMemory(ctx, id); err != nil {
		return err
	}
	return s.repo.AddEvent(ctx, &db.MemoryEvent{
		ID:        uuid.New().String(),
		MemoryID:  id,
		Type:      "archive",
		Message:   "归档长期记忆。",
		CreatedAt: time.Now(),
	})
}

func (s *ServiceMemory) AddCandidate(ctx context.Context, req AddCandidateRequest) (*db.MemoryCandidate, error) {
	if req.Scope == "" {
		req.Scope = MemoryScopeUser
	}
	if req.Kind == "" {
		req.Kind = MemoryKindPreference
	}
	if req.Key == "" {
		return nil, fmt.Errorf("candidate key is required")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("candidate text is required")
	}
	if req.EvidenceCount <= 0 {
		req.EvidenceCount = 1
	}
	if req.Confidence <= 0 {
		req.Confidence = 0.5
	}
	now := time.Now()
	candidate := &db.MemoryCandidate{
		ID:            uuid.New().String(),
		Scope:         req.Scope,
		Kind:          req.Kind,
		Key:           req.Key,
		ValueJSON:     encodeJSON(req.Value),
		Text:          req.Text,
		EvidenceCount: req.EvidenceCount,
		Confidence:    req.Confidence,
		Status:        "pending",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.AddCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (s *ServiceMemory) ListCandidates(ctx context.Context, status string, limit int) ([]*db.MemoryCandidate, error) {
	return s.repo.ListCandidates(ctx, status, limit)
}

func (s *ServiceMemory) PromoteCandidate(ctx context.Context, candidateID string) (*db.UserMemory, error) {
	candidate, err := s.repo.GetCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, fmt.Errorf("memory candidate %q not found", candidateID)
	}

	var value any
	_ = json.Unmarshal([]byte(candidate.ValueJSON), &value)
	memory, err := s.UpsertMemory(ctx, UpsertMemoryRequest{
		Scope:      candidate.Scope,
		Kind:       candidate.Kind,
		Key:        candidate.Key,
		Value:      value,
		Text:       candidate.Text,
		Confidence: candidate.Confidence,
		Source:     MemorySourceInferred,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateCandidateStatus(ctx, candidateID, "promoted"); err != nil {
		return nil, err
	}
	_ = s.repo.AddEvent(ctx, &db.MemoryEvent{
		ID:        uuid.New().String(),
		MemoryID:  memory.ID,
		Type:      "promote_candidate",
		Message:   "候选记忆已转为长期记忆。",
		Payload:   encodeJSON(candidate),
		CreatedAt: time.Now(),
	})
	return memory, nil
}

func (s *ServiceMemory) RejectCandidate(ctx context.Context, candidateID string) error {
	return s.repo.UpdateCandidateStatus(ctx, candidateID, "rejected")
}

func (s *ServiceMemory) UpsertConversationSummary(ctx context.Context, conversationID, summary string, tokenEstimate int) (*db.ConversationSummary, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("conversation id is required")
	}
	row := &db.ConversationSummary{
		ConversationID: conversationID,
		Summary:        summary,
		TokenEstimate:  tokenEstimate,
		UpdatedAt:      time.Now(),
	}
	if err := s.repo.UpsertConversationSummary(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *ServiceMemory) BuildTaskContext(ctx context.Context, req TaskContextRequest) (*TaskContext, error) {
	if len(req.Scopes) == 0 {
		req.Scopes = []string{MemoryScopeUser, MemoryScopeProject}
	}

	result := &TaskContext{
		Preferences: make(map[string]any),
		Facts:       make(map[string]any),
		Project:     make(map[string]any),
		MemoryIDs:   make([]string, 0),
	}

	seen := make(map[string]bool)
	for _, scope := range req.Scopes {
		for _, kind := range []string{MemoryKindPreference, MemoryKindProject, MemoryKindInstruction} {
			memories, err := s.repo.SearchMemories(ctx, scope, kind, "", 100)
			if err != nil {
				return nil, err
			}
			s.applyMemories(ctx, result, memories, seen)
		}

		for _, kind := range []string{MemoryKindFact, MemoryKindEpisode} {
			memories, err := s.repo.SearchMemories(ctx, scope, kind, req.Query, 50)
			if err != nil {
				return nil, err
			}
			s.applyMemories(ctx, result, memories, seen)
		}
	}

	if req.ConversationID != "" {
		summary, err := s.repo.GetConversationSummary(ctx, req.ConversationID)
		if err != nil {
			return nil, err
		}
		if summary != nil {
			result.ConversationSummary = summary.Summary
		}
	}

	snapshot := &db.TaskContextSnapshot{
		ID:          uuid.New().String(),
		TaskID:      req.TaskID,
		Scope:       "task_projection",
		ContextJSON: encodeJSON(result),
		CreatedAt:   time.Now(),
	}
	if err := s.repo.CreateTaskContextSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ServiceMemory) applyMemories(ctx context.Context, result *TaskContext, memories []*db.UserMemory, seen map[string]bool) {
	for _, memory := range memories {
		if seen[memory.ID] {
			continue
		}
		seen[memory.ID] = true
		value := decodeAny(memory.ValueJSON)
		switch memory.Kind {
		case MemoryKindPreference:
			result.Preferences[memory.Key] = value
		case MemoryKindFact:
			result.Facts[memory.Key] = value
		case MemoryKindProject:
			result.Project[memory.Key] = value
		case MemoryKindInstruction:
			result.Instructions = append(result.Instructions, memory.Text)
		case MemoryKindEpisode:
			result.Episodes = append(result.Episodes, memory.Text)
		}
		result.MemoryIDs = append(result.MemoryIDs, memory.ID)
		_ = s.repo.MarkMemoryUsed(ctx, memory.ID)
	}
}

func encodeJSON(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

func decodeAny(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}
