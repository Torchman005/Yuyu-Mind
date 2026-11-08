package study

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service 学习服务
type Service struct {
	repo   *Repository
	config PomodoroConfig

	mu              sync.RWMutex
	currentSession  *ActiveSession
	sessionCount    int // 当前已完成的学习会话数
	ticker          *time.Ticker
	stopChan        chan struct{}
	callbackHandler func(event StudyEvent)
}

// ActiveSession 当前活跃的学习会话
type ActiveSession struct {
	Session         *StudySession
	RemainingSeconds int
	IsPaused        bool
}

// StudyEvent 学习事件
type StudyEvent struct {
	Type      string                 `json:"type"` // session_start, session_end, mode_change, tick, reminder
	SessionID string                 `json:"session_id,omitempty"`
	Mode      StudyMode              `json:"mode,omitempty"`
	Remaining int                    `json:"remaining,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NewService 创建学习服务
func NewService(repo *Repository, config PomodoroConfig) *Service {
	if config.StudyDuration == 0 {
		config.StudyDuration = 25 // 默认 25 分钟
	}
	if config.BreakDuration == 0 {
		config.BreakDuration = 5 // 默认 5 分钟
	}
	if config.LongBreakDuration == 0 {
		config.LongBreakDuration = 15 // 默认 15 分钟
	}
	if config.SessionsBeforeLongBreak == 0 {
		config.SessionsBeforeLongBreak = 4 // 默认 4 次后长休息
	}

	return &Service{
		repo:     repo,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// SetCallbackHandler 设置事件回调
func (s *Service) SetCallbackHandler(handler func(event StudyEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbackHandler = handler
}

// StartStudy 开始学习
func (s *Service) StartStudy(ctx context.Context, userID string) (*StudySession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession != nil {
		return nil, fmt.Errorf("already in session")
	}

	session := &StudySession{
		ID:        uuid.New().String(),
		UserID:    userID,
		StartTime: time.Now(),
		Mode:      ModeStudy,
		CreatedAt: time.Now(),
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	s.currentSession = &ActiveSession{
		Session:          session,
		RemainingSeconds: s.config.StudyDuration * 60,
		IsPaused:         false,
	}

	// 启动计时器
	s.startTicker()

	s.emitEvent(StudyEvent{
		Type:      "session_start",
		SessionID: session.ID,
		Mode:      ModeStudy,
		Remaining: s.currentSession.RemainingSeconds,
		Message:   "开始学习",
	})

	slog.Info("study session started", "session_id", session.ID, "duration", s.config.StudyDuration)
	return session, nil
}

// StartBreak 开始休息
func (s *Service) StartBreak(ctx context.Context, userID string, isLongBreak bool) (*StudySession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession != nil {
		return nil, fmt.Errorf("already in session")
	}

	duration := s.config.BreakDuration
	if isLongBreak {
		duration = s.config.LongBreakDuration
	}

	session := &StudySession{
		ID:        uuid.New().String(),
		UserID:    userID,
		StartTime: time.Now(),
		Mode:      ModeBreak,
		CreatedAt: time.Now(),
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	s.currentSession = &ActiveSession{
		Session:          session,
		RemainingSeconds: duration * 60,
		IsPaused:         false,
	}

	s.startTicker()

	s.emitEvent(StudyEvent{
		Type:      "session_start",
		SessionID: session.ID,
		Mode:      ModeBreak,
		Remaining: s.currentSession.RemainingSeconds,
		Message:   "开始休息",
	})

	slog.Info("break session started", "session_id", session.ID, "duration", duration, "long_break", isLongBreak)
	return session, nil
}

// PauseSession 暂停当前会话
func (s *Service) PauseSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	s.currentSession.IsPaused = true
	s.stopTicker()

	s.emitEvent(StudyEvent{
		Type:      "session_paused",
		SessionID: s.currentSession.Session.ID,
		Message:   "已暂停",
	})

	return nil
}

// ResumeSession 恢复当前会话
func (s *Service) ResumeSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	s.currentSession.IsPaused = false
	s.startTicker()

	s.emitEvent(StudyEvent{
		Type:      "session_resumed",
		SessionID: s.currentSession.Session.ID,
		Message:   "已恢复",
	})

	return nil
}

// StopSession 停止当前会话
func (s *Service) StopSession(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	s.stopTicker()

	// 更新会话结束时间和时长
	now := time.Now()
	s.currentSession.Session.EndTime = &now
	elapsed := int(now.Sub(s.currentSession.Session.StartTime).Minutes())
	s.currentSession.Session.DurationMinutes = elapsed

	if err := s.repo.UpdateSession(ctx, s.currentSession.Session); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	// 如果是学习会话，更新今日记录
	if s.currentSession.Session.Mode == ModeStudy {
		if err := s.repo.AddMinutesToToday(ctx, s.currentSession.Session.UserID, elapsed); err != nil {
			slog.Error("failed to update today record", "error", err)
		}
		s.sessionCount++
	}

	sessionID := s.currentSession.Session.ID
	mode := s.currentSession.Session.Mode

	s.emitEvent(StudyEvent{
		Type:      "session_end",
		SessionID: sessionID,
		Mode:      mode,
		Message:   "会话结束",
		Data: map[string]interface{}{
			"duration_minutes": elapsed,
		},
	})

	s.currentSession = nil
	slog.Info("session stopped", "session_id", sessionID, "duration", elapsed)

	return nil
}

// GetCurrentSession 获取当前会话状态
func (s *Service) GetCurrentSession() *ActiveSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentSession
}

// GetTodayStats 获取今日统计
func (s *Service) GetTodayStats(ctx context.Context, userID string) (*StudyRecord, error) {
	return s.repo.GetTodayRecord(ctx, userID)
}

// GetRecentStats 获取最近统计
func (s *Service) GetRecentStats(ctx context.Context, userID string, days int) ([]StudyRecord, error) {
	return s.repo.GetRecentRecords(ctx, userID, days)
}

// startTicker 启动计时器
func (s *Service) startTicker() {
	if s.ticker != nil {
		s.ticker.Stop()
	}

	s.ticker = time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.onTick()
			case <-s.stopChan:
				return
			}
		}
	}()
}

// stopTicker 停止计时器
func (s *Service) stopTicker() {
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
}

// onTick 计时器回调
func (s *Service) onTick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession == nil || s.currentSession.IsPaused {
		return
	}

	s.currentSession.RemainingSeconds--

	// 发送 tick 事件（用于 UI 更新）
	s.emitEvent(StudyEvent{
		Type:      "tick",
		SessionID: s.currentSession.Session.ID,
		Mode:      s.currentSession.Session.Mode,
		Remaining: s.currentSession.RemainingSeconds,
	})

	// 检查提醒时机
	s.checkReminders()

	// 时间到
	if s.currentSession.RemainingSeconds <= 0 {
		s.onSessionComplete()
	}
}

// checkReminders 检查是否需要发送提醒
func (s *Service) checkReminders() {
	remaining := s.currentSession.RemainingSeconds

	// 学习模式：每 25 分钟提醒一次休息眼睛
	if s.currentSession.Session.Mode == ModeStudy {
		if remaining == 5*60 { // 还剩 5 分钟
			s.emitEvent(StudyEvent{
				Type:      "reminder",
				SessionID: s.currentSession.Session.ID,
				Message:   "还有 5 分钟，继续加油~",
			})
		}
	}

	// 休息模式：时间快到了
	if s.currentSession.Session.Mode == ModeBreak {
		if remaining == 1*60 { // 还剩 1 分钟
			s.emitEvent(StudyEvent{
				Type:      "reminder",
				SessionID: s.currentSession.Session.ID,
				Message:   "休息时间快结束了，准备继续学习~",
			})
		}
	}
}

// onSessionComplete 会话完成
func (s *Service) onSessionComplete() {
	mode := s.currentSession.Session.Mode
	sessionID := s.currentSession.Session.ID

	s.emitEvent(StudyEvent{
		Type:      "session_complete",
		SessionID: sessionID,
		Mode:      mode,
		Message:   s.getCompleteMessage(),
	})

	slog.Info("session completed", "session_id", sessionID, "mode", mode)
}

// getCompleteMessage 获取完成提示消息
func (s *Service) getCompleteMessage() string {
	if s.currentSession.Session.Mode == ModeStudy {
		// 判断是否应该长休息
		if s.sessionCount > 0 && s.sessionCount%s.config.SessionsBeforeLongBreak == 0 {
			return "太棒了！已经完成了一个番茄钟，休息一下吧~（建议长休息 15 分钟）"
		}
		return "太棒了！已经完成了一个番茄钟，休息一下吧~"
	}
	return "休息时间到，准备继续学习了吗？"
}

// emitEvent 发送事件
func (s *Service) emitEvent(event StudyEvent) {
	if s.callbackHandler != nil {
		// 异步发送，避免阻塞
		go s.callbackHandler(event)
	}
}

// Close 关闭服务
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.stopChan)
	s.stopTicker()
}
