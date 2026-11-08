import React from 'react';
import './StudyMode.css';

export interface PomodoroState {
  mode: 'idle' | 'study' | 'break';
  remainingSeconds: number;
  totalSeconds: number;
  isPaused: boolean;
  sessionCount: number;
}

export interface StudyStats {
  todayMinutes: number;
  weekMinutes: number;
  currentStreak: number;
}

interface StudyModeProps {
  pomodoro: PomodoroState;
  stats: StudyStats;
  onStartStudy: () => void;
  onStartBreak: (isLong: boolean) => void;
  onPause: () => void;
  onResume: () => void;
  onStop: () => void;
}

export const StudyMode: React.FC<StudyModeProps> = ({
  pomodoro,
  stats,
  onStartStudy,
  onStartBreak,
  onPause,
  onResume,
  onStop,
}) => {
  const formatTime = (seconds: number): string => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  const getProgress = (): number => {
    if (pomodoro.totalSeconds === 0) return 0;
    return ((pomodoro.totalSeconds - pomodoro.remainingSeconds) / pomodoro.totalSeconds) * 100;
  };

  const getModeText = (): string => {
    switch (pomodoro.mode) {
      case 'study': return '专注学习中';
      case 'break': return '休息中';
      default: return '准备开始';
    }
  };

  const getModeColor = (): string => {
    switch (pomodoro.mode) {
      case 'study': return '#4CAF50';
      case 'break': return '#FF9800';
      default: return '#9E9E9E';
    }
  };

  return (
    <div className="study-mode-container">
      {/* 顶部统计卡片 */}
      <div className="stats-cards">
        <div className="stat-card">
          <div className="stat-icon">📚</div>
          <div className="stat-content">
            <div className="stat-value">{stats.todayMinutes}</div>
            <div className="stat-label">今日学习（分钟）</div>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-icon">📅</div>
          <div className="stat-content">
            <div className="stat-value">{stats.weekMinutes}</div>
            <div className="stat-label">本周学习（分钟）</div>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-icon">🔥</div>
          <div className="stat-content">
            <div className="stat-value">{stats.currentStreak}</div>
            <div className="stat-label">连续天数</div>
          </div>
        </div>
      </div>

      {/* 番茄钟主界面 */}
      <div className="pomodoro-main">
        <div className="timer-wrapper">
          {/* 圆形进度条 */}
          <svg className="progress-ring" width="280" height="280">
            <circle
              className="progress-ring-bg"
              cx="140"
              cy="140"
              r="120"
              fill="none"
              stroke="#E0E0E0"
              strokeWidth="12"
            />
            <circle
              className="progress-ring-progress"
              cx="140"
              cy="140"
              r="120"
              fill="none"
              stroke={getModeColor()}
              strokeWidth="12"
              strokeDasharray={`${2 * Math.PI * 120}`}
              strokeDashoffset={`${2 * Math.PI * 120 * (1 - getProgress() / 100)}`}
              strokeLinecap="round"
              transform="rotate(-90 140 140)"
            />
          </svg>

          {/* 时间显示 */}
          <div className="timer-display">
            <div className="timer-mode" style={{ color: getModeColor() }}>
              {getModeText()}
            </div>
            <div className="timer-time">
              {formatTime(pomodoro.remainingSeconds)}
            </div>
            <div className="timer-sessions">
              🍅 × {pomodoro.sessionCount}
            </div>
          </div>
        </div>

        {/* 控制按钮 */}
        <div className="control-buttons">
          {pomodoro.mode === 'idle' && (
            <>
              <button className="btn-primary" onClick={onStartStudy}>
                <span className="btn-icon">▶</span>
                开始学习
              </button>
              <button className="btn-secondary" onClick={() => onStartBreak(false)}>
                <span className="btn-icon">☕</span>
                短休息
              </button>
            </>
          )}

          {pomodoro.mode !== 'idle' && (
            <>
              {pomodoro.isPaused ? (
                <button className="btn-primary" onClick={onResume}>
                  <span className="btn-icon">▶</span>
                  继续
                </button>
              ) : (
                <button className="btn-warning" onClick={onPause}>
                  <span className="btn-icon">⏸</span>
                  暂停
                </button>
              )}
              <button className="btn-danger" onClick={onStop}>
                <span className="btn-icon">⏹</span>
                结束
              </button>
            </>
          )}
        </div>
      </div>

      {/* 快捷工具栏 */}
      <div className="quick-tools">
        <button className="tool-btn" title="截图解释">
          <span className="tool-icon">📸</span>
          <span className="tool-label">截图</span>
        </button>
        <button className="tool-btn" title="PDF 总结">
          <span className="tool-icon">📄</span>
          <span className="tool-label">总结</span>
        </button>
        <button className="tool-btn" title="生成卡片">
          <span className="tool-icon">🃏</span>
          <span className="tool-label">卡片</span>
        </button>
        <button className="tool-btn" title="学习统计">
          <span className="tool-icon">📊</span>
          <span className="tool-label">统计</span>
        </button>
      </div>
    </div>
  );
};
