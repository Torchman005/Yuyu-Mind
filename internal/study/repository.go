package study

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository 学习数据仓储
type Repository struct {
	db *sql.DB
}

// NewRepository 创建学习数据仓储
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateSession 创建学习会话
func (r *Repository) CreateSession(ctx context.Context, session *StudySession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO study_sessions (id, user_id, start_time, end_time, mode, duration_minutes, focus_score, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.StartTime, session.EndTime,
		session.Mode, session.DurationMinutes, session.FocusScore,
		session.Notes, session.CreatedAt,
	)
	return err
}

// UpdateSession 更新学习会话
func (r *Repository) UpdateSession(ctx context.Context, session *StudySession) error {
	query := `
		UPDATE study_sessions
		SET end_time = ?, duration_minutes = ?, focus_score = ?, notes = ?
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		session.EndTime, session.DurationMinutes, session.FocusScore,
		session.Notes, session.ID,
	)
	return err
}

// GetSession 获取学习会话
func (r *Repository) GetSession(ctx context.Context, id string) (*StudySession, error) {
	query := `
		SELECT id, user_id, start_time, end_time, mode, duration_minutes, focus_score, notes, created_at
		FROM study_sessions
		WHERE id = ?
	`
	var session StudySession
	var endTime sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&session.ID, &session.UserID, &session.StartTime, &endTime,
		&session.Mode, &session.DurationMinutes, &session.FocusScore,
		&session.Notes, &session.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if endTime.Valid {
		session.EndTime = &endTime.Time
	}
	return &session, nil
}

// GetTodayRecord 获取今日学习记录
func (r *Repository) GetTodayRecord(ctx context.Context, userID string) (*StudyRecord, error) {
	today := time.Now().Format("2006-01-02")
	return r.GetRecordByDate(ctx, userID, today)
}

// GetRecordByDate 获取指定日期的学习记录
func (r *Repository) GetRecordByDate(ctx context.Context, userID, date string) (*StudyRecord, error) {
	query := `
		SELECT id, user_id, date, total_minutes, topics, screenshots, cards_created, created_at, updated_at
		FROM study_records
		WHERE user_id = ? AND date = ?
	`
	var record StudyRecord
	var topicsJSON sql.NullString
	err := r.db.QueryRowContext(ctx, query, userID, date).Scan(
		&record.ID, &record.UserID, &record.Date, &record.TotalMinutes,
		&topicsJSON, &record.Screenshots, &record.CardsCreated,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if topicsJSON.Valid && topicsJSON.String != "" {
		if err := json.Unmarshal([]byte(topicsJSON.String), &record.Topics); err != nil {
			return nil, fmt.Errorf("unmarshal topics: %w", err)
		}
	}

	return &record, nil
}

// UpsertRecord 创建或更新学习记录
func (r *Repository) UpsertRecord(ctx context.Context, record *StudyRecord) error {
	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.UpdatedAt = time.Now()

	topicsJSON, err := json.Marshal(record.Topics)
	if err != nil {
		return fmt.Errorf("marshal topics: %w", err)
	}

	query := `
		INSERT INTO study_records (id, user_id, date, total_minutes, topics, screenshots, cards_created, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			total_minutes = excluded.total_minutes,
			topics = excluded.topics,
			screenshots = excluded.screenshots,
			cards_created = excluded.cards_created,
			updated_at = excluded.updated_at
	`
	_, err = r.db.ExecContext(ctx, query,
		record.ID, record.UserID, record.Date, record.TotalMinutes,
		string(topicsJSON), record.Screenshots, record.CardsCreated,
		record.CreatedAt, record.UpdatedAt,
	)
	return err
}

// GetRecentRecords 获取最近N天的学习记录
func (r *Repository) GetRecentRecords(ctx context.Context, userID string, days int) ([]StudyRecord, error) {
	query := `
		SELECT id, user_id, date, total_minutes, topics, screenshots, cards_created, created_at, updated_at
		FROM study_records
		WHERE user_id = ?
		ORDER BY date DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, userID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []StudyRecord
	for rows.Next() {
		var record StudyRecord
		var topicsJSON sql.NullString
		if err := rows.Scan(
			&record.ID, &record.UserID, &record.Date, &record.TotalMinutes,
			&topicsJSON, &record.Screenshots, &record.CardsCreated,
			&record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if topicsJSON.Valid && topicsJSON.String != "" {
			if err := json.Unmarshal([]byte(topicsJSON.String), &record.Topics); err != nil {
				return nil, fmt.Errorf("unmarshal topics: %w", err)
			}
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

// AddMinutesToToday 增加今日学习时长
func (r *Repository) AddMinutesToToday(ctx context.Context, userID string, minutes int) error {
	today := time.Now().Format("2006-01-02")

	record, err := r.GetRecordByDate(ctx, userID, today)
	if err != nil {
		return err
	}

	if record == nil {
		record = &StudyRecord{
			UserID:       userID,
			Date:         today,
			TotalMinutes: minutes,
		}
	} else {
		record.TotalMinutes += minutes
	}

	return r.UpsertRecord(ctx, record)
}
