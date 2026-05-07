package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakeOwnedRepo は port.OwnedChartRepo のテスト用実装。
type fakeOwnedRepo struct {
	mu       sync.Mutex
	calls    int
	resp     map[string]struct{}
	err      error
	lastPath string
}

func (r *fakeOwnedRepo) LoadOwnedMD5Set(_ context.Context, dbPath string) (map[string]struct{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastPath = dbPath
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]struct{}, len(r.resp))
	for k := range r.resp {
		out[k] = struct{}{}
	}
	return out, nil
}

// fixedClock は時刻を固定するテスト用 port.Clock。
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newOwnedCache(repo *fakeOwnedRepo, store *fakeConfigStore, clock fixedClock) *usecase.OwnedMD5Cache {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return usecase.NewOwnedMD5Cache(repo, store, clock, logger)
}

func TestOwnedMD5Cache_Get_AutoLoadsOnce(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"abc": {}, "def": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/path/to/db"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, repo.calls)

	// 2 回目は repo を再呼出ししない
	got, err = c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, repo.calls)
}

func TestOwnedMD5Cache_Reload_HitsRepoAgain(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"a": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())
	require.Equal(t, 1, repo.calls)

	// repo の戻り値を増やしてから Reload
	repo.mu.Lock()
	repo.resp = map[string]struct{}{"a": {}, "b": {}, "c": {}}
	repo.mu.Unlock()
	require.NoError(t, c.Reload(context.Background()))
	require.Equal(t, 2, repo.calls)

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3)
}

func TestOwnedMD5Cache_Reload_KeepsPreviousSetOnError(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"a": {}, "b": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())

	repo.mu.Lock()
	repo.err = errors.New("disk full")
	repo.mu.Unlock()
	err := c.Reload(context.Background())
	require.Error(t, err)

	// 前回の set は保持されている
	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)

	st := c.Status()
	require.Equal(t, 2, st.Count)
	require.Contains(t, st.LastError, "disk full")
}

func TestOwnedMD5Cache_Invalidate_TriggersReload(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"x": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())
	require.Equal(t, 1, repo.calls)

	c.Invalidate()
	_, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls)
}

func TestOwnedMD5Cache_EmptyPath_ReturnsEmptySet(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{}}
	store := newFakeConfigStore()
	// songdata_db_path 未設定
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)
	// repo は呼ばれる（dbPath="" でも repo の責務として空 set を返す）
	require.Equal(t, 1, repo.calls)
	require.Equal(t, "", repo.lastPath)
}
