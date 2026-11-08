import React from 'react';
import { LineChart, Line, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import './StudyStats.css';

export interface DailyData {
  date: string;
  minutes: number;
}

interface StudyStatsProps {
  dailyData: DailyData[];
  todayMinutes: number;
  weekMinutes: number;
  monthMinutes: number;
  totalSessions: number;
  currentStreak: number;
  onClose: () => void;
}

export const StudyStats: React.FC<StudyStatsProps> = ({
  dailyData,
  todayMinutes,
  weekMinutes,
  monthMinutes,
  totalSessions,
  currentStreak,
  onClose,
}) => {
  const formatDate = (dateStr: string): string => {
    const date = new Date(dateStr);
    return `${date.getMonth() + 1}/${date.getDate()}`;
  };

  const chartData = dailyData.map(d => ({
    date: formatDate(d.date),
    hours: Number((d.minutes / 60).toFixed(1)),
  }));

  return (
    <div className="stats-overlay">
      <div className="stats-panel">
        <div className="stats-header">
          <h2>📊 学习统计</h2>
          <button className="close-btn" onClick={onClose}>✕</button>
        </div>

        <div className="stats-content">
          {/* 总览卡片 */}
          <div className="overview-cards">
            <div className="overview-card">
              <div className="overview-icon">📅</div>
              <div className="overview-text">
                <div className="overview-value">{todayMinutes}</div>
                <div className="overview-label">今日（分钟）</div>
              </div>
            </div>

            <div className="overview-card">
              <div className="overview-icon">📆</div>
              <div className="overview-text">
                <div className="overview-value">{weekMinutes}</div>
                <div className="overview-label">本周（分钟）</div>
              </div>
            </div>

            <div className="overview-card">
              <div className="overview-icon">📈</div>
              <div className="overview-text">
                <div className="overview-value">{monthMinutes}</div>
                <div className="overview-label">本月（分钟）</div>
              </div>
            </div>

            <div className="overview-card">
              <div className="overview-icon">🍅</div>
              <div className="overview-text">
                <div className="overview-value">{totalSessions}</div>
                <div className="overview-label">总会话数</div>
              </div>
            </div>

            <div className="overview-card highlight">
              <div className="overview-icon">🔥</div>
              <div className="overview-text">
                <div className="overview-value">{currentStreak}</div>
                <div className="overview-label">连续天数</div>
              </div>
            </div>
          </div>

          {/* 趋势图表 */}
          <div className="chart-section">
            <h3>📉 学习时长趋势</h3>
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#E0E0E0" />
                <XAxis dataKey="date" stroke="#666" />
                <YAxis stroke="#666" label={{ value: '小时', angle: -90, position: 'insideLeft' }} />
                <Tooltip
                  contentStyle={{ backgroundColor: '#fff', border: '1px solid #E0E0E0', borderRadius: '8px' }}
                  labelStyle={{ fontWeight: 600 }}
                />
                <Line
                  type="monotone"
                  dataKey="hours"
                  stroke="#4CAF50"
                  strokeWidth={3}
                  dot={{ fill: '#4CAF50', r: 5 }}
                  activeDot={{ r: 7 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div className="chart-section">
            <h3>📊 每日学习时长</h3>
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#E0E0E0" />
                <XAxis dataKey="date" stroke="#666" />
                <YAxis stroke="#666" label={{ value: '小时', angle: -90, position: 'insideLeft' }} />
                <Tooltip
                  contentStyle={{ backgroundColor: '#fff', border: '1px solid #E0E0E0', borderRadius: '8px' }}
                  labelStyle={{ fontWeight: 600 }}
                />
                <Bar dataKey="hours" fill="#667eea" radius={[8, 8, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>

          {/* 成就徽章 */}
          <div className="achievements-section">
            <h3>🏆 成就徽章</h3>
            <div className="achievements-grid">
              <div className={`achievement ${currentStreak >= 7 ? 'unlocked' : 'locked'}`}>
                <div className="achievement-icon">🔥</div>
                <div className="achievement-name">连续 7 天</div>
              </div>
              <div className={`achievement ${totalSessions >= 10 ? 'unlocked' : 'locked'}`}>
                <div className="achievement-icon">🍅</div>
                <div className="achievement-name">10 个番茄钟</div>
              </div>
              <div className={`achievement ${weekMinutes >= 600 ? 'unlocked' : 'locked'}`}>
                <div className="achievement-icon">⭐</div>
                <div className="achievement-name">周学习 10 小时</div>
              </div>
              <div className={`achievement ${monthMinutes >= 2400 ? 'unlocked' : 'locked'}`}>
                <div className="achievement-icon">💎</div>
                <div className="achievement-name">月学习 40 小时</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
