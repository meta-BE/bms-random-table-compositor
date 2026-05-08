package persistence_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/stretchr/testify/require"
)

func newAttacherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "main.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	return db
}

func newAttacher(t *testing.T, db *sql.DB) *persistence.SongdataAttacher {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return persistence.NewSongdataAttacher(db, clock.System{}, logger)
}

func TestSongdataAttacher_Attach_EmptyPathIsNoop(t *testing.T) {
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	err := a.Attach(context.Background(), "")
	require.NoError(t, err)

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.Empty(t, st.Path)
	require.Empty(t, st.LastError)
}

func TestSongdataAttacher_Attach_NonexistentPathFails(t *testing.T) {
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	err := a.Attach(context.Background(), "/non/existent/path/songdata.db")
	require.Error(t, err)

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.NotEmpty(t, st.LastError)
}

// songdataPathOrSkip は testdata/songdata.db のパスを返す。
// ファイルが無ければ t.Skip でスキップ (CLAUDE.md: testdata は .gitignore 対象)。
func songdataPathOrSkip(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "songdata.db")
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("testdata/songdata.db が無いためスキップ: %v", err)
	}
	return abs
}

func TestSongdataAttacher_Attach_RealDB(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))

	require.True(t, a.IsAttached())
	st := a.Status()
	require.True(t, st.Attached)
	require.Equal(t, songdataPath, st.Path)
	require.Greater(t, st.SongCount, 0)
	require.NotNil(t, st.AttachedAt)
	require.Empty(t, st.LastError)
}

func TestSongdataAttacher_DetachThenStatus(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))
	require.NoError(t, a.Detach(context.Background()))

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.Empty(t, st.Path)
	require.Equal(t, 0, st.SongCount)
	require.Nil(t, st.AttachedAt)
}

func TestSongdataAttacher_ReAttach(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))
	first := a.Status().SongCount

	// 同じパスで再 ATTACH しても問題なく成功する
	require.NoError(t, a.ReAttach(context.Background(), songdataPath))
	require.True(t, a.IsAttached())
	require.Equal(t, first, a.Status().SongCount)
}

func TestSongdataAttacher_Attach_CreatesMD5Index(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)
	require.NoError(t, a.Attach(context.Background(), songdataPath))

	// アタッチ済み sd 上に複合インデックスが存在することを確認
	var name string
	err := db.QueryRowContext(context.Background(),
		"SELECT name FROM sd.sqlite_master WHERE type='index' AND name='idx_song_md5_sha256'",
	).Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "idx_song_md5_sha256", name)
}

// TestSongdataAttacher_Attach_CreatesIndexOnFreshDB は ensurePerformanceIndex の
// 新規作成パス (RW 短命接続で CREATE INDEX を実行する経路) をカバーする。
// testdata/songdata.db は既存テスト実行で索引が作られた状態のため
// IF NOT EXISTS no-op パスしか踏まれない。本テストは fresh な songdata.db を
// 作って実際に CREATE INDEX が走ることを保証する。
func TestSongdataAttacher_Attach_CreatesIndexOnFreshDB(t *testing.T) {
	// 最小スキーマの songdata.db を作成 (idx_song_md5_sha256 なし)
	dir := t.TempDir()
	songdataPath := filepath.Join(dir, "songdata.db")
	src, err := sql.Open("sqlite", songdataPath)
	require.NoError(t, err)
	_, err = src.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL DEFAULT '', title TEXT, PRIMARY KEY(md5))`)
	require.NoError(t, err)
	require.NoError(t, src.Close())

	// fresh な songdata.db に index がまだ無いことを確認
	verify, err := sql.Open("sqlite", songdataPath)
	require.NoError(t, err)
	var indexCount int
	require.NoError(t, verify.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_song_md5_sha256'",
	).Scan(&indexCount))
	require.Equal(t, 0, indexCount, "fresh DB のはずなのに index が存在する")
	require.NoError(t, verify.Close())

	// Attach (内部で ensurePerformanceIndex が走り CREATE INDEX が実行されるはず)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)
	require.NoError(t, a.Attach(context.Background(), songdataPath))

	// アタッチ済 sd 経由で index が見えることを確認
	var name string
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT name FROM sd.sqlite_master WHERE type='index' AND name='idx_song_md5_sha256'",
	).Scan(&name))
	require.Equal(t, "idx_song_md5_sha256", name)
}
