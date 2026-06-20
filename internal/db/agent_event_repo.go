package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AgentTaskEvent struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentTaskControl struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	Type      string     `json:"type"`
	Payload   string     `json:"payload,omitempty"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	AppliedAt *time.Time `json:"applied_at,omitempty"`
}

type AgentOperationLog struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Kind      string    `json:"kind"`
	Target    string    `json:"target"`
	Summary   string    `json:"summary"`
	Status    string    `json:"status"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentEventRepo struct {
	db *sql.DB
}

func NewAgentEventRepo(db *sql.DB) *AgentEventRepo {
	return &AgentEventRepo{db: db}
}

func (r *AgentEventRepo) AddEvent(ctx context.Context, e *AgentTaskEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agent_task_events (id, task_id, type, level, message, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.TaskID, e.Type, e.Level, e.Message, nullableString(e.Payload), e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent task event: %w", err)
	}
	return nil
}

func (r *AgentEventRepo) ListEvents(ctx context.Context, taskID string) ([]*AgentTaskEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_id, type, level, message, payload, created_at
		 FROM agent_task_events WHERE task_id = ? ORDER BY created_at ASC`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent task events: %w", err)
	}
	defer rows.Close()

	var events []*AgentTaskEvent
	for rows.Next() {
		var e AgentTaskEvent
		var payload sql.NullString
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Type, &e.Level, &e.Message, &payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent task event: %w", err)
		}
		e.Payload = payload.String
		events = append(events, &e)
	}
	return events, rows.Err()
}

func (r *AgentEventRepo) AddControl(ctx context.Context, c *AgentTaskControl) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agent_task_controls (id, task_id, type, payload, status, created_at, applied_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TaskID, c.Type, nullableString(c.Payload), c.Status, c.CreatedAt, c.AppliedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent task control: %w", err)
	}
	return nil
}

func (r *AgentEventRepo) PendingControls(ctx context.Context, taskID string) ([]*AgentTaskControl, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_id, type, payload, status, created_at, applied_at
		 FROM agent_task_controls WHERE task_id = ? AND status = 'pending' ORDER BY created_at ASC`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent task controls: %w", err)
	}
	defer rows.Close()

	var controls []*AgentTaskControl
	for rows.Next() {
		control, err := scanAgentTaskControl(rows)
		if err != nil {
			return nil, err
		}
		controls = append(controls, control)
	}
	return controls, rows.Err()
}

func (r *AgentEventRepo) MarkControlApplied(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`UPDATE agent_task_controls SET status = 'applied', applied_at = ? WHERE id = ?`, now, id,
	)
	if err != nil {
		return fmt.Errorf("mark agent task control applied: %w", err)
	}
	return nil
}

func (r *AgentEventRepo) AddOperation(ctx context.Context, op *AgentOperationLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agent_operation_logs (id, task_id, kind, target, summary, status, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.TaskID, op.Kind, op.Target, op.Summary, op.Status, nullableString(op.Payload), op.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent operation log: %w", err)
	}
	return nil
}

func scanAgentTaskControl(scanner interface{ Scan(dest ...any) error }) (*AgentTaskControl, error) {
	var c AgentTaskControl
	var payload sql.NullString
	var appliedAt sql.NullTime
	if err := scanner.Scan(&c.ID, &c.TaskID, &c.Type, &payload, &c.Status, &c.CreatedAt, &appliedAt); err != nil {
		return nil, fmt.Errorf("scan agent task control: %w", err)
	}
	c.Payload = payload.String
	if appliedAt.Valid {
		c.AppliedAt = &appliedAt.Time
	}
	return &c, nil
}
