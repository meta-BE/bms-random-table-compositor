package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/weighter"
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
	return newPickUCFixtureWithWeighter(t, weighter.UniformWeighter{})
}

// newPickUCFixtureWithWeighter は任意の Weighter を注入できる fixture コンストラクタ。
// TestPickUseCase_WeighterFiltersZeroWeights など、特定の譜面の重みを 0 にしたいテスト用。
func newPickUCFixtureWithWeighter(t *testing.T, w port.Weighter) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger, w)
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

// mappingSpec は seedPubWithLevels に渡す 1 マッピングの仕様。
type mappingSpec struct {
	srcID, level string
}

// levelSpec は seedPubWithLevels に渡す 1 公開レベル分の仕様。
type levelSpec struct {
	name     string
	m, n     int
	mappings []mappingSpec
}

// seedPubWithLevels は新仕様の Levels/Mappings を組み立てて pubRepo.Create する。
func (f *pickUCFixture) seedPubWithLevels(
	t *testing.T, id, slug string, ownedOnly bool, mode domain.RefreshMode, specs []levelSpec,
) {
	t.Helper()
	levels := make([]domain.PublishedTableLevel, 0, len(specs))
	for i, s := range specs {
		mappings := make([]domain.PublishedTableLevelMapping, 0, len(s.mappings))
		for j, mp := range s.mappings {
			mappings = append(mappings, domain.PublishedTableLevelMapping{
				ID:                    "MAP-" + id + "-" + s.name + "-" + mp.srcID + "-" + mp.level,
				PublishedTableLevelID: "LV-" + id + "-" + s.name,
				SourceTableID:         mp.srcID,
				SourceLevel:           mp.level,
				SortOrder:             j,
			})
		}
		levels = append(levels, domain.PublishedTableLevel{
			ID:               "LV-" + id + "-" + s.name,
			PublishedTableID: id,
			Name:             s.name,
			SortOrder:        i,
			PerMappingPick:   s.m,
			TotalPick:        s.n,
			Mappings:         mappings,
		})
	}
	_, err := f.pubRepo.Create(context.Background(), domain.PublishedTable{
		ID: id, Slug: slug, DisplayName: slug,
		OwnedOnly: ownedOnly,
		Pick:      domain.PickConfig{RefreshMode: mode},
		Levels:    levels,
	})
	require.NoError(t, err)
}

// zeroWeightWeighter は特定 MD5 の譜面の重みを 0 にする Weighter。
// TestPickUseCase_WeighterFiltersZeroWeights 用。
type zeroWeightWeighter struct {
	zeroMD5 string
}

func (z zeroWeightWeighter) Weight(_ context.Context, c domain.EnrichedChart, _ time.Time) float64 {
	if c.MD5 == z.zeroMD5 {
		return 0
	}
	return 1
}

func zeroWeightFor(md5 string) port.Weighter {
	return zeroWeightWeighter{zeroMD5: md5}
}

// ---- 既存仕様から保持する基本テスト ----

func TestPickUseCase_NotFound(t *testing.T) {
	f := newPickUCFixture(t)
	_, _, err := f.uc.PickBySlug(context.Background(), "no-such-slug")
	require.True(t, errors.Is(err, usecase.ErrPublishedTableNotFound))
}

func TestPickUseCase_SourceNotFetchedReturnsError(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusNever, nil)
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.True(t, errors.Is(err, usecase.ErrSourceNotFetched))
}

func TestPickUseCase_DailyMode_SameDayCached(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
		chartFixture("SRC1", "0", 2, "c"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeDaily, []levelSpec{
		{name: "0", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "0"}}},
	})

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
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeManual, []levelSpec{
		{name: "0", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "0"}}},
	})

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
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "0", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "0"}}},
	})

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	f.clock.t = f.clock.t.Add(1 * time.Nanosecond)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.SeedKey, second.SeedKey)
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
		f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeDaily, []levelSpec{
			{name: "0", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "0"}}},
		})
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

// TestPickUseCase_DeterministicAcrossRestarts は複数公開レベル + 複数マッピング条件下でも
// 同一シードで結果が完全一致することを保証する。各レベルのシードを baseSeed XOR fnv32(level.ID)
// で独立させているので、Go map の反復順揺らぎが結果に影響しないはず。
func TestPickUseCase_DeterministicAcrossRestarts(t *testing.T) {
	build := func() *pickUCFixture {
		f := newPickUCFixture(t)
		var charts []domain.SourceChart
		levels := []string{"0", "1", "2", "3", "4"}
		pos := 0
		for _, lv := range levels {
			for i := 0; i < 5; i++ {
				charts = append(charts, chartFixture("SRC1", lv, pos,
					"L"+lv+"-"+string(rune('a'+i))))
				pos++
			}
		}
		f.seedSource(t, "SRC1", levels, domain.FetchStatusOK, charts)
		specs := make([]levelSpec, 0, len(levels))
		for _, lv := range levels {
			specs = append(specs, levelSpec{
				name: lv, m: 2, n: 2,
				mappings: []mappingSpec{{srcID: "SRC1", level: lv}},
			})
		}
		f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeDaily, specs)
		return f
	}

	expected, _, err := build().uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)

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

func TestPickUseCase_InvalidateAll_ClearsStore(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeManual, []levelSpec{
		{name: "0", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "0"}}},
	})

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, f.store.Snapshot(), 1)

	f.uc.InvalidateAll()
	require.Empty(t, f.store.Snapshot())
}

// ---- 新仕様（フェーズ 1+2）固有のテスト ----

// フェーズ 1 のみ: m=1, n=0, 2 マッピング → 各 1 曲、合計 2 曲。フェーズ 2 はスキップ。
func TestPickUseCase_PhaseOneOnly_NEqualsZero(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a1"),
		chartFixture("SRC1", "5", 1, "a2"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 0, "b1"),
		chartFixture("SRC2", "5", 1, "b2"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 0, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2, "m=1 * 2 mappings = 2 曲。フェーズ 2 はスキップ")
	require.Equal(t, []string{"5"}, r.LevelOrder)
}

// フェーズ 2 が合計 n に達するまで補填する: m=1, n=4, 2 マッピング → フェーズ 1 で 2、フェーズ 2 で +2 → 合計 4。
func TestPickUseCase_PhaseTwoFillsToTotalN(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a1"),
		chartFixture("SRC1", "5", 1, "a2"),
		chartFixture("SRC1", "5", 2, "a3"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 0, "b1"),
		chartFixture("SRC2", "5", 1, "b2"),
		chartFixture("SRC2", "5", 2, "b3"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 4, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 4, "フェーズ 1 で 2、フェーズ 2 で +2 → 合計 4")
}

// sum(m) > n の場合はフェーズ 2 をスキップ: m=3, n=4, 2 マッピング → フェーズ 1 で 6（n 超過）。
func TestPickUseCase_SumOfMExceedsN_PhaseTwoSkipped(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a1"),
		chartFixture("SRC1", "5", 1, "a2"),
		chartFixture("SRC1", "5", 2, "a3"),
		chartFixture("SRC1", "5", 3, "a4"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 0, "b1"),
		chartFixture("SRC2", "5", 1, "b2"),
		chartFixture("SRC2", "5", 2, "b3"),
		chartFixture("SRC2", "5", 3, "b4"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 3, n: 4, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 6, "m=3 * 2 mappings = 6。n=4 を超えるためフェーズ 2 はスキップ")
}

// 同一 MD5 が複数マッピングに含まれる場合 dedup される: m=1, n=1, 2 マッピング、両方に MD5="X" のみ → 1 曲のみ。
func TestPickUseCase_Dedup_SameMD5InMultipleMappings(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "X"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 0, "X"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 1, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 1, "MD5 が同じならフェーズ 1 の 2 マッピング目で dedup されて 1 曲のみ")
	require.Equal(t, "X", r.Charts[0].MD5)
}

// 供給不足の場合は補填しない: m=2 だが pool に 1 曲のみ → 1 曲だけ返る。
func TestPickUseCase_InsufficientSupply_NoCompensation(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "only"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 2, n: 0, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 1, "pool 1 曲しかないので m=2 でも 1 曲しか取れない")
}

// daily モードで同一日内なら同じ結果が返る（決定論性）。
func TestPickUseCase_Deterministic_DailyMode(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a"),
		chartFixture("SRC1", "5", 1, "b"),
		chartFixture("SRC1", "5", 2, "c"),
		chartFixture("SRC1", "5", 3, "d"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModeDaily, []levelSpec{
		{name: "5", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// 別の時刻（同じ日）でもう 1 度
	f.clock.t = f.clock.t.Add(3 * time.Hour)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, first.SeedKey, second.SeedKey)
	require.Equal(t, len(first.Charts), len(second.Charts))
	for i := range first.Charts {
		require.Equal(t, first.Charts[i].MD5, second.Charts[i].MD5)
	}
}

// Weighter で重み 0 を返した譜面はピック対象外になる。
// pool に 2 曲（"keep", "skip"）、m=1, n=1 で skip の重みを 0 にすると必ず "keep" が返る。
func TestPickUseCase_WeighterFiltersZeroWeights(t *testing.T) {
	f := newPickUCFixtureWithWeighter(t, zeroWeightFor("skip"))
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "skip"),
		chartFixture("SRC1", "5", 1, "keep"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 1)
	require.Equal(t, "keep", r.Charts[0].MD5, "重み 0 の譜面は選ばれないはず")
}

// LevelOrder は pub.Levels の SortOrder 順（seedPubWithLevels の specs 配列順）で組まれる。
func TestPickUseCase_LevelOrderRespectsLevels(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"a", "b", "c"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "a", 0, "ma"),
		chartFixture("SRC1", "b", 1, "mb"),
		chartFixture("SRC1", "c", 2, "mc"),
	})
	// 公開レベル順を意図的に [c, a, b] と入れ替える
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "Lc", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "c"}}},
		{name: "La", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "a"}}},
		{name: "Lb", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "b"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, []string{"Lc", "La", "Lb"}, r.LevelOrder)
}

// PickedChart は EnrichedChart.Level (= ソース表側のレベル) を保持しつつ、
// 公開レベル名は PublicLevel フィールドに別途保存する。
// HTML 行頭セルではソースレベルを使い、data.json/header.json では公開レベル名を使う想定。
func TestPickUseCase_PublicLevelTrackedSeparately(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "x"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5-mix", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 1)
	require.Equal(t, "5", r.Charts[0].Level, "ソースレベル (EnrichedChart.Level) は保持される")
	require.Equal(t, "5-mix", r.Charts[0].PublicLevel, "公開レベル名は PublicLevel フィールドに入る")
	require.Equal(t, []string{"5-mix"}, r.LevelOrder)
}

// OwnedOnly: pool 構築時点で IsOwned=false の譜面が落ちる（既存仕様の継続）。
func TestPickUseCase_OwnedOnlyFiltersBeforePick(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "owned-1"),
		chartFixture("SRC1", "5", 1, "not-owned"),
		chartFixture("SRC1", "5", 2, "owned-2"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", true /* ownedOnly */, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 0, n: 10, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})
	f.srcRepo.markOwned("owned-1", "owned-2")

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	for _, c := range r.Charts {
		require.NotEqual(t, "not-owned", c.MD5)
	}
}

// 結果の Charts は最終的に Position 昇順で並ぶ（pickLevel の出力 stable sort 仕様）。
func TestPickUseCase_OutputSortedByPositionWithinLevel(t *testing.T) {
	f := newPickUCFixture(t)
	// 意図的に position が降順に並んだ pool を仕込む
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 30, "z"),
		chartFixture("SRC1", "5", 10, "x"),
		chartFixture("SRC1", "5", 20, "y"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 0, n: 3, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 3)
	positions := make([]int, len(r.Charts))
	for i, c := range r.Charts {
		positions[i] = c.Position
	}
	require.True(t, sort.IntsAreSorted(positions), "Position 昇順で並ぶはず: %v", positions)
}
