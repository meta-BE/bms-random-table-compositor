package persistence

import (
	"database/sql"
	"fmt"
)

// schemaVersion は現在のスキーマバージョン。スキーマ変更時にインクリメント。
const schemaVersion = "2"

// RunMigrations は compositor.db のスキーマを冪等に作成する。
// schema_version=1 から 2 へ上げるときは旧 published_table を DROP/CREATE する
// （複数ソース表合成スペックのクリーンブレイク方針）。
func RunMigrations(db *sql.DB) error {
	// config テーブルだけは先に確保
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("migration exec: %w", err)
	}

	// 現在の schema_version を取得（初回起動時は空）
	var current string
	if err := db.QueryRow(
		`SELECT value FROM config WHERE key='schema_version'`,
	).Scan(&current); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read schema_version: %w", err)
	}

	// source_table 系は v1 と同じ（変更なし）
	v1Statements := []string{
		`CREATE TABLE IF NOT EXISTS source_table (
			id                TEXT PRIMARY KEY,
			input_url         TEXT NOT NULL,
			input_kind        TEXT NOT NULL CHECK(input_kind IN ('html', 'header_json')),
			display_name      TEXT NOT NULL DEFAULT '',
			name              TEXT NOT NULL DEFAULT '',
			symbol            TEXT NOT NULL DEFAULT '',
			level_order_json  TEXT NOT NULL DEFAULT '[]',
			data_url          TEXT NOT NULL DEFAULT '',
			etag              TEXT NOT NULL DEFAULT '',
			last_fetched_at   TEXT,
			last_fetch_status TEXT NOT NULL DEFAULT 'never' CHECK(last_fetch_status IN ('never','ok','error')),
			last_fetch_error  TEXT NOT NULL DEFAULT '',
			sort_order        INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS source_table_chart (
			source_id  TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			position   INTEGER NOT NULL,
			md5        TEXT NOT NULL,
			sha256     TEXT NOT NULL DEFAULT '',
			level      TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			artist     TEXT NOT NULL DEFAULT '',
			raw_json   TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (source_id, position)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_stc_md5 ON source_table_chart(md5)`,
		`CREATE INDEX IF NOT EXISTS idx_stc_source_level ON source_table_chart(source_id, level)`,
	}
	for _, s := range v1Statements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// v1 -> v2 遷移: 旧 published_table を破棄
	if current == "1" {
		if _, err := db.Exec(`DROP TABLE IF EXISTS published_table`); err != nil {
			return fmt.Errorf("drop published_table: %w", err)
		}
	}

	// v2 スキーマ（初回起動も v1->v2 もここを通る）
	v2Statements := []string{
		`CREATE TABLE IF NOT EXISTS published_table (
			id                TEXT PRIMARY KEY,
			slug              TEXT NOT NULL UNIQUE,
			display_name      TEXT NOT NULL,
			symbol            TEXT NOT NULL DEFAULT '',
			owned_only        INTEGER NOT NULL DEFAULT 0,
			pick_refresh_mode TEXT NOT NULL DEFAULT 'manual'
			                  CHECK(pick_refresh_mode IN ('per_request','daily','manual')),
			sort_order        INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS published_table_level (
			id                  TEXT PRIMARY KEY,
			published_table_id  TEXT NOT NULL REFERENCES published_table(id) ON DELETE CASCADE,
			name                TEXT NOT NULL,
			sort_order          INTEGER NOT NULL DEFAULT 0,
			per_mapping_pick    INTEGER NOT NULL DEFAULT 0,
			total_pick          INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ptl_table ON published_table_level(published_table_id, sort_order)`,
		`CREATE TABLE IF NOT EXISTS published_table_level_mapping (
			id                         TEXT PRIMARY KEY,
			published_table_level_id   TEXT NOT NULL REFERENCES published_table_level(id) ON DELETE CASCADE,
			source_table_id            TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			source_level               TEXT NOT NULL,
			sort_order                 INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ptlm_level ON published_table_level_mapping(published_table_level_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_ptlm_source ON published_table_level_mapping(source_table_id)`,
	}
	for _, s := range v2Statements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// schema_version を書き込む
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO config(key, value) VALUES('schema_version', ?)`,
		schemaVersion,
	); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}
