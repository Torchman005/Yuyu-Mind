package db

import (
	"context"
	"database/sql"
	"fmt"
)

// SettingsRepo 负责键值配置存储。
type SettingsRepo struct {
	db *sql.DB
}

// NewSettingsRepo 创建配置仓储。
func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

// Get 返回指定配置项的值；不存在时返回空字符串和 nil。
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key,
	).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, nil
}

// Set 插入或更新配置项。
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
