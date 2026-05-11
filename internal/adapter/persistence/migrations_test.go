package persistence

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMigrations_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db))

	for _, table := range []string{"config", "source_table", "source_table_chart", "published_table"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		require.NoError(t, err, "table %s not found", table)
		require.Equal(t, table, name)
	}
}

func TestRunMigrations_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db), "second migration should succeed")
	require.NoError(t, RunMigrations(db), "third migration should succeed")
}

func TestRunMigrations_CreatesIndexes(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	for _, idx := range []string{"idx_stc_md5", "idx_stc_source_level"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&name)
		require.NoError(t, err, "index %s not found", idx)
	}
}

func TestRunMigrations_SetsSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	var v string
	err = db.QueryRow(`SELECT value FROM config WHERE key='schema_version'`).Scan(&v)
	require.NoError(t, err)
	require.Equal(t, "2", v)
}

func TestRunMigrations_UpgradeV1ToV2_DropsOldPublishedTableAndCreatesNewTables(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	// 1. まず v1 相当のスキーマを直接作成する（旧 published_table カラム構成）
	_, err = db.Exec(`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO config(key, value) VALUES('schema_version', '1')`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE source_table (id TEXT PRIMARY KEY, input_url TEXT NOT NULL, input_kind TEXT NOT NULL CHECK(input_kind IN ('html','header_json')), display_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', symbol TEXT NOT NULL DEFAULT '', level_order_json TEXT NOT NULL DEFAULT '[]', data_url TEXT NOT NULL DEFAULT '', etag TEXT NOT NULL DEFAULT '', last_fetched_at TEXT, last_fetch_status TEXT NOT NULL DEFAULT 'never' CHECK(last_fetch_status IN ('never','ok','error')), last_fetch_error TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE published_table (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, symbol TEXT NOT NULL DEFAULT '', source_table_id TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE, owned_only INTEGER NOT NULL DEFAULT 0, pick_per_level INTEGER NOT NULL DEFAULT 0, pick_refresh_mode TEXT NOT NULL DEFAULT 'manual' CHECK(pick_refresh_mode IN ('per_request','daily','manual')), prefer_old_play INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)

	// ダミーデータ: 旧公開表 1 件
	_, err = db.Exec(`INSERT INTO source_table(id, input_url, input_kind) VALUES('s1', 'http://x', 'html')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO published_table(id, slug, display_name, source_table_id) VALUES('p1', 'old', 'Old', 's1')`)
	require.NoError(t, err)

	// 2. RunMigrations 実行
	require.NoError(t, RunMigrations(db))

	// 3. schema_version が "2" になっている
	var ver string
	require.NoError(t, db.QueryRow(`SELECT value FROM config WHERE key='schema_version'`).Scan(&ver))
	require.Equal(t, "2", ver)

	// 4. 旧 published_table データが消えている（クリーンブレイク）
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM published_table`).Scan(&n))
	require.Equal(t, 0, n)

	// 5. published_table に source_table_id / pick_per_level / prefer_old_play カラムが「ない」
	cols := tableColumns(t, db, "published_table")
	require.NotContains(t, cols, "source_table_id")
	require.NotContains(t, cols, "pick_per_level")
	require.NotContains(t, cols, "prefer_old_play")
	require.Contains(t, cols, "owned_only")
	require.Contains(t, cols, "pick_refresh_mode")

	// 6. 新テーブル published_table_level / published_table_level_mapping が存在する
	require.Contains(t, tableColumns(t, db, "published_table_level"), "per_mapping_pick")
	require.Contains(t, tableColumns(t, db, "published_table_level"), "total_pick")
	require.Contains(t, tableColumns(t, db, "published_table_level_mapping"), "source_level")
}

func TestRunMigrations_AddsWeightModeColumns(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	cols := tableColumns(t, db, "published_table")
	require.Contains(t, cols, "weight_mode")
	require.Contains(t, cols, "weight_param_x")
}

func TestRunMigrations_BackfillsWeightDefaults(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db))
	_, err = db.Exec(`INSERT INTO published_table(id, slug, display_name) VALUES('p1','s1','t1')`)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(db))
	var mode string
	var x int
	require.NoError(t, db.QueryRow(
		`SELECT weight_mode, weight_param_x FROM published_table WHERE id='p1'`,
	).Scan(&mode, &x))
	require.Equal(t, "off", mode)
	require.Equal(t, 10, x)
}

func TestRunMigrations_WeightColumnsAddedAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db))
	cols := tableColumns(t, db, "published_table")
	count := 0
	for _, c := range cols {
		if c == "weight_mode" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

// tableColumns は対象テーブルのカラム名を返す。
func tableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		out = append(out, n)
	}
	return out
}
