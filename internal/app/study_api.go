package app

import (
	"github.com/yuyu-mind/backend/internal/study"
)

// StartStudySession 开始学习会话
func (a *App) StartStudySession() (*study.StudySession, error) {
	return a.studySvc.StartStudy(a.ctx, "default")
}

// StartBreak 开始休息
func (a *App) StartBreak(isLongBreak bool) (*study.StudySession, error) {
	return a.studySvc.StartBreak(a.ctx, "default", isLongBreak)
}

// PauseStudySession 暂停当前会话
func (a *App) PauseStudySession() error {
	return a.studySvc.PauseSession()
}

// ResumeStudySession 恢复当前会话
func (a *App) ResumeStudySession() error {
	return a.studySvc.ResumeSession()
}

// StopStudySession 停止当前会话
func (a *App) StopStudySession() error {
	return a.studySvc.StopSession(a.ctx)
}

// GetCurrentStudySession 获取当前会话状态
func (a *App) GetCurrentStudySession() *study.ActiveSession {
	return a.studySvc.GetCurrentSession()
}

// GetTodayStudyStats 获取今日学习统计
func (a *App) GetTodayStudyStats() (*study.StudyRecord, error) {
	return a.studySvc.GetTodayStats(a.ctx, "default")
}

// GetRecentStudyStats 获取最近N天的学习统计
func (a *App) GetRecentStudyStats(days int) ([]study.StudyRecord, error) {
	return a.studySvc.GetRecentStats(a.ctx, "default", days)
}
