package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB 封装 SQLite 连接，并集中暴露各类仓储。
type DB struct {
	sqlDB *sql.DB

	Conversations *ConversationRepo
	Messages      *MessageRepo
	Settings      *SettingsRepo
	TokenUsage    *TokenUsageRepo
	AgentTasks    *AgentTaskRepo
	AgentEvents   *AgentEventRepo
	Memories      *MemoryRepo
}

// New 打开 SQLite 数据库、执行迁移，并初始化所有仓储。
func New(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// 这些 PRAGMA 适合桌面端读写混合场景。
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000", // 64MB 缓存
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	// 启动时执行内嵌迁移，保证用户本地数据库自动升级。
	if err := runMigrations(sqlDB); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	slog.Info("database opened", "path", dbPath)

	return &DB{
		sqlDB:         sqlDB,
		Conversations: NewConversationRepo(sqlDB),
		Messages:      NewMessageRepo(sqlDB),
		Settings:      NewSettingsRepo(sqlDB),
		TokenUsage:    NewTokenUsageRepo(sqlDB),
		AgentTasks:    NewAgentTaskRepo(sqlDB),
		AgentEvents:   NewAgentEventRepo(sqlDB),
		Memories:      NewMemoryRepo(sqlDB),
	}, nil
}

// Close 关闭数据库连接。
func (d *DB) Close() error {
	if d.sqlDB != nil {
		return d.sqlDB.Close()
	}
	return nil
}

// SQL 返回底层 *sql.DB，供少数高级场景使用。
func (d *DB) SQL() *sql.DB {
	return d.sqlDB
}

// runMigrations 读取并执行内嵌的 SQL 迁移文件。
func runMigrations(db *sql.DB) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		// 只执行 Up 段，Down 段保留给未来迁移工具使用。
		upSQL := extractUpSection(string(data))
		if upSQL == "" {
			continue
		}

		slog.Debug("running migration", "file", entry.Name())
		if _, err := db.ExecContext(context.Background(), upSQL); err != nil {
			return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// extractUpSection 提取 "-- +migrate Up" 和 "-- +migrate Down" 之间的 SQL。
func extractUpSection(content string) string {
	const upMarker = "-- +migrate Up"
	const downMarker = "-- +migrate Down"

	upIdx := indexOf(content, upMarker)
	if upIdx == -1 {
		return content // 没有迁移标记时，默认执行整个文件。
	}
	start := upIdx + len(upMarker)

	downIdx := indexOf(content[upIdx:], downMarker)
	if downIdx == -1 {
		return content[start:]
	}
	return content[start : upIdx+downIdx]
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
