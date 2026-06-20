package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AgentTask struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id,omitempty"`
	ParentTaskID   string     `json:"parent_task_id,omitempty"`
	Title          string     `json:"title"`
	Goal           string     `json:"goal"`
	Status         string     `json:"status"`
	Priority       int        `json:"priority"`
	SpecJSON       string     `json:"spec_json"`
	ResultJSON     string     `json:"result_json,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type AgentTaskRepo struct {
	db *sql.DB
}

func NewAgentTaskRepo(db *sql.DB) *AgentTaskRepo {
	return &AgentTaskRepo{db: db}
}

func (r *AgentTaskRepo) Create(ctx context.Context, task *AgentTask) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agent_tasks (
			id, conversation_id, parent_task_id, title, goal, status, priority,
			spec_json, result_json, error, created_at, updated_at, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, nullableString(task.ConversationID), nullableString(task.ParentTaskID),
		task.Title, task.Goal, task.Status, task.Priority, task.SpecJSON,
		nullableString(task.ResultJSON), nullableString(task.Error),
		task.CreatedAt, task.UpdatedAt, task.StartedAt, task.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent task: %w", err)
	}
	return nil
}

func (r *AgentTaskRepo) Get(ctx context.Context, id string) (*AgentTask, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, parent_task_id, title, goal, status, priority,
			spec_json, result_json, error, created_at, updated_at, started_at, completed_at
		 FROM agent_tasks WHERE id = ?`, id,
	)
	task, err := scanAgentTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func (r *AgentTaskRepo) List(ctx context.Context, conversationID string, limit int) ([]*AgentTask, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if conversationID == "" {
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, conversation_id, parent_task_id, title, goal, status, priority,
				spec_json, result_json, error, created_at, updated_at, started_at, completed_at
			 FROM agent_tasks ORDER BY created_at DESC LIMIT ?`, limit,
		)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, conversation_id, parent_task_id, title, goal, status, priority,
				spec_json, result_json, error, created_at, updated_at, started_at, completed_at
			 FROM agent_tasks WHERE conversation_id = ? ORDER BY created_at DESC LIMIT ?`, conversationID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list agent tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*AgentTask
	for rows.Next() {
		task, err := scanAgentTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *AgentTaskRepo) NextQueued(ctx context.Context) (*AgentTask, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, parent_task_id, title, goal, status, priority,
			spec_json, result_json, error, created_at, updated_at, started_at, completed_at
		 FROM agent_tasks
		 WHERE status = 'queued'
		 ORDER BY priority DESC, created_at ASC
		 LIMIT 1`,
	)
	task, err := scanAgentTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return task, nil
}

func (r *AgentTaskRepo) ClaimNextQueued(ctx context.Context) (*AgentTask, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim task tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx,
		`SELECT id, conversation_id, parent_task_id, title, goal, status, priority,
			spec_json, result_json, error, created_at, updated_at, started_at, completed_at
		 FROM agent_tasks
		 WHERE status = 'queued'
		 ORDER BY priority DESC, created_at ASC
		 LIMIT 1`,
	)
	task, err := scanAgentTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	now := time.Now()
	result, err := tx.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = 'running', updated_at = ?, started_at = COALESCE(started_at, ?)
		 WHERE id = ? AND status = 'queued'`,
		now, now, task.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("claim agent task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check claimed rows: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim task tx: %w", err)
	}

	task.Status = "running"
	task.UpdatedAt = now
	if task.StartedAt == nil {
		task.StartedAt = &now
	}
	return task, nil
}

func (r *AgentTaskRepo) UpdateStatus(ctx context.Context, id, status, errText, resultJSON string, startedAt, completedAt *time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agent_tasks
		 SET status = ?, error = ?, result_json = ?, updated_at = ?, started_at = COALESCE(?, started_at), completed_at = COALESCE(?, completed_at)
		 WHERE id = ?`,
		status, nullableString(errText), nullableString(resultJSON), time.Now(), startedAt, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update agent task status: %w", err)
	}
	return nil
}

func scanAgentTask(scanner interface{ Scan(dest ...any) error }) (*AgentTask, error) {
	var task AgentTask
	var conversationID, parentTaskID, resultJSON, errText sql.NullString
	var startedAt, completedAt sql.NullTime
	if err := scanner.Scan(
		&task.ID, &conversationID, &parentTaskID, &task.Title, &task.Goal, &task.Status, &task.Priority,
		&task.SpecJSON, &resultJSON, &errText, &task.CreatedAt, &task.UpdatedAt, &startedAt, &completedAt,
	); err != nil {
		return nil, err
	}
	task.ConversationID = conversationID.String
	task.ParentTaskID = parentTaskID.String
	task.ResultJSON = resultJSON.String
	task.Error = errText.String
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	return &task, nil
}
