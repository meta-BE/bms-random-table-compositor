package persistence

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/stretchr/testify/require"
)

// makeScoreDBFile は最小限の score テーブルを持つテスト用 DB を作る。
func makeScoreDBFile(t *testing.T, path string, rows [][2]any) {
	t.Helper()
	db, err := OpenDB(path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE score (sha256 TEXT NOT NULL, mode INTEGER, date INTEGER, PRIMARY KEY(sha256, mode))`)
	require.NoError(t, err)
	for _, r := range rows {
		_, err = db.Exec(`INSERT INTO score(sha256, mode, date) VALUES(?, 0, ?)`, r[0], r[1])
		require.NoError(t, err)
	}
}

func TestScoreDBAttacher_AttachAndDetach(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	scorePath := filepath.Join(dir, "score.db")
	makeScoreDBFile(t, scorePath, [][2]any{{"sha-a", 1000}, {"sha-b", 2000}})

	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.False(t, a.IsAttached())

	require.NoError(t, a.Attach(context.Background(), scorePath))
	require.True(t, a.IsAttached())

	var n int
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 2, n)

	require.NoError(t, a.Detach(context.Background()))
	require.False(t, a.IsAttached())
}

func TestScoreDBAttacher_AttachEmptyPathIsNoop(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, a.Attach(context.Background(), ""))
	require.False(t, a.IsAttached())
}

// TestScoreDBAttacher_RejectsDBWithoutScoreTable は score テーブルが無い DB
// (例: songdata.db を誤って選んだ場合) を attach 試行したとき、エラーを返し
// IsAttached=false を維持することを検証する。後続クエリの "no such table" を
// 防ぐための前段バリデーション。
func TestScoreDBAttacher_RejectsDBWithoutScoreTable(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	songdataLikePath := filepath.Join(dir, "not_score.db")

	// score テーブルが無い (song テーブルしかない) DB を作る
	notScoreDB, err := OpenDB(songdataLikePath)
	require.NoError(t, err)
	_, err = notScoreDB.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY(md5))`)
	require.NoError(t, err)
	notScoreDB.Close()

	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	err = a.Attach(context.Background(), songdataLikePath)
	require.Error(t, err, "score テーブルが無い DB は attach 失敗扱い")
	require.False(t, a.IsAttached(), "失敗時はアタッチ状態を保持しない")

	// DETACH 済みなので別の正しい DB を attach できる
	correctScorePath := filepath.Join(dir, "ok.db")
	makeScoreDBFile(t, correctScorePath, [][2]any{{"x", 1}})
	require.NoError(t, a.Attach(context.Background(), correctScorePath))
	require.True(t, a.IsAttached())
}

func TestScoreDBAttacher_ReAttach(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	score1 := filepath.Join(dir, "s1.db")
	score2 := filepath.Join(dir, "s2.db")
	makeScoreDBFile(t, score1, [][2]any{{"a", 1}})
	makeScoreDBFile(t, score2, [][2]any{{"b", 2}, {"c", 3}})

	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, a.Attach(context.Background(), score1))

	var n int
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 1, n)

	require.NoError(t, a.ReAttach(context.Background(), score2))
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 2, n)
}
