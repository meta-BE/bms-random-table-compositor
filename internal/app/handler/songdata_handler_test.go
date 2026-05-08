package handler_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupHandlerDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	return db
}

func TestSongdataHandler_GetStatus_NotAttached(t *testing.T) {
	db := setupHandlerDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	configUC := usecase.NewConfigUseCase(persistence.NewConfigStoreSQL(db))
	// PickUseCase はテスト用に nil 引数で作成 (status のみ呼ぶので問題ない)
	pickUC := usecase.NewPickUseCase(nil, nil, usecase.NewPickResultStore(), clock.System{}, nil, logger)

	h := handler.NewSongdataHandler(a, configUC, pickUC)
	h.SetContext(context.Background())

	st := h.GetSongdataAttachStatus()
	require.False(t, st.Attached)
	require.Equal(t, 0, st.SongCount)
	require.Empty(t, st.AttachedAt)
}
