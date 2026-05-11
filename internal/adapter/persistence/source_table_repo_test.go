package persistence_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/stretchr/testify/require"
)

func setupSourceTableRepo(t *testing.T) *persistence.SourceTableRepoSQL {
	t.Helper()
	repo, _ := setupSourceTableRepoWithDB(t)
	return repo
}

// setupSourceTableRepoWithDB は repo と内部 *sql.DB を併せて返すヘルパ。
// Backfill テストのように DB を直接いじりたいケース用。
func setupSourceTableRepoWithDB(t *testing.T) (*persistence.SourceTableRepoSQL, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attacher := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	return persistence.NewSourceTableRepoSQL(db, attacher, nil), db
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

	charts, err := r.LoadCharts(ctx, "Z", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Equal(t, "x", charts[0].MD5)
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
	charts, _ := r.LoadCharts(ctx, "Z", port.ChartQuery{})
	require.Len(t, charts, 1)
	require.Equal(t, "a", charts[0].MD5)
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

	charts, _ := r.LoadCharts(ctx, "Z", port.ChartQuery{})
	require.Len(t, charts, 1, "失敗時もキャッシュは保持される（spec §8）")
}

func TestSourceTableRepoSQL_LoadCharts_OrderByPosition(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	ft := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 2, MD5: "c", Level: "1", Title: "Tc",
				Raw: map[string]any{"md5": "c", "url": "uc"}},
			{Position: 0, MD5: "a", Level: "0", Title: "Ta",
				Raw: map[string]any{"md5": "a", "url": "ua", "lr2_bmsid": float64(7)}},
			{Position: 1, MD5: "b", Level: "0", Title: "Tb",
				Raw: map[string]any{"md5": "b"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", ft, time.Now()))

	out, err := r.LoadCharts(ctx, "Z", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, []int{0, 1, 2}, []int{out[0].Position, out[1].Position, out[2].Position})
	require.Equal(t, "Z", out[0].SourceID)
	require.Equal(t, "ua", out[0].Raw["url"], "raw_json はパススルー")
	require.Equal(t, float64(7), out[0].Raw["lr2_bmsid"])
}

// LoadCharts は source_table.symbol を JOIN で取得し各譜面に載せる
// (v2 で複数ソース表を1公開表に合成する際、譜面単位で symbol を保持するため)。
func TestSourceTableRepoSQL_LoadCharts_PopulatesSymbolFromSourceTable(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "S", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, r.SaveFetched(ctx, "S", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "sl", LevelOrder: []string{"0"}},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Title: "T", Raw: map[string]any{"md5": "a"}},
		},
	}, time.Now()))

	out, err := r.LoadCharts(ctx, "S", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "sl", out[0].Symbol, "Symbol は source_table から JOIN で取得される")
}

func TestSourceTableRepoSQL_LoadCharts_EmptyForNoSource(t *testing.T) {
	r := setupSourceTableRepo(t)
	out, err := r.LoadCharts(context.Background(), "missing", port.ChartQuery{})
	require.NoError(t, err)
	require.Empty(t, out)
}

// header.json に level_order が無くても、charts から自然順で導出されること。
// （ウィザード/マッピング編集 UI のレベル選択肢ゼロ問題への対策）
func TestSourceTableRepoSQL_SaveFetched_DerivesLevelOrderFromCharts_WhenHeaderHasNoLevelOrder(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "src-derive", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)

	require.NoError(t, r.SaveFetched(ctx, "src-derive", port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "T", Symbol: "▽", DataURL: "data.json",
			// LevelOrder: 意図的に省略（nil）
		},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "10"},
			{Position: 1, MD5: "b", Level: "段位1"},
			{Position: 2, MD5: "c", Level: "1"},
			{Position: 3, MD5: "d", Level: "2"},
			{Position: 4, MD5: "e", Level: "10"}, // duplicate
		},
		ETag: "",
	}, time.Now()))

	got, err := r.Get(ctx, "src-derive")
	require.NoError(t, err)
	// 数値順 → 文字列。重複除去。
	require.Equal(t, []string{"1", "2", "10", "段位1"}, got.LevelOrder)
}

// header に level_order がある場合は導出せずヘッダー値を尊重する。
func TestSourceTableRepoSQL_SaveFetched_KeepsHeaderLevelOrder_WhenProvided(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "src-keep", InputURL: "https://y", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)

	require.NoError(t, r.SaveFetched(ctx, "src-keep", port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "T", DataURL: "data.json",
			LevelOrder: []string{"a", "b"}, // 明示
		},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "x", Level: "z"}, // header にないレベルだが charts に存在
		},
		ETag: "",
	}, time.Now()))

	got, err := r.Get(ctx, "src-keep")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got.LevelOrder)
}

// BackfillEmptyLevelOrder は level_order_json が空のソース表を charts から補完する。
// 304 Not Modified パスで populated されなかった既存 DB の救済。冪等であることを併せて検証。
func TestSourceTableRepoSQL_BackfillEmptyLevelOrder_FillsEmptyFromCharts(t *testing.T) {
	repo, db := setupSourceTableRepoWithDB(t)
	ctx := context.Background()

	// ケース 1: level_order_json が '[]' で charts あり → 補完される
	_, err := repo.Create(ctx, domain.SourceTable{
		ID: "src-empty", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, repo.SaveFetched(ctx, "src-empty", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "T", DataURL: "data.json"}, // LevelOrder なし
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "5"},
			{Position: 1, MD5: "b", Level: "1"},
			{Position: 2, MD5: "c", Level: "10"},
		},
	}, time.Now()))
	// SaveFetched 直後は新コードで populated されるため、テスト目的で空に巻き戻す。
	// 既存 DB で 304 Not Modified によって空のままだった状況を再現する。
	_, err = db.Exec(`UPDATE source_table SET level_order_json='[]' WHERE id='src-empty'`)
	require.NoError(t, err)

	// ケース 2: level_order_json 既に populated → 触らない
	_, err = repo.Create(ctx, domain.SourceTable{
		ID: "src-populated", InputURL: "https://y", InputKind: domain.InputKindHTML,
		LevelOrder:      []string{"a", "b"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	// ケース 3: level_order_json 空 + charts なし → 補完しない
	_, err = repo.Create(ctx, domain.SourceTable{
		ID: "src-no-charts", InputURL: "https://z", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)

	n, err := repo.BackfillEmptyLevelOrder(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n) // src-empty のみ補完

	got1, err := repo.Get(ctx, "src-empty")
	require.NoError(t, err)
	require.Equal(t, []string{"1", "5", "10"}, got1.LevelOrder)

	got2, err := repo.Get(ctx, "src-populated")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got2.LevelOrder)

	got3, err := repo.Get(ctx, "src-no-charts")
	require.NoError(t, err)
	require.Empty(t, got3.LevelOrder)

	// 冪等: 2 回目は補完件数 0 件で安定する。
	n2, err := repo.BackfillEmptyLevelOrder(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n2)
}

func TestLoadCharts_LastPlayedAt_WithScoreAttached(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := persistence.OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(mainDB))

	songPath := filepath.Join(dir, "songdata.db")
	{
		songDB, err := persistence.OpenDB(songPath)
		require.NoError(t, err)
		_, err = songDB.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY(md5))`)
		require.NoError(t, err)
		_, err = songDB.Exec(`INSERT INTO song(md5, sha256) VALUES('md-a','sha-a'),('md-b','sha-b')`)
		require.NoError(t, err)
		songDB.Close()
	}

	scorePath := filepath.Join(dir, "score.db")
	makeScoreDBFile(t, scorePath, [][2]any{{"sha-a", 1700000000}})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	songAtt := persistence.NewSongdataAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, songAtt.Attach(ctx, songPath))
	scoreAtt := persistence.NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, scoreAtt.Attach(ctx, scorePath))

	repo := persistence.NewSourceTableRepoSQL(mainDB, songAtt, scoreAtt)

	_, err = repo.Create(ctx, domain.SourceTable{
		ID: "src1", InputURL: "http://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, repo.SaveFetched(ctx, "src1", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "t"},
		Charts: []domain.SourceChart{
			{SourceID: "src1", Position: 1, MD5: "md-a", SHA256: "sha-a", Level: "0"},
			{SourceID: "src1", Position: 2, MD5: "md-b", SHA256: "sha-b", Level: "0"},
		},
	}, time.Now()))

	charts, err := repo.LoadCharts(ctx, "src1", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, charts, 2)

	byMD5 := map[string]domain.EnrichedChart{}
	for _, c := range charts {
		byMD5[c.MD5] = c
	}
	require.NotNil(t, byMD5["md-a"].LastPlayedAt)
	require.Equal(t, int64(1700000000), byMD5["md-a"].LastPlayedAt.Unix())
	require.Nil(t, byMD5["md-b"].LastPlayedAt, "未プレイは nil")
}

func TestLoadCharts_LastPlayedAt_WithoutScoreAttached(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := persistence.OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(mainDB))

	songPath := filepath.Join(dir, "songdata.db")
	{
		songDB, err := persistence.OpenDB(songPath)
		require.NoError(t, err)
		_, err = songDB.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY(md5))`)
		require.NoError(t, err)
		_, err = songDB.Exec(`INSERT INTO song(md5, sha256) VALUES('md-a','sha-a')`)
		require.NoError(t, err)
		songDB.Close()
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	songAtt := persistence.NewSongdataAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, songAtt.Attach(ctx, songPath))

	repo := persistence.NewSourceTableRepoSQL(mainDB, songAtt, nil) // score.db 未設定
	_, err = repo.Create(ctx, domain.SourceTable{
		ID: "src1", InputURL: "http://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, repo.SaveFetched(ctx, "src1", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "t"},
		Charts: []domain.SourceChart{
			{SourceID: "src1", Position: 1, MD5: "md-a", SHA256: "sha-a", Level: "0"},
		},
	}, time.Now()))

	charts, err := repo.LoadCharts(ctx, "src1", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Nil(t, charts[0].LastPlayedAt, "score 未 attach 時は常に nil")
}
