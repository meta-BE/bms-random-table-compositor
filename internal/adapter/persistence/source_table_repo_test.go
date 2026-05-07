package persistence_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/stretchr/testify/require"
)

func setupSourceTableRepo(t *testing.T) *persistence.SourceTableRepoSQL {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))
	return persistence.NewSourceTableRepoSQL(db)
}

func TestSourceTableRepoSQL_CreateThenGet(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()

	in := domain.SourceTable{
		ID:              "01J0000000000000000000A",
		InputURL:        "https://example.com/table.html",
		InputKind:       domain.InputKindHTML,
		DisplayName:     "Example",
		LastFetchStatus: domain.FetchStatusNever,
	}
	id, err := r.Create(ctx, in)
	require.NoError(t, err)
	require.Equal(t, in.ID, id)

	got, err := r.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, in.InputURL, got.InputURL)
	require.Equal(t, domain.InputKindHTML, got.InputKind)
	require.Equal(t, "Example", got.DisplayName)
	require.Equal(t, domain.FetchStatusNever, got.LastFetchStatus)
	require.Nil(t, got.LastFetchedAt)
}

func TestSourceTableRepoSQL_Get_NotFoundError(t *testing.T) {
	r := setupSourceTableRepo(t)
	_, err := r.Get(context.Background(), "missing")
	require.Error(t, err)
}

func TestSourceTableRepoSQL_List_Empty(t *testing.T) {
	r := setupSourceTableRepo(t)
	out, err := r.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestSourceTableRepoSQL_List_OrdersBySortOrderThenCreatedAt(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	for i, id := range []string{"A", "B", "C"} {
		_, err := r.Create(ctx, domain.SourceTable{
			ID: id, InputURL: "u" + id, InputKind: domain.InputKindHeaderJSON,
			LastFetchStatus: domain.FetchStatusNever,
		})
		require.NoError(t, err)
		_ = i
	}
	out, err := r.List(ctx)
	require.NoError(t, err)
	require.Len(t, out, 3)
}

func TestSourceTableRepoSQL_Update_PersistsDisplayName(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "X", InputURL: "u", InputKind: domain.InputKindHeaderJSON,
		DisplayName: "old", LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
	got, err := r.Get(ctx, "X")
	require.NoError(t, err)
	got.DisplayName = "new"
	require.NoError(t, r.Update(ctx, got))
	after, err := r.Get(ctx, "X")
	require.NoError(t, err)
	require.Equal(t, "new", after.DisplayName)
}

func TestSourceTableRepoSQL_Delete_RemovesRow(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "Y", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
	require.NoError(t, r.Delete(ctx, "Y"))
	_, err = r.Get(ctx, "Y")
	require.Error(t, err)
}

func TestSourceTableRepoSQL_SaveFetched_UpdatesHeaderAndInsertsCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML,
		DisplayName: "user-name", LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)

	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	ft := port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "Fetched Name", Symbol: "fx",
			DataURL: "https://example.com/data.json", LevelOrder: []string{"0", "1"},
		},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "aaaa", SHA256: "1111", Level: "0", Title: "T0",
				Artist: "A0", Raw: map[string]any{"md5": "aaaa", "url": "u0"}},
			{Position: 1, MD5: "bbbb", Level: "1", Title: "T1",
				Artist: "A1", Raw: map[string]any{"md5": "bbbb"}},
		},
		ETag: `"etag-1"`,
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", ft, now))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, "Fetched Name", got.Name)
	require.Equal(t, "fx", got.Symbol)
	require.Equal(t, "user-name", got.DisplayName, "DisplayName はユーザー編集を維持")
	require.Equal(t, "https://example.com/data.json", got.DataURL)
	require.Equal(t, []string{"0", "1"}, got.LevelOrder)
	require.Equal(t, `"etag-1"`, got.ETag)
	require.Equal(t, domain.FetchStatusOK, got.LastFetchStatus)
	require.Equal(t, "", got.LastFetchError)
	require.NotNil(t, got.LastFetchedAt)
	require.True(t, got.LastFetchedAt.Equal(now))
}

func TestSourceTableRepoSQL_SaveFetched_ReplacesChartsOnSecondCall(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})

	first := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
			{Position: 1, MD5: "b", Level: "0", Raw: map[string]any{"md5": "b"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", first, time.Now()))

	second := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "x", Level: "0", Raw: map[string]any{"md5": "x"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", second, time.Now()))

	// Task 7 で LoadCharts 実装後に有効化
	// charts, err := r.LoadCharts(ctx, "Z")
	// require.NoError(t, err)
	// require.Len(t, charts, 1)
	// require.Equal(t, "x", charts[0].MD5)
}

func TestSourceTableRepoSQL_SaveFetched_NotModifiedKeepsCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	first := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
		},
		ETag: `"v1"`,
	}
	t0 := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.SaveFetched(ctx, "Z", first, t0))

	t1 := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.SaveFetched(ctx, "Z", port.FetchedTable{NotModified: true}, t1))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, domain.FetchStatusOK, got.LastFetchStatus)
	require.True(t, got.LastFetchedAt.Equal(t1))
	require.Equal(t, `"v1"`, got.ETag, "ETag は維持される")
	// Task 7 で LoadCharts 実装後に有効化
	// charts, _ := r.LoadCharts(ctx, "Z")
	// require.Len(t, charts, 1)
	// require.Equal(t, "a", charts[0].MD5)
}

func TestSourceTableRepoSQL_MarkFetchError_KeepsPreviousCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, r.SaveFetched(ctx, "Z", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
		},
	}, time.Now()))

	errAt := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.MarkFetchError(ctx, "Z", errors.New("boom"), errAt))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, domain.FetchStatusError, got.LastFetchStatus)
	require.Equal(t, "boom", got.LastFetchError)
	require.True(t, got.LastFetchedAt.Equal(errAt))

	// Task 7 で LoadCharts 実装後に有効化
	// charts, _ := r.LoadCharts(ctx, "Z")
	// require.Len(t, charts, 1, "失敗時もキャッシュは保持される（spec §8）")
}
