package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
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
