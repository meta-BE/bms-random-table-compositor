package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type stubClock struct{ t time.Time }

func (c stubClock) Now() time.Time { return c.t }

type stubIDGen struct{ seq int }

func (g *stubIDGen) New() string {
	g.seq++
	return "01J0PUB" + string(rune('A'+g.seq-1)) + "00000000000000000"
}

// httpFixture は handler テストで使う Mux + 種データ。
type httpFixture struct {
	mux     *httptest.Server
	pubUC   *usecase.PublishedTableUseCase
	pickUC  *usecase.PickUseCase
	srcRepo *persistence.SourceTableRepoSQL
	pubRepo *persistence.PublishedTableRepoSQL
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))

	srcRepo := persistence.NewSourceTableRepoSQL(db)
	pubRepo := persistence.NewPublishedTableRepoSQL(db)
	cfgStore := persistence.NewConfigStoreSQL(db)
	owned := usecase.NewOwnedMD5Cache(
		persistence.NewSongdataReader(),
		cfgStore,
		stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	store := usecase.NewPickResultStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pubUC := usecase.NewPublishedTableUseCase(pubRepo, srcRepo, &stubIDGen{}, logger)
	pickUC := usecase.NewPickUseCase(
		pubRepo, srcRepo, owned, store,
		stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)},
		port.RandSourceFactory(func(seed int64) port.RandSource { return randsrc.NewMathRandSource(seed) }),
		logger,
	)

	mux := httpserver.NewMux(httpserver.Deps{Pick: pickUC, Pub: pubUC, Log: logger})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &httpFixture{
		mux:     srv,
		pubUC:   pubUC,
		pickUC:  pickUC,
		srcRepo: srcRepo,
		pubRepo: pubRepo,
	}
}

// seedSourceWithCharts はソース表 + 譜面を本物の Repo へ保存する。
func (f *httpFixture) seedSourceWithCharts(t *testing.T, id, name string, levelOrder []string, charts []domain.SourceChart) {
	t.Helper()
	_, err := f.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: name, LevelOrder: levelOrder,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, f.srcRepo.SaveFetched(context.Background(), id, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: name, Symbol: "sl", DataURL: "data.json", LevelOrder: levelOrder},
		Charts: charts,
		ETag:   "",
	}, time.Now()))
}

// seedPublished は公開表を作成する。
func (f *httpFixture) seedPublished(t *testing.T, slug, sourceID string, mode domain.RefreshMode, perLevel int, ownedOnly bool) string {
	t.Helper()
	id, err := f.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: slug, DisplayName: slug, Symbol: "sl",
		SourceTableID: sourceID, OwnedOnly: ownedOnly, PickPerLevel: perLevel, RefreshMode: mode,
	})
	require.NoError(t, err)
	return id
}
