package persistence_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
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
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attacher := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	return persistence.NewPublishedTableRepoSQL(db), persistence.NewSourceTableRepoSQL(db, attacher, nil)
}

func seedSourceTable(t *testing.T, src *persistence.SourceTableRepoSQL, id string) {
	t.Helper()
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
}

func TestPublishedTableRepoSQL_CreateAndGet_RoundTripsLevelsAndMappings(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")
	seedSourceTable(t, src, "src-B")

	in := domain.PublishedTable{
		ID: "pub-1", Slug: "lv5", DisplayName: "Mixed Lv5", Symbol: "★",
		OwnedOnly: true,
		Pick:      domain.PickConfig{RefreshMode: domain.RefreshModeDaily},
		SortOrder: 0,
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "5", SortOrder: 0,
				PerMappingPick: 2, TotalPick: 5,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "map-1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "5", SortOrder: 0},
					{ID: "map-2", PublishedTableLevelID: "lvl-1", SourceTableID: "src-B", SourceLevel: "5", SortOrder: 1},
				},
			},
			{
				ID: "lvl-2", PublishedTableID: "pub-1", Name: "5-6", SortOrder: 1,
				PerMappingPick: 1, TotalPick: 4,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "map-3", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "5", SortOrder: 0},
					{ID: "map-4", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "6", SortOrder: 1},
				},
			},
		},
	}

	id, err := pub.Create(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "pub-1", id)

	got, err := pub.Get(context.Background(), "pub-1")
	require.NoError(t, err)
	require.Equal(t, in.Slug, got.Slug)
	require.Equal(t, in.DisplayName, got.DisplayName)
	require.Equal(t, in.Symbol, got.Symbol)
	require.Equal(t, in.OwnedOnly, got.OwnedOnly)
	require.Equal(t, in.Pick.RefreshMode, got.Pick.RefreshMode)
	require.Len(t, got.Levels, 2)
	require.Equal(t, "5", got.Levels[0].Name)
	require.Equal(t, 2, got.Levels[0].PerMappingPick)
	require.Equal(t, 5, got.Levels[0].TotalPick)
	require.Len(t, got.Levels[0].Mappings, 2)
	require.Equal(t, "src-A", got.Levels[0].Mappings[0].SourceTableID)
	require.Equal(t, "5", got.Levels[0].Mappings[0].SourceLevel)
	require.Equal(t, "5-6", got.Levels[1].Name)
	require.Len(t, got.Levels[1].Mappings, 2)
}

func TestPublishedTableRepoSQL_GetBySlug_ReturnsLevelsAndMappings(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	_, err := pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-1", Slug: "stella", DisplayName: "Stella",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "0",
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "m1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "0"},
				},
			},
		},
	})
	require.NoError(t, err)

	got, err := pub.GetBySlug(context.Background(), "stella")
	require.NoError(t, err)
	require.Len(t, got.Levels, 1)
	require.Len(t, got.Levels[0].Mappings, 1)
}

func TestPublishedTableRepoSQL_Update_ReplacesAllLevelsAndMappings(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	initial := domain.PublishedTable{
		ID: "pub-1", Slug: "tbl", DisplayName: "T",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "old", SortOrder: 0,
				PerMappingPick: 1, TotalPick: 1,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "m1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "old"},
				},
			},
		},
	}
	_, err := pub.Create(context.Background(), initial)
	require.NoError(t, err)

	updated := initial
	updated.DisplayName = "T2"
	updated.Levels = []domain.PublishedTableLevel{
		{
			ID: "lvl-2", PublishedTableID: "pub-1", Name: "new", SortOrder: 0,
			PerMappingPick: 3, TotalPick: 7,
			Mappings: []domain.PublishedTableLevelMapping{
				{ID: "m2", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "new"},
			},
		},
	}
	require.NoError(t, pub.Update(context.Background(), updated))

	got, err := pub.Get(context.Background(), "pub-1")
	require.NoError(t, err)
	require.Equal(t, "T2", got.DisplayName)
	require.Len(t, got.Levels, 1)
	require.Equal(t, "new", got.Levels[0].Name)
	require.Equal(t, "lvl-2", got.Levels[0].ID)
	require.Equal(t, 3, got.Levels[0].PerMappingPick)
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "m2", got.Levels[0].Mappings[0].ID)
}

func TestPublishedTableRepoSQL_List_DoesNotEagerLoadLevelsForListView(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	_, err := pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-1", Slug: "x", DisplayName: "X",
		Pick:   domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{{ID: "lvl-1", PublishedTableID: "pub-1", Name: "5"}},
	})
	require.NoError(t, err)

	list, err := pub.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "X", list[0].DisplayName)
	require.Empty(t, list[0].Levels)
}

func TestPublishedTableRepoSQL_Delete_CascadesToLevelsAndMappings(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	_, err := pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-1", Slug: "x", DisplayName: "X",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "5",
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "m1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "5"},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, pub.Delete(context.Background(), "pub-1"))

	got, err := pub.Get(context.Background(), "pub-1")
	require.True(t, errors.Is(err, usecase.ErrPublishedTableNotFound))
	_ = got
}

func TestPublishedTableRepoSQL_Create_DuplicateSlug_ReturnsErrSlugDuplicated(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	_, err := pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-1", Slug: "same", DisplayName: "A",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.NoError(t, err)

	_, err = pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-2", Slug: "same", DisplayName: "B",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableRepoSQL_SlugExists(t *testing.T) {
	pub, src := setupPublishedTableRepo(t)
	seedSourceTable(t, src, "src-A")

	_, err := pub.Create(context.Background(), domain.PublishedTable{
		ID: "pub-1", Slug: "stella", DisplayName: "S",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.NoError(t, err)

	exists, err := pub.SlugExists(context.Background(), "stella", "")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = pub.SlugExists(context.Background(), "stella", "pub-1")
	require.NoError(t, err)
	require.False(t, exists)

	exists, err = pub.SlugExists(context.Background(), "other", "")
	require.NoError(t, err)
	require.False(t, exists)
}
