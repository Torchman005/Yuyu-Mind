package chat

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/memory"
)

type RuntimeState string

const (
	RuntimeIdle    RuntimeState = "idle"
	RuntimeWaiting RuntimeState = "waiting"
	RuntimeRunning RuntimeState = "running"
)

type NormalizedMessage struct {
	ID             string
	SessionID      string
	ConversationID string
	SenderID       string
	SenderName     string
	Content        string
	Mentioned      bool
	SourceKind     string
	CreatedAt      time.Time
}

type TurnSnapshot struct {
	SessionID      string
	ConversationID string
	State          RuntimeState
	Pending        []NormalizedMessage
	History        []*schema.Message
	Target         NormalizedMessage
	LastInboundAt  time.Time
	LastBotAt      time.Time
	BotStreak      int
	Now            time.Time
}

type RuntimeManager struct {
	mu       sync.Mutex
	memory   memory.Store
	runtimes map[string]*ConversationRuntime
}

func NewRuntimeManager(memStore memory.Store) *RuntimeManager {
	return &RuntimeManager{
		memory:   memStore,
		runtimes: make(map[string]*ConversationRuntime),
	}
}

func (m *RuntimeManager) Get(sessionID string) *ConversationRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()

	rt := m.runtimes[sessionID]
	if rt == nil {
		rt = &ConversationRuntime{
			sessionID: sessionID,
			state:     RuntimeIdle,
			seen:      make(map[string]struct{}),
			memory:    m.memory,
		}
		m.runtimes[sessionID] = rt
	}
	return rt
}

type ConversationRuntime struct {
	mu sync.Mutex

	sessionID string
	memory    memory.Store
	loaded    bool
	seen      map[string]struct{}

	state         RuntimeState
	pending       []NormalizedMessage
	history       []*schema.Message
	lastInboundAt time.Time
	lastBotAt     time.Time
	botStreak     int
}

func (r *ConversationRuntime) Ingest(ctx context.Context, msg NormalizedMessage) (TurnSnapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureLoaded(ctx, msg.ConversationID); err != nil {
		return TurnSnapshot{}, false, err
	}
	if _, ok := r.seen[msg.ID]; ok {
		return r.snapshotLocked(time.Now()), false, nil
	}

	r.seen[msg.ID] = struct{}{}
	r.pending = append(r.pending, msg)
	r.history = append(r.history, &schema.Message{Role: schema.User, Content: msg.Content})
	r.history = trimSchemaHistory(r.history, 80)
	r.lastInboundAt = msg.CreatedAt
	if r.state != RuntimeRunning {
		r.state = RuntimeWaiting
	}

	return r.snapshotLocked(time.Now()), true, nil
}

func (r *ConversationRuntime) MarkRunning() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = RuntimeRunning
}

func (r *ConversationRuntime) MarkWaiting() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != RuntimeRunning {
		r.state = RuntimeWaiting
	}
}

func (r *ConversationRuntime) CompleteReply(sent []string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, content := range sent {
		r.history = append(r.history, &schema.Message{Role: schema.Assistant, Content: content})
	}
	r.history = trimSchemaHistory(r.history, 80)
	r.pending = nil
	r.state = RuntimeIdle
	r.lastBotAt = at
	r.botStreak++
}

func (r *ConversationRuntime) CompleteNoReply() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) > 0 {
		r.state = RuntimeWaiting
	} else {
		r.state = RuntimeIdle
	}
}

func (r *ConversationRuntime) ensureLoaded(ctx context.Context, conversationID string) error {
	if r.loaded || r.memory == nil {
		r.loaded = true
		return nil
	}

	history, err := r.memory.GetHistory(ctx, conversationID)
	if err != nil {
		return err
	}
	r.history = append(r.history, history...)
	r.history = trimSchemaHistory(r.history, 80)
	r.loaded = true
	return nil
}

func (r *ConversationRuntime) snapshotLocked(now time.Time) TurnSnapshot {
	pending := append([]NormalizedMessage(nil), r.pending...)
	history := append([]*schema.Message(nil), r.history...)
	target := NormalizedMessage{}
	if len(pending) > 0 {
		target = pending[len(pending)-1]
	}
	return TurnSnapshot{
		SessionID:      r.sessionID,
		ConversationID: target.ConversationID,
		State:          r.state,
		Pending:        pending,
		History:        history,
		Target:         target,
		LastInboundAt:  r.lastInboundAt,
		LastBotAt:      r.lastBotAt,
		BotStreak:      r.botStreak,
		Now:            now,
	}
}

func trimSchemaHistory(messages []*schema.Message, max int) []*schema.Message {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	return messages[len(messages)-max:]
}
