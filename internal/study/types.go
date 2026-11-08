package study

import "time"

// StudyMode 学习模式
type StudyMode string

const (
	ModeStudy StudyMode = "study" // 学习中
	ModeBreak StudyMode = "break" // 休息中
	ModeIdle  StudyMode = "idle"  // 空闲
)

// StudySession 学习会话
type StudySession struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	StartTime       time.Time `json:"start_time"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	Mode            StudyMode `json:"mode"`
	DurationMinutes int       `json:"duration_minutes"`
	FocusScore      float64   `json:"focus_score"`
	Notes           string    `json:"notes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// StudyRecord 每日学习记录
type StudyRecord struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Date         string    `json:"date"` // YYYY-MM-DD
	TotalMinutes int       `json:"total_minutes"`
	Topics       []string  `json:"topics,omitempty"`
	Screenshots  int       `json:"screenshots"`
	CardsCreated int       `json:"cards_created"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Flashcard 记忆卡片
type Flashcard struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Question     string    `json:"question"`
	Answer       string    `json:"answer"`
	Tags         []string  `json:"tags,omitempty"`
	Source       string    `json:"source,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	NextReviewAt *time.Time `json:"next_review_at,omitempty"`
	ReviewCount  int       `json:"review_count"`
	Difficulty   int       `json:"difficulty"`
}

// StudyGoal 学习目标
type StudyGoal struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	TargetMinutes int       `json:"target_minutes,omitempty"`
	Deadline      *string   `json:"deadline,omitempty"` // YYYY-MM-DD
	Status        string    `json:"status"` // active, completed, cancelled
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// PomodoroConfig 番茄钟配置
type PomodoroConfig struct {
	StudyDuration int `json:"study_duration"` // 学习时长（分钟）
	BreakDuration int `json:"break_duration"` // 休息时长（分钟）
	LongBreakDuration int `json:"long_break_duration"` // 长休息时长（分钟）
	SessionsBeforeLongBreak int `json:"sessions_before_long_break"` // 长休息前的学习次数
}

// StudyStats 学习统计
type StudyStats struct {
	TodayMinutes     int            `json:"today_minutes"`
	WeekMinutes      int            `json:"week_minutes"`
	TotalSessions    int            `json:"total_sessions"`
	CurrentStreak    int            `json:"current_streak"` // 连续学习天数
	DailyBreakdown   []DailyStat    `json:"daily_breakdown,omitempty"`
}

// DailyStat 每日统计
type DailyStat struct {
	Date    string `json:"date"`
	Minutes int    `json:"minutes"`
}
