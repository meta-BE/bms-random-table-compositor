package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// stubRand は決定論的に動く RandSource。Int63 は単調に進む数列を返す。
type stubRand struct {
	seed int64
	step int64
}

func (s *stubRand) Int63() int64    { s.step++; return s.seed*1000 + s.step }
func (s *stubRand) Seed(seed int64) { s.seed = seed; s.step = 0 }

func newStubFactory() port.RandSourceFactory {
	return func(seed int64) port.RandSource { return &stubRand{seed: seed} }
}

func chartFixture(sourceID, level string, pos int, md5 string) domain.SourceChart {
	return domain.SourceChart{
		SourceID: sourceID, Position: pos, Level: level,
		MD5: md5, Title: "T-" + md5, Artist: "A", Raw: map[string]any{"md5": md5},
	}
}

// pickUCFixture は PickUseCase + 各種 fake/in-memory コンポーネントを束ねたテスト fixture。
type pickUCFixture struct {
	uc      *usecase.PickUseCase
	pubRepo *fakePublishedRepo
	srcRepo *fakeSourceRepo
	store   *usecase.PickResultStore
	clock   *mutableClock
}

type mutableClock struct{ t time.Time }

func (c *mutableClock) Now() time.Time { return c.t }

func newPickUCFixture(t *testing.T) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger)
	return &pickUCFixture{uc: uc, pubRepo: pub, srcRepo: src, store: store, clock: clock}
}

func (f *pickUCFixture) seedSource(t *testing.T, id string, levelOrder []string, status domain.FetchStatus, charts []domain.SourceChart) {
	t.Helper()
	_, err := f.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: id, LevelOrder: levelOrder, LastFetchStatus: status,
	})
	require.NoError(t, err)
	for _, c := range charts {
		c.SourceID = id
	}
	f.srcRepo.charts[id] = charts
}

func (f *pickUCFixture) seedPub(t *testing.T, id, slug, sourceID string, ownedOnly bool, perLevel int, mode domain.RefreshMode) {
	t.Helper()
	_, err := f.pubRepo.Create(context.Background(), domain.PublishedTable{
		ID: id, Slug: slug, DisplayName: slug,
		SourceTableID: sourceID, OwnedOnly: ownedOnly,
		Pick: domain.PickConfig{PerLevel: perLevel, RefreshMode: mode},
	})
	require.NoError(t, err)
}

func TestPickUseCase_NotFound(t *testing.T) {
	f := newPickUCFixture(t)
	_, _, err := f.uc.PickBySlug(context.Background(), "no-such-slug")
	require.True(t, errors.Is(err, usecase.ErrPublishedTableNotFound))
}

func TestPickUseCase_SourceNotFetchedReturnsError(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusNever, nil)
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.True(t, errors.Is(err, usecase.ErrSourceNotFetched))
}

func TestPickUseCase_PerLevelZeroReturnsAll(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0", "1"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "aaa"),
		chartFixture("SRC1", "0", 1, "bbb"),
		chartFixture("SRC1", "1", 2, "ccc"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 3)
	require.Equal(t, []string{"0", "1"}, r.LevelOrder)
}

func TestPickUseCase_PerLevelLimitsResults(t *testing.T) {
	f := newPickUCFixture(t)
	charts := []domain.SourceChart{}
	for i := 0; i < 5; i++ {
		charts = append(charts, chartFixture("SRC1", "0", i, "L0-"+string(rune('a'+i))))
	}
	for i := 0; i < 2; i++ {
		charts = append(charts, chartFixture("SRC1", "1", 10+i, "L1-"+string(rune('a'+i))))
	}
	f.seedSource(t, "SRC1", []string{"0", "1"}, domain.FetchStatusOK, charts)
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 3, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	level0 := 0
	level1 := 0
	for _, c := range r.Charts {
		switch c.Level {
		case "0":
			level0++
		case "1":
			level1++
		}
	}
	require.Equal(t, 3, level0)
	require.Equal(t, 2, level1)
}

func TestPickUseCase_OwnedOnlyFiltersBeforePick(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "owned-1"),
		chartFixture("SRC1", "0", 1, "not-owned"),
		chartFixture("SRC1", "0", 2, "owned-2"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", true, 0, domain.RefreshModePerRequest)
	f.srcRepo.markOwned("owned-1", "owned-2")

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	for _, c := range r.Charts {
		require.NotEqual(t, "not-owned", c.MD5)
	}
}

func TestPickUseCase_OwnedOnly_NoOwnedReturnsEmpty(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "x"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", true, 0, domain.RefreshModePerRequest)
	// markOwned は呼ばない → owned セットが空なので 0 件が返る

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Empty(t, r.Charts)
	require.Empty(t, r.LevelOrder, "1 曲以上残ったレベルが無いので level_order は空")
}

func TestPickUseCase_DailyMode_SameDayCached(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
		chartFixture("SRC1", "0", 2, "c"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)

	f.clock.t = f.clock.t.Add(2 * time.Hour)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, first.GeneratedAt, second.GeneratedAt, "同じ日のキャッシュが返るはず")

	f.clock.t = f.clock.t.AddDate(0, 0, 1)
	third, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.GeneratedAt, third.GeneratedAt, "日付が変わったので再生成")
	require.NotEqual(t, first.SeedKey, third.SeedKey)
}

func TestPickUseCase_ManualMode_KeepsCacheUntilManualRefresh(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 1, domain.RefreshModeManual)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	f.clock.t = f.clock.t.Add(48 * time.Hour)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, first.GeneratedAt, second.GeneratedAt)

	require.NoError(t, f.uc.ManualRefresh(context.Background(), "PUB1"))
	third, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.GeneratedAt, third.GeneratedAt)
}

func TestPickUseCase_PerRequestMode_RegeneratesEachCall(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 1, domain.RefreshModePerRequest)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	f.clock.t = f.clock.t.Add(1 * time.Nanosecond)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.SeedKey, second.SeedKey)
}

func TestPickUseCase_LevelOrderRespected(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"sl0", "sl1", "sl2"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "sl2", 0, "c1"),
		chartFixture("SRC1", "sl0", 1, "a1"),
		chartFixture("SRC1", "sl1", 2, "b1"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, []string{"sl0", "sl1", "sl2"}, r.LevelOrder)
	require.Equal(t, "sl0", r.Charts[0].Level)
	require.Equal(t, "sl1", r.Charts[1].Level)
	require.Equal(t, "sl2", r.Charts[2].Level)
}

func TestPickUseCase_LevelOrder_FallbackWhenSourceHasNone(t *testing.T) {
	f := newPickUCFixture(t)
	// 数値レベルのみ → 文字列ソートでは ["1","10","2"] になるが、自然順では数値昇順 ["1","2","10"]
	f.seedSource(t, "SRC1", nil, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "10", 0, "x"),
		chartFixture("SRC1", "1", 1, "y"),
		chartFixture("SRC1", "2", 2, "z"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "10"}, r.LevelOrder)
}

// 数値解釈できないレベル（"段位1" 等）を含む場合、数値が先で文字列が末尾になる。
func TestPickUseCase_LevelOrder_FallbackMixedNumericAndString(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", nil, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "段位2", 0, "a"),
		chartFixture("SRC1", "10", 1, "b"),
		chartFixture("SRC1", "段位1", 2, "c"),
		chartFixture("SRC1", "2", 3, "d"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// 数値レベルが昇順で先、文字列レベルが昇順で末尾
	require.Equal(t, []string{"2", "10", "段位1", "段位2"}, r.LevelOrder)
}

func TestPickUseCase_DeterministicWithSameSeed(t *testing.T) {
	build := func() *pickUCFixture {
		f := newPickUCFixture(t)
		f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
			chartFixture("SRC1", "0", 0, "a"),
			chartFixture("SRC1", "0", 1, "b"),
			chartFixture("SRC1", "0", 2, "c"),
			chartFixture("SRC1", "0", 3, "d"),
		})
		f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)
		return f
	}
	a := build()
	b := build()
	ra, _, err := a.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	rb, _, err := b.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, len(ra.Charts), len(rb.Charts))
	for i := range ra.Charts {
		require.Equal(t, ra.Charts[i].MD5, rb.Charts[i].MD5)
	}
}

// TestPickUseCase_DeterministicAcrossRestarts は、同一シードのもとで
// regenerate を何度走らせても結果が完全一致することを保証する。
// 複数レベル + PerLevel 制限の条件下で、シャッフルループが Go の map 反復
// ランダム化に依存していると、レベルごとに割り当たる乱数列がブレて
// 同一日でも結果が変わる（再起動で変わって見える）。複数回ループで検出する。
func TestPickUseCase_DeterministicAcrossRestarts(t *testing.T) {
	build := func() *pickUCFixture {
		f := newPickUCFixture(t)
		var charts []domain.SourceChart
		levels := []string{"0", "1", "2", "3", "4"}
		pos := 0
		for _, lv := range levels {
			for i := 0; i < 5; i++ {
				charts = append(charts, chartFixture("SRC1", lv, pos,
					fmt.Sprintf("L%s-%d", lv, i)))
				pos++
			}
		}
		f.seedSource(t, "SRC1", levels, domain.FetchStatusOK, charts)
		f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)
		return f
	}

	expected, _, err := build().uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)

	// 複数回新しい fixture（新しい store / 新しい map）で生成して
	// map 反復順がブレても結果が一致することを確認する。
	for i := 0; i < 50; i++ {
		got, _, err := build().uc.PickBySlug(context.Background(), "p1")
		require.NoError(t, err)
		require.Equal(t, len(expected.Charts), len(got.Charts), "iteration %d", i)
		for j := range expected.Charts {
			require.Equal(t, expected.Charts[j].MD5, got.Charts[j].MD5,
				"iteration %d, position %d", i, j)
		}
		require.Equal(t, expected.LevelOrder, got.LevelOrder, "iteration %d", i)
	}
}

// TestPickUseCase_DeterministicAcrossRestarts_NoSrcLevelOrder は、
// ソース表が level_order を提供しない場合（自然順フォールバック）でも
// 同一シードなら結果が一致することを保証する。
func TestPickUseCase_DeterministicAcrossRestarts_NoSrcLevelOrder(t *testing.T) {
	build := func() *pickUCFixture {
		f := newPickUCFixture(t)
		var charts []domain.SourceChart
		pos := 0
		for lv := 0; lv < 6; lv++ {
			for i := 0; i < 4; i++ {
				charts = append(charts, chartFixture("SRC1", strconv.Itoa(lv), pos,
					fmt.Sprintf("L%d-%d", lv, i)))
				pos++
			}
		}
		// level_order を空にしてフォールバック経路を通す
		f.seedSource(t, "SRC1", nil, domain.FetchStatusOK, charts)
		f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)
		return f
	}

	expected, _, err := build().uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)

	for i := 0; i < 50; i++ {
		got, _, err := build().uc.PickBySlug(context.Background(), "p1")
		require.NoError(t, err)
		for j := range expected.Charts {
			require.Equal(t, expected.Charts[j].MD5, got.Charts[j].MD5,
				"iteration %d, position %d", i, j)
		}
	}
}

func TestPickUseCase_InvalidateAll_ClearsStore(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModeManual)

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, f.store.Snapshot(), 1)

	f.uc.InvalidateAll()
	require.Empty(t, f.store.Snapshot())
}
