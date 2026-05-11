package handler_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupPublishedTableHandler(t *testing.T) (*handler.PublishedTableHandler, *persistence.SourceTableRepoSQL) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "h.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attacher := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	src := persistence.NewSourceTableRepoSQL(db, attacher, nil)
	pub := persistence.NewPublishedTableRepoSQL(db)
	uc := usecase.NewPublishedTableUseCase(pub, src, idgen.NewULID(), logger)
	h := handler.NewPublishedTableHandler(uc)
	h.SetContext(context.Background())
	return h, src
}

func TestPublishedTableHandler_Create_List_Delete(t *testing.T) {
	h, src := setupPublishedTableHandler(t)
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: "SRC1", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LevelOrder:      []string{"1", "2"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	// Levels を明示的に渡して作成する。
	id, err := h.CreatePublishedTable(handler.CreatePublishedTableRequest{
		Slug: "ok", DisplayName: "OK", Symbol: "★",
		RefreshMode: "per_request",
		Levels: []handler.PublishedTableLevelInputDTO{
			{
				Name: "1", PerMappingPick: 0, TotalPick: 0,
				Mappings: []handler.PublishedTableLevelMappingInputDTO{
					{SourceTableID: "SRC1", SourceLevel: "1"},
				},
			},
			{
				Name: "2", PerMappingPick: 0, TotalPick: 0,
				Mappings: []handler.PublishedTableLevelMappingInputDTO{
					{SourceTableID: "SRC1", SourceLevel: "2"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	list, err := h.ListPublishedTables()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "ok", list[0].Slug)
	// List では Levels は空配列で返す（仕様）。
	require.Empty(t, list[0].Levels)

	// Get では Levels / Mappings 込みで返る。
	got, err := h.GetPublishedTable(id)
	require.NoError(t, err)
	require.Len(t, got.Levels, 2)
	require.Equal(t, "1", got.Levels[0].Name)
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "SRC1", got.Levels[0].Mappings[0].SourceTableID)

	require.NoError(t, h.DeletePublishedTable(id))
	list, err = h.ListPublishedTables()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestPublishedTableHandler_ValidateSlug(t *testing.T) {
	h, src := setupPublishedTableHandler(t)
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: "SRC1", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LevelOrder:      []string{"1"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	require.True(t, h.ValidateSlug("ok-slug", "").OK)
	require.False(t, h.ValidateSlug("BadSlug", "").OK)
	require.Equal(t, "invalid_format", h.ValidateSlug("BadSlug", "").Reason)
	require.Equal(t, "reserved", h.ValidateSlug("_admin", "").Reason)

	id, err := h.CreatePublishedTableFromSource(handler.CreateFromSourceRequest{
		SourceTableID: "SRC1",
		Slug:          "taken", DisplayName: "T", Symbol: "★",
	})
	require.NoError(t, err)

	require.Equal(t, "duplicate", h.ValidateSlug("taken", "").Reason)
	require.True(t, h.ValidateSlug("taken", id).OK, "自分自身を除外すれば OK")
}

func TestPublishedTableHandler_CreatePublishedTableFromSource(t *testing.T) {
	h, src := setupPublishedTableHandler(t)
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: "SRC1", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LevelOrder:      []string{"1", "2"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	id, err := h.CreatePublishedTableFromSource(handler.CreateFromSourceRequest{
		SourceTableID: "SRC1",
		Slug:          "stella", DisplayName: "Stella", Symbol: "★",
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := h.GetPublishedTable(id)
	require.NoError(t, err)
	require.Len(t, got.Levels, 2)
	require.Equal(t, "1", got.Levels[0].Name)
	require.Equal(t, "2", got.Levels[1].Name)
	// 各レベルがソース表の同レベルへ 1:1 マッピングされる。
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "SRC1", got.Levels[0].Mappings[0].SourceTableID)
	require.Equal(t, "1", got.Levels[0].Mappings[0].SourceLevel)
}
