// Package persistence は SQLite を用いた永続化層の実装を提供する。
package persistence

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDB は指定パスの SQLite ファイルを開き、外部キー制約を有効化した *sql.DB を返す。
// ファイルが存在しなければ新規作成される。
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", path, err)
	}

	// SQLite の外部キー制約はデフォルトOFF。明示的に有効化する。
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}

	return db, nil
}
