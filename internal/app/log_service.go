package app

import (
	"log/slog"
	"strings"

	"github.com/yuyu-mind/backend/internal/loghub"
)

// parseLogLevel 把配置里的字符串等级解析为 slog.Level；非法值按 DEBUG 处理。
func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "INFO", "INFORMATION":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default: // DEBUG 与空串
		return slog.LevelDebug
	}
}

// GetLogLevel 返回当前日志采集等级（DEBUG|INFO|WARN|ERROR）。
func (a *App) GetLogLevel() (string, error) {
	if a.logHub == nil {
		return "DEBUG", nil
	}
	return a.logHub.Level().String(), nil
}

// SetLogLevel 运行时切换日志等级（DEBUG|INFO|WARN|ERROR）并写回配置文件。
func (a *App) SetLogLevel(level string) error {
	normalized := strings.ToUpper(strings.TrimSpace(level))
	if a.logHub != nil {
		a.logHub.SetLevel(parseLogLevel(normalized))
	}
	if a.cfg != nil {
		return a.cfg.SetLogLevel(normalized)
	}
	return nil
}

// GetLogs 返回最近的日志条目（新→旧）。level 为空时用当前采集等级；count <=0 时取默认 500 条。
func (a *App) GetLogs(level string, count int) ([]loghub.Entry, error) {
	if a.logHub == nil {
		return nil, nil
	}
	lvl := a.logHub.Level()
	if strings.TrimSpace(level) != "" {
		lvl = parseLogLevel(level)
	}
	if count <= 0 {
		count = 500
	}
	return a.logHub.Snapshot(lvl, count), nil
}
