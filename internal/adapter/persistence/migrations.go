package persistence

import (
	"database/sql"
	"fmt"
)

// schemaVersion は現在のスキーマバージョン。スキーマ変更時にインクリメント。
const schemaVersion = "1"

// RunMigrations は compositor.db のスキーマを冪等に作成する。
// CREATE IF NOT EXISTS と pragma_table_info チェックを使い、
// 既存DBが壊れないようにする。
func RunMigrations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS config (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
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
		`CREATE TABLE IF NOT EXISTS published_table (
			id                 TEXT PRIMARY KEY,
			slug               TEXT NOT NULL UNIQUE,
			display_name       TEXT NOT NULL,
			symbol             TEXT NOT NULL DEFAULT '',
			source_table_id    TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			owned_only         INTEGER NOT NULL DEFAULT 0,
			pick_per_level     INTEGER NOT NULL DEFAULT 0,
			pick_refresh_mode  TEXT NOT NULL DEFAULT 'manual'
			                   CHECK(pick_refresh_mode IN ('per_request', 'daily', 'manual')),
			prefer_old_play    INTEGER NOT NULL DEFAULT 0,
			sort_order         INTEGER NOT NULL DEFAULT 0,
			created_at         TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// schema_version を config に書き込む（INSERT OR REPLACE で冪等）
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO config(key, value) VALUES('schema_version', ?)`,
		schemaVersion,
	); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}
