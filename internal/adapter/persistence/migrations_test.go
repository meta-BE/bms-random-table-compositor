package persistence

import (
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
	require.Equal(t, "1", v)
}
