package httpserver_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/weighter"
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
	mux      *httptest.Server
	pubUC    *usecase.PublishedTableUseCase
	pickUC   *usecase.PickUseCase
	srcRepo  *persistence.SourceTableRepoSQL
	pubRepo  *persistence.PublishedTableRepoSQL
	attacher *persistence.SongdataAttacher
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))

	db.SetMaxOpenConns(1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attacher := persistence.NewSongdataAttacher(db, stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}, logger)
	srcRepo := persistence.NewSourceTableRepoSQL(db, attacher)
	pubRepo := persistence.NewPublishedTableRepoSQL(db)
	store := usecase.NewPickResultStore()
	pubUC := usecase.NewPublishedTableUseCase(pubRepo, srcRepo, &stubIDGen{}, logger)
	pickUC := usecase.NewPickUseCase(
		pubRepo, srcRepo, store,
		stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)},
		port.RandSourceFactory(func(seed int64) port.RandSource { return randsrc.NewMathRandSource(seed) }),
		logger,
		weighter.Factory{},
	)

	mux := httpserver.NewMux(httpserver.Deps{Pick: pickUC, Pub: pubUC, Log: logger})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &httpFixture{
		mux:      srv,
		pubUC:    pubUC,
		pickUC:   pickUC,
		srcRepo:  srcRepo,
		pubRepo:  pubRepo,
		attacher: attacher,
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

// seedPublished は公開表を作成する。マッピングは sourceID + 各レベル 1:1 で生成する（旧 PerLevel 仕様の代替）。
// symbolOverride を渡すと Symbol を指定値に上書きする (省略時は "sl"; 既存テストとの後方互換のため可変引数)。
func (f *httpFixture) seedPublished(t *testing.T, slug, sourceID string, mode domain.RefreshMode, perLevel int, ownedOnly bool, symbolOverride ...string) string {
	t.Helper()
	symbol := "sl"
	if len(symbolOverride) > 0 {
		symbol = symbolOverride[0]
	}
	src, err := f.srcRepo.Get(context.Background(), sourceID)
	require.NoError(t, err)

	// 旧 PerLevel セマンティクスへの後方互換マッピング:
	// perLevel == 0 (旧「全件ピック」) → PerMappingPick=巨大値で phase 1 が全件取る
	// perLevel  > 0                   → PerMappingPick=perLevel で phase 1 が perLevel 件取る
	mPerMapping := perLevel
	if perLevel == 0 {
		// 実質無制限。weightedSampleWithoutReplacement は make([]T, 0, k) で k 分の容量を
		// 即時確保するため、過大な値を渡すとテストが OOM/時間切れになる。テストの pool は高々
		// 数十件なので 999999 で十分。
		mPerMapping = 999999
	}

	// src.LevelOrder が空のケース (例: 未フェッチ source の 503 テスト) でも、
	// pickLevel が source の状態を検査できるよう最低 1 レベルは作る。
	// 公開レベル名は空不可なので "all" をフォールバックに使う (source 側はマッチングしない空文字列 OK)。
	srcLevels := src.LevelOrder
	if len(srcLevels) == 0 {
		srcLevels = []string{"all"}
	}
	levels := make([]usecase.PublishedTableLevelInput, 0, len(srcLevels))
	for _, lv := range srcLevels {
		levels = append(levels, usecase.PublishedTableLevelInput{
			Name:           lv,
			PerMappingPick: mPerMapping,
			TotalPick:      0,
			Mappings: []usecase.PublishedTableLevelMappingInput{
				{SourceTableID: sourceID, SourceLevel: lv},
			},
		})
	}
	id, err := f.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: slug, DisplayName: slug, Symbol: symbol,
		OwnedOnly: ownedOnly, RefreshMode: mode,
		Levels: levels,
	})
	require.NoError(t, err)
	return id
}

// seedAttachedSongdata は temp dir に最小スキーマの songdata.db を作り attacher で ATTACH する。
// テストで IsOwned 判定を検証するために使う。
func (f *httpFixture) seedAttachedSongdata(t *testing.T, ownedMD5s ...string) {
	t.Helper()
	dir := t.TempDir()
	songdataPath := filepath.Join(dir, "songdata.db")
	src, err := sql.Open("sqlite", songdataPath)
	require.NoError(t, err)
	_, err = src.Exec(`CREATE TABLE song (md5 TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	for _, m := range ownedMD5s {
		_, err = src.Exec(`INSERT INTO song(md5) VALUES (?)`, m)
		require.NoError(t, err)
	}
	require.NoError(t, src.Close())
	require.NoError(t, f.attacher.Attach(context.Background(), songdataPath))
}
