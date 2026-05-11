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

// stubWeighterFactory は固定の Weighter を返す port.WeighterFactory のテスト実装。
// 既存テスト互換のため、newPickUCFixtureWithWeighter が指定 Weighter を Factory 経由で注入できるよう薄ラップする。
type stubWeighterFactory struct {
	w port.Weighter
}

func (f stubWeighterFactory) For(_ domain.PickConfig) port.Weighter { return f.w }

// newPickUCFixtureWithWeighter は任意の Weighter を注入できる fixture コンストラクタ。
// TestPickUseCase_WeighterFiltersZeroWeights など、特定の譜面の重みを 0 にしたいテスト用。
func newPickUCFixtureWithWeighter(t *testing.T, w port.Weighter) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger, stubWeighterFactory{w: w})
	return &pickUCFixture{uc: uc, pubRepo: pub, srcRepo: src, store: store, clock: clock}
}

// newPickUCFixtureWithFactory は port.WeighterFactory ごとそのまま注入できる fixture コンストラクタ。
// 実 Factory (weighter.Factory{}) を使って PickConfig 由来の切り替えそのものをテストする用途。
func newPickUCFixtureWithFactory(t *testing.T, factory port.WeighterFactory) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger, factory)
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
	f.seedPubWithLevelsAndPick(t, id, slug, ownedOnly, domain.PickConfig{RefreshMode: mode}, specs)
}

// seedPubWithLevelsAndPick は PickConfig を直接渡せる版。WeightMode/WeightParamX を指定する WeightMode テスト用。
func (f *pickUCFixture) seedPubWithLevelsAndPick(
	t *testing.T, id, slug string, ownedOnly bool, pick domain.PickConfig, specs []levelSpec,
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
		Pick:      pick,
		Levels:    levels,
	})
	require.NoError(t, err)
}

// aZeroWeighter は aOf=0 (= unionPool 内で最新プレイ) の譜面の重みを 0 にする Weighter。
// TestPickUseCase_WeighterFiltersZeroWeights では「skip 譜面の LastPlayedAt を now と同時刻」、
// 「keep 譜面は LastPlayedAt=nil で未プレイ扱い (aOf=1)」と組み合わせて使う。
type aZeroWeighter struct{}

func (aZeroWeighter) Weight(a float64) float64 {
	if a == 0 {
		return 0
	}
	return 1
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
// pool に 2 曲（"keep", "skip"）、m=1, n=1。
// skip の LastPlayedAt を now と同時刻にして aOf=0 を作り、
// aZeroWeighter で「aOf=0 の重みは 0」とすることで skip が除外されることを確認する。
func TestPickUseCase_WeighterFiltersZeroWeights(t *testing.T) {
	f := newPickUCFixtureWithWeighter(t, aZeroWeighter{})
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "skip"),
		chartFixture("SRC1", "5", 1, "keep"),
	})
	// skip = 最新 (aOf=0)、keep = 1 時間前 (aOf=1 = unionPool 内 max)。
	f.srcRepo.setLastPlayed("skip", f.clock.t)
	f.srcRepo.setLastPlayed("keep", f.clock.t.Add(-1*time.Hour))
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

// 複数マッピング合成時、出力は (mappingIdx 昇順, Position 昇順) で並ぶ。
// マッピング 0 由来の譜面が全件先に並び、続いてマッピング 1 由来。
// フェーズ 2 で全体プールから補填された譜面も「起源マッピング群」に混ざる（末尾に固まらない）。
func TestPickUseCase_OutputGroupedByMappingThenPosition(t *testing.T) {
	f := newPickUCFixture(t)
	// SRC1 (mapping 0): position 10, 20, 30 で 3 曲
	// SRC2 (mapping 1): position 5, 15 で 2 曲
	// PerMappingPick=1 → フェーズ 1 で各マッピング 1 曲
	// TotalPick=5 → フェーズ 2 で残り全部補填 (合計 5 曲)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 10, "a1"),
		chartFixture("SRC1", "5", 20, "a2"),
		chartFixture("SRC1", "5", 30, "a3"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 5, "b1"),
		chartFixture("SRC2", "5", 15, "b2"),
	})
	f.seedPubWithLevels(t, "PUB1", "p1", false, domain.RefreshModePerRequest, []levelSpec{
		{name: "5", m: 1, n: 5, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 5)

	// 出力順を SourceID で見ると、SRC1 由来が前半 (3 件) → SRC2 由来が後半 (2 件)
	srcIDs := make([]string, len(r.Charts))
	positions := make([]int, len(r.Charts))
	for i, c := range r.Charts {
		srcIDs[i] = c.SourceID
		positions[i] = c.Position
	}
	require.Equal(t, []string{"SRC1", "SRC1", "SRC1", "SRC2", "SRC2"}, srcIDs)
	// 各マッピング群内では Position 昇順
	require.Equal(t, []int{10, 20, 30, 5, 15}, positions)
}

// ---- WeightMode 分岐 (Task 8 Step 8) ----

// WeightMode=sort: 古い譜面ほど優先される決定論的ソート。
// マッピング 0 内で「3 曲、それぞれ LastPlayedAt が古い順 c→b→a」だが pool は a, b, c の順に並ぶ。
// PerMappingPick=2 で a 降順 (= 古い順) で c, b が選ばれ、結果は mappingIdx + Position 順で b, c。
func TestPickUseCase_WeightModeSort_PicksOldestFirst(t *testing.T) {
	f := newPickUCFixtureWithFactory(t, weighter.Factory{})
	now := f.clock.t
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a"), // 最新
		chartFixture("SRC1", "5", 1, "b"), // 中間
		chartFixture("SRC1", "5", 2, "c"), // 最古
	})
	f.srcRepo.setLastPlayed("a", now.Add(-1*time.Hour))
	f.srcRepo.setLastPlayed("b", now.Add(-24*time.Hour))
	f.srcRepo.setLastPlayed("c", now.Add(-72*time.Hour))
	f.seedPubWithLevelsAndPick(t, "PUB1", "p1", false, domain.PickConfig{
		RefreshMode: domain.RefreshModePerRequest,
		WeightMode:  domain.WeightModeSort,
	}, []levelSpec{
		{name: "5", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	// 出力順は (mappingIdx 昇順, Position 昇順)。同マッピングなので Position 昇順 → b(pos=1), c(pos=2)。
	require.Equal(t, "b", r.Charts[0].MD5)
	require.Equal(t, "c", r.Charts[1].MD5)
}

// WeightMode=sort + フェーズ 2: マッピング 1 にしか古い曲がない場合、フェーズ 2 で union から古い順に補填。
func TestPickUseCase_WeightModeSort_PhaseTwoOrderedByAge(t *testing.T) {
	f := newPickUCFixtureWithFactory(t, weighter.Factory{})
	now := f.clock.t
	// SRC1: a (最新), b (中間)
	// SRC2: c (最古), d (やや新しい)
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "a"),
		chartFixture("SRC1", "5", 1, "b"),
	})
	f.seedSource(t, "SRC2", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC2", "5", 0, "c"),
		chartFixture("SRC2", "5", 1, "d"),
	})
	f.srcRepo.setLastPlayed("a", now.Add(-1*time.Hour))
	f.srcRepo.setLastPlayed("b", now.Add(-12*time.Hour))
	f.srcRepo.setLastPlayed("c", now.Add(-72*time.Hour))
	f.srcRepo.setLastPlayed("d", now.Add(-6*time.Hour))
	// m=1 → 各マッピング 1 曲、n=3 → フェーズ 2 で +1 曲
	f.seedPubWithLevelsAndPick(t, "PUB1", "p1", false, domain.PickConfig{
		RefreshMode: domain.RefreshModePerRequest,
		WeightMode:  domain.WeightModeSort,
	}, []levelSpec{
		{name: "5", m: 1, n: 3, mappings: []mappingSpec{
			{srcID: "SRC1", level: "5"},
			{srcID: "SRC2", level: "5"},
		}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 3)
	// フェーズ 1: SRC1 最古 = b, SRC2 最古 = c
	// フェーズ 2 残り: a(SRC1), d(SRC2) → 古い順 d→a。1 曲補填で d を採用。
	// 出力 (mappingIdx 昇順, Position 昇順): SRC1.b → SRC2.c, SRC2.d (pos 0, 1)
	md5s := make([]string, len(r.Charts))
	for i, c := range r.Charts {
		md5s[i] = c.MD5
	}
	require.Equal(t, []string{"b", "c", "d"}, md5s)
}

// WeightMode=probability + X=10000: 古い曲に大きく偏る (seed 固定で複数回試行)。
// 古い曲 ("old") と新しい曲 ("new1"..."new4") 計 5 曲から 1 曲ピックを seed 固定で 1 回実行し、
// X=10000 なら old (aOf=1) の重みは new (aOf≒0) の 10000 倍となり、old が選ばれる確率がほぼ 1。
func TestPickUseCase_WeightModeProbability_BiasedToOlder(t *testing.T) {
	f := newPickUCFixtureWithFactory(t, weighter.Factory{})
	now := f.clock.t
	charts := []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "old"),
	}
	for i := 1; i <= 4; i++ {
		charts = append(charts, chartFixture("SRC1", "5", i, "new"+string(rune('0'+i))))
	}
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, charts)
	f.srcRepo.setLastPlayed("old", now.Add(-100*time.Hour))
	for i := 1; i <= 4; i++ {
		f.srcRepo.setLastPlayed("new"+string(rune('0'+i)), now.Add(-1*time.Second))
	}
	f.seedPubWithLevelsAndPick(t, "PUB1", "p1", false, domain.PickConfig{
		RefreshMode:  domain.RefreshModeDaily,
		WeightMode:   domain.WeightModeProbability,
		WeightParamX: 10000,
	}, []levelSpec{
		{name: "5", m: 1, n: 1, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	// Daily モード + 固定 clock なら決定論。X=10000 なら old が圧倒的に選ばれるはず。
	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 1)
	require.Equal(t, "old", r.Charts[0].MD5,
		"X=10000 で old (aOf=1) の重みは new (aOf≒0) の 10000 倍。決定論シードでも old を引くはず")
}

// 全曲未プレイ: aOf 分母 = 0 → 全 aOf=0 → 一様ランダムへ退化する。
// probability + X=10000 でも一様になるので、出力が一様 Weighter の場合と同じ譜面群になる。
func TestPickUseCase_WeightModeProbability_AllUnplayedFallsBackToUniform(t *testing.T) {
	build := func(mode domain.WeightMode) []string {
		f := newPickUCFixtureWithFactory(t, weighter.Factory{})
		f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
			chartFixture("SRC1", "5", 0, "a"),
			chartFixture("SRC1", "5", 1, "b"),
			chartFixture("SRC1", "5", 2, "c"),
			chartFixture("SRC1", "5", 3, "d"),
		})
		// 全曲 LastPlayedAt = nil (= 未プレイ)
		f.seedPubWithLevelsAndPick(t, "PUB1", "p1", false, domain.PickConfig{
			RefreshMode:  domain.RefreshModeDaily,
			WeightMode:   mode,
			WeightParamX: 10000,
		}, []levelSpec{
			{name: "5", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
		})
		r, _, err := f.uc.PickBySlug(context.Background(), "p1")
		require.NoError(t, err)
		md5s := make([]string, len(r.Charts))
		for i, c := range r.Charts {
			md5s[i] = c.MD5
		}
		return md5s
	}
	off := build(domain.WeightModeOff)
	prob := build(domain.WeightModeProbability)
	require.Equal(t, off, prob, "全曲未プレイなら probability でも off と同じ (一様)")
}

// 未プレイ譜面と最古プレイ譜面が sort 出力で隣接する (どちらも aOf=1.0 同点)。
// sort 経路では同点時 mappingIdx 昇順 + Position 昇順で安定。
func TestPickUseCase_WeightModeSort_UnplayedTiedWithOldestPlayed(t *testing.T) {
	f := newPickUCFixtureWithFactory(t, weighter.Factory{})
	now := f.clock.t
	f.seedSource(t, "SRC1", []string{"5"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "5", 0, "oldest"),   // 最古プレイ → aOf=1
		chartFixture("SRC1", "5", 1, "unplayed"), // 未プレイ → aOf=1 (同点)
		chartFixture("SRC1", "5", 2, "recent"),   // 中間 → aOf<1
	})
	f.srcRepo.setLastPlayed("oldest", now.Add(-100*time.Hour))
	f.srcRepo.setLastPlayed("recent", now.Add(-1*time.Hour))
	// unplayed は setLastPlayed しない (= nil)
	f.seedPubWithLevelsAndPick(t, "PUB1", "p1", false, domain.PickConfig{
		RefreshMode: domain.RefreshModePerRequest,
		WeightMode:  domain.WeightModeSort,
	}, []levelSpec{
		{name: "5", m: 2, n: 2, mappings: []mappingSpec{{srcID: "SRC1", level: "5"}}},
	})

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	// 同マッピング、同 aOf=1.0 の 2 曲。Position 昇順 → oldest(pos=0), unplayed(pos=1)
	md5s := make([]string, len(r.Charts))
	for i, c := range r.Charts {
		md5s[i] = c.MD5
	}
	require.Equal(t, []string{"oldest", "unplayed"}, md5s,
		"未プレイ譜面と最古プレイ譜面は aOf=1 で同点、出力は Position 昇順で隣接")
}
