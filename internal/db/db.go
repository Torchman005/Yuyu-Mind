package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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
	if err := ensureDBDir(dbPath); err != nil {
		return nil, err
	}

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
	if err := ensureSchemaExtensions(sqlDB); err != nil {
		return nil, fmt.Errorf("ensure schema extensions: %w", err)
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

func ensureDBDir(dbPath string) error {
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return nil
	}

	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database dir: %w", err)
	}
	return nil
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

func ensureSchemaExtensions(db *sql.DB) error {
	extensions := []struct{ table, column, ddl string }{
		{"messages", "source_kind", "ALTER TABLE messages ADD COLUMN source_kind TEXT"},
		{"messages", "emotion", "ALTER TABLE messages ADD COLUMN emotion TEXT"},
		{"messages", "mood", "ALTER TABLE messages ADD COLUMN mood TEXT"},
		{"messages", "energy", "ALTER TABLE messages ADD COLUMN energy REAL"},
		{"messages", "gesture", "ALTER TABLE messages ADD COLUMN gesture TEXT"},
		{"messages", "hand", "ALTER TABLE messages ADD COLUMN hand TEXT"},
	}

	for _, ext := range extensions {
		ok, err := columnExists(db, ext.table, ext.column)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := db.Exec(ext.ddl); err != nil {
				return fmt.Errorf("add %s.%s: %w", ext.table, ext.column, err)
			}
		}
	}
	return nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
