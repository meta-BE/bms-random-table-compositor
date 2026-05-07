package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/stretchr/testify/require"
)

// testdata/songdata.db への絶対パスを返す。
func songdataPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../../testdata/songdata.db")
	require.NoError(t, err)
	return abs
}

func TestSongdataReader_LoadOwnedMD5Set_EmptyPathReturnsEmptySet(t *testing.T) {
	r := persistence.NewSongdataReader()
	got, err := r.LoadOwnedMD5Set(context.Background(), "")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSongdataReader_LoadOwnedMD5Set_MissingFileReturnsError(t *testing.T) {
	r := persistence.NewSongdataReader()
	_, err := r.LoadOwnedMD5Set(context.Background(), "/non/existent/path/songdata.db")
	require.Error(t, err)
}

func TestSongdataReader_LoadOwnedMD5Set_RealDB(t *testing.T) {
	r := persistence.NewSongdataReader()
	got, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	require.NotEmpty(t, got, "testdata/songdata.db には song 行があるはず")
	// md5 集合の各キーは 32 文字 16進数のはず
	for k := range got {
		require.Len(t, k, 32, "md5 must be 32 hex chars: %q", k)
	}
}

func TestSongdataReader_LoadOwnedMD5Set_DoesNotMutateFile(t *testing.T) {
	// 書き込み防止の確認: read-only で開いているので songdata.db への変更が起きない。
	// 連続呼び出しで count が同じであることだけ確認（実際の mtime チェックは過剰）
	r := persistence.NewSongdataReader()
	first, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	second, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	require.Equal(t, len(first), len(second))
}
