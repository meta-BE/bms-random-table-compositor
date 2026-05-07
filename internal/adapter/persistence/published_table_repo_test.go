package persistence_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupPublishedTableRepo(t *testing.T) (*persistence.PublishedTableRepoSQL, *persistence.SourceTableRepoSQL) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))
	return persistence.NewPublishedTableRepoSQL(db), persistence.NewSourceTableRepoSQL(db)
}

func seedSourceTable(t *testing.T, src *persistence.SourceTableRepoSQL, id string) {
	t.Helper()
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
}

func TestPublishedTableRepoSQL_CreateThenGet(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000A")

	in := domain.PublishedTable{
		ID: "01J0PUB0000000000000000A", Slug: "satellite-mix",
		DisplayName: "Satellite Mix", Symbol: "sl",
		SourceTableID: "01J0SOURCE000000000000A",
		OwnedOnly:     true,
		Pick:          domain.PickConfig{PerLevel: 5, RefreshMode: domain.RefreshModeDaily},
	}
	id, err := repo.Create(ctx, in)
	require.NoError(t, err)
	require.Equal(t, in.ID, id)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, in.Slug, got.Slug)
	require.Equal(t, in.DisplayName, got.DisplayName)
	require.Equal(t, in.SourceTableID, got.SourceTableID)
	require.True(t, got.OwnedOnly)
	require.Equal(t, 5, got.Pick.PerLevel)
	require.Equal(t, domain.RefreshModeDaily, got.Pick.RefreshMode)
}

func TestPublishedTableRepoSQL_GetBySlug(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000B")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000B", Slug: "lookup-me",
		DisplayName: "Lookup", SourceTableID: "01J0SOURCE000000000000B",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	got, err := repo.GetBySlug(ctx, "lookup-me")
	require.NoError(t, err)
	require.Equal(t, "01J0PUB0000000000000000B", got.ID)

	_, err = repo.GetBySlug(ctx, "no-such-slug")
	require.ErrorIs(t, err, usecase.ErrPublishedTableNotFound)
}

func TestPublishedTableRepoSQL_SlugExists(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000C")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000C", Slug: "taken",
		DisplayName: "T", SourceTableID: "01J0SOURCE000000000000C",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.NoError(t, err)

	exists, err := repo.SlugExists(ctx, "taken", "")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = repo.SlugExists(ctx, "free", "")
	require.NoError(t, err)
	require.False(t, exists)

	// 自分自身は除外
	exists, err = repo.SlugExists(ctx, "taken", "01J0PUB0000000000000000C")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestPublishedTableRepoSQL_Create_DuplicateSlugError(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000D")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB000000000000000D1", Slug: "dup",
		DisplayName: "A", SourceTableID: "01J0SOURCE000000000000D",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB000000000000000D2", Slug: "dup",
		DisplayName: "B", SourceTableID: "01J0SOURCE000000000000D",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableRepoSQL_Update_RoundTrip(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000E")
	id, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000E", Slug: "before",
		DisplayName: "Before", SourceTableID: "01J0SOURCE000000000000E",
		OwnedOnly: false,
		Pick:      domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	got.Slug = "after"
	got.DisplayName = "After"
	got.OwnedOnly = true
	got.Pick.PerLevel = 3
	got.Pick.RefreshMode = domain.RefreshModeDaily
	require.NoError(t, repo.Update(ctx, got))

	again, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "after", again.Slug)
	require.Equal(t, "After", again.DisplayName)
	require.True(t, again.OwnedOnly)
	require.Equal(t, 3, again.Pick.PerLevel)
	require.Equal(t, domain.RefreshModeDaily, again.Pick.RefreshMode)
}

func TestPublishedTableRepoSQL_Delete_Idempotent(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000F")
	id, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000F", Slug: "to-delete",
		DisplayName: "X", SourceTableID: "01J0SOURCE000000000000F",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, id))
	require.NoError(t, repo.Delete(ctx, id)) // 二度目もエラーにならない

	_, err = repo.Get(ctx, id)
	require.ErrorIs(t, err, usecase.ErrPublishedTableNotFound)
}

func TestPublishedTableRepoSQL_List_OrdersBySortOrderThenCreatedAt(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE0000000000010")

	for i, slug := range []string{"a-second", "b-first", "c-third"} {
		so := 0
		switch slug {
		case "b-first":
			so = -1
		case "c-third":
			so = 1
		}
		_, err := repo.Create(ctx, domain.PublishedTable{
			ID:            string(rune('A'+i)) + "01J0PUB0000000000000010",
			Slug:          slug,
			DisplayName:   slug,
			SourceTableID: "01J0SOURCE0000000000010",
			SortOrder:     so,
			Pick:          domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
		})
		require.NoError(t, err)
	}

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, "b-first", list[0].Slug)
	require.Equal(t, "a-second", list[1].Slug)
	require.Equal(t, "c-third", list[2].Slug)
}
