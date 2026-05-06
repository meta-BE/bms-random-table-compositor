package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ConfigStoreSQL は config テーブルを使った port.ConfigStore の実装。
type ConfigStoreSQL struct {
	db *sql.DB
}

// NewConfigStoreSQL は新しい ConfigStoreSQL を作る。
func NewConfigStoreSQL(db *sql.DB) *ConfigStoreSQL {
	return &ConfigStoreSQL{db: db}
}

// Get は指定キーの値を返す。
func (s *ConfigStoreSQL) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("config get %q: %w", key, err)
	}
	return v, true, nil
}

// Set は指定キーに値を保存する。既存キーは上書き。
func (s *ConfigStoreSQL) Set(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("config set %q: %w", key, err)
	}
	return nil
}
