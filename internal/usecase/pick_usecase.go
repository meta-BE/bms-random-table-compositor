package usecase

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// PickUseCase はピック生成を担う。
// docs/superpowers/specs/2026-05-10-multi-source-table-composition-design.md §3 の
// フェーズ 1 (各マッピングから m 曲) + フェーズ 2 (合計 n 曲まで補填) を実装する。
type PickUseCase struct {
	pubRepo  port.PublishedTableRepo
	srcRepo  port.SourceTableRepo
	store    *PickResultStore
	clock    port.Clock
	randNew  port.RandSourceFactory
	log      *slog.Logger
	weighter port.Weighter
}

// NewPickUseCase は新しい PickUseCase を作る。
// weighter は重み付き非復元サンプリング時の重み関数。MVP では UniformWeighter を渡す。
func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
	weighter port.Weighter,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, store: store,
		clock: clock, randNew: randNew, log: log, weighter: weighter,
	}
}

// PickBySlug は slug から公開表を取得し、モードに応じてキャッシュ判定 / 再生成する。
func (u *PickUseCase) PickBySlug(ctx context.Context, slug string) (domain.PickResult, domain.PublishedTable, error) {
	pub, err := u.pubRepo.GetBySlug(ctx, slug)
	if err != nil {
		return domain.PickResult{}, domain.PublishedTable{}, err
	}

	if cached, ok := u.cachedIfFresh(pub); ok {
		return cached, pub, nil
	}

	r, err := u.regenerate(ctx, pub)
	if err != nil {
		return domain.PickResult{}, pub, err
	}
	u.store.Set(pub.ID, r)
	return r, pub, nil
}

// ManualRefresh は手動再ピック。即座に再生成して store を上書きする。
// ※ spec §7.4 の `POST /:slug/_refresh` で呼ばれるパス。GUI からも呼ぶ。
func (u *PickUseCase) ManualRefresh(ctx context.Context, publishedID string) error {
	pub, err := u.pubRepo.Get(ctx, publishedID)
	if err != nil {
		return err
	}
	r, err := u.regenerate(ctx, pub)
	if err != nil {
		return err
	}
	u.store.Set(pub.ID, r)
	u.log.Info("pick manually refreshed", "id", pub.ID, "slug", pub.Slug)
	return nil
}

// InvalidateAll は store の全エントリを削除する。
// 設定変更（songdata_db_path 変更等）で SongdataAttacher が再接続された後に呼ばれる想定。
func (u *PickUseCase) InvalidateAll() {
	u.store.Clear()
}

// cachedIfFresh はモード別のキャッシュ判定。返り値の bool が true ならそのまま使える。
func (u *PickUseCase) cachedIfFresh(pub domain.PublishedTable) (domain.PickResult, bool) {
	cached, ok := u.store.Get(pub.ID)
	if !ok {
		return domain.PickResult{}, false
	}
	switch pub.Pick.RefreshMode {
	case domain.RefreshModeManual:
		return cached, true
	case domain.RefreshModeDaily:
		todayKey := u.clock.Now().Local().Format("2006-01-02")
		if cached.SeedKey == todayKey {
			return cached, true
		}
	}
	return domain.PickResult{}, false
}

// regenerate はピック結果を一から作成する。
// pub.Levels を SortOrder 順に走査し、各公開レベルごとに pickLevel() でフェーズ 1+2 を実行する。
// 各公開レベルのシードは baseSeed XOR fnv32(level.ID) として独立させ、レベル単位の
// 決定論性を確保する（公開レベル順や追加・削除の影響が他レベルに波及しない）。
func (u *PickUseCase) regenerate(ctx context.Context, pub domain.PublishedTable) (domain.PickResult, error) {
	baseSeed, seedKey := u.makeSeed(pub)
	now := u.clock.Now()

	var finalCharts []domain.PickedChart
	var finalLevelOrder []string

	for _, lv := range pub.Levels {
		levelSeed := baseSeed ^ int64(fnv32(lv.ID))
		rng := rand.New(u.randNew(levelSeed))
		picked, err := u.pickLevel(ctx, pub, lv, rng, now)
		if err != nil {
			return domain.PickResult{}, fmt.Errorf("pick level %q: %w", lv.Name, err)
		}
		if len(picked) == 0 {
			continue
		}
		finalCharts = append(finalCharts, picked...)
		finalLevelOrder = append(finalLevelOrder, lv.Name)
	}

	return domain.PickResult{
		PublishedTableID: pub.ID,
		GeneratedAt:      u.clock.Now(),
		SeedKey:          seedKey,
		Charts:           finalCharts,
		LevelOrder:       finalLevelOrder,
	}, nil
}

// pickLevel は 1 公開レベル分のフェーズ 1 + フェーズ 2 を実行する。
// ソース譜面の LoadCharts はソース表 ID ごとに 1 度だけ呼ぶ（マッピング数が多い場合の重複呼び出し回避）。
// dedup の主キーは MD5、空なら SHA256 をフォールバック。
// 出力する PickedChart は EnrichedChart (ソース由来) + PublicLevel (公開レベル名 lv.Name) を持つ。
// EnrichedChart.Level はソース側のレベルそのままで、HTML 行頭セル等で参照される。
func (u *PickUseCase) pickLevel(
	ctx context.Context, pub domain.PublishedTable, lv domain.PublishedTableLevel,
	rng *rand.Rand, now time.Time,
) ([]domain.PickedChart, error) {
	// ソース表 ID → EnrichedChart[] のキャッシュ。同じ source_table_id を複数マッピングが参照しても LoadCharts は 1 回。
	sources := map[string][]domain.EnrichedChart{}
	for _, mp := range lv.Mappings {
		if _, ok := sources[mp.SourceTableID]; ok {
			continue
		}
		src, err := u.srcRepo.Get(ctx, mp.SourceTableID)
		if err != nil {
			return nil, fmt.Errorf("get source table %q: %w", mp.SourceTableID, err)
		}
		if src.LastFetchStatus == domain.FetchStatusNever {
			return nil, ErrSourceNotFetched
		}
		cs, err := u.srcRepo.LoadCharts(ctx, mp.SourceTableID, port.ChartQuery{OwnedOnly: pub.OwnedOnly})
		if err != nil {
			return nil, fmt.Errorf("load charts %q: %w", mp.SourceTableID, err)
		}
		sources[mp.SourceTableID] = cs
	}

	// 各マッピングの局所プール（source level でフィルタ）
	pools := make([][]domain.EnrichedChart, len(lv.Mappings))
	for i, mp := range lv.Mappings {
		pools[i] = filterByLevel(sources[mp.SourceTableID], mp.SourceLevel)
	}

	// dedup キー（MD5 主、空なら SHA256）
	keyOf := func(c domain.EnrichedChart) string {
		if c.MD5 != "" {
			return "md5:" + c.MD5
		}
		return "sha:" + c.SHA256
	}

	// unionPool: SortOrder 昇順（= mappings 順）で走査し、重複は最初に出会ったマッピング側のみ採用する。
	// フェーズ 2 で「フェーズ 1 で取り切れなかった残りプール」として使う。
	seenUnion := map[string]struct{}{}
	var unionPool []domain.EnrichedChart
	for _, p := range pools {
		for _, c := range p {
			k := keyOf(c)
			if _, ok := seenUnion[k]; ok {
				continue
			}
			seenUnion[k] = struct{}{}
			unionPool = append(unionPool, c)
		}
	}

	// フェーズ 1: 各マッピングから m 曲。既選曲は除外。
	var picked []domain.EnrichedChart
	pickedKeys := map[string]struct{}{}
	m := lv.PerMappingPick
	for i := range pools {
		avail := make([]domain.EnrichedChart, 0, len(pools[i]))
		for _, c := range pools[i] {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			avail = append(avail, c)
		}
		sort.SliceStable(avail, func(a, b int) bool { return avail[a].Position < avail[b].Position })
		taken := weightedSampleWithoutReplacement(ctx, avail, m, u.weighter, rng, now)
		for _, c := range taken {
			pickedKeys[keyOf(c)] = struct{}{}
		}
		picked = append(picked, taken...)
	}

	// フェーズ 2: 合計 n 曲を目標に、unionPool 残りから補填。
	// sum(m) > n の場合は need <= 0 となりスキップ（フェーズ 1 で既に超過分を取得している）。
	need := lv.TotalPick - len(picked)
	if need > 0 {
		remaining := make([]domain.EnrichedChart, 0, len(unionPool))
		for _, c := range unionPool {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			remaining = append(remaining, c)
		}
		sort.SliceStable(remaining, func(a, b int) bool { return remaining[a].Position < remaining[b].Position })
		taken := weightedSampleWithoutReplacement(ctx, remaining, need, u.weighter, rng, now)
		for _, c := range taken {
			pickedKeys[keyOf(c)] = struct{}{}
		}
		picked = append(picked, taken...)
	}

	// 出力前: PickedChart に包んで PublicLevel を併記する (Level はソース側のまま)。
	out := make([]domain.PickedChart, 0, len(picked))
	for _, c := range picked {
		out = append(out, domain.PickedChart{EnrichedChart: c, PublicLevel: lv.Name})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Position < out[b].Position })
	return out, nil
}

// filterByLevel は EnrichedChart 列を SourceChart.Level 一致でフィルタする。
func filterByLevel(charts []domain.EnrichedChart, level string) []domain.EnrichedChart {
	out := make([]domain.EnrichedChart, 0, len(charts))
	for _, c := range charts {
		if c.Level == level {
			out = append(out, c)
		}
	}
	return out
}

// weightedSampleWithoutReplacement は重み付き非復元サンプリング（k 件まで）。
// 重み 0 以下の譜面は対象外。pool が k 件未満なら採れた件数だけ返す。
// 累積重み + 線形走査による「ロト方式」を採用（pool 規模が高々数百〜数千なので O(k * |pool|) で十分）。
func weightedSampleWithoutReplacement(
	ctx context.Context, pool []domain.EnrichedChart, k int,
	w port.Weighter, rng *rand.Rand, now time.Time,
) []domain.EnrichedChart {
	if k <= 0 || len(pool) == 0 {
		return nil
	}
	weights := make([]float64, len(pool))
	totalWeight := 0.0
	for i, c := range pool {
		wt := w.Weight(ctx, c, now)
		if wt <= 0 {
			weights[i] = 0
			continue
		}
		weights[i] = wt
		totalWeight += wt
	}
	taken := make([]domain.EnrichedChart, 0, k)
	used := make([]bool, len(pool))
	for len(taken) < k && totalWeight > 0 {
		r := rng.Float64() * totalWeight
		cum := 0.0
		picked := -1
		for i, wt := range weights {
			if used[i] || wt <= 0 {
				continue
			}
			cum += wt
			if r <= cum {
				picked = i
				break
			}
		}
		if picked < 0 {
			break
		}
		taken = append(taken, pool[picked])
		totalWeight -= weights[picked]
		used[picked] = true
	}
	return taken
}

// makeSeed はモード別のシードと SeedKey を返す。
func (u *PickUseCase) makeSeed(pub domain.PublishedTable) (int64, string) {
	now := u.clock.Now()
	hash := fnv32(pub.ID)
	switch pub.Pick.RefreshMode {
	case domain.RefreshModeDaily:
		key := now.Local().Format("2006-01-02")
		num, _ := strconv.ParseInt(now.Local().Format("20060102"), 10, 64)
		return num + int64(hash), key
	case domain.RefreshModePerRequest:
		nano := now.UnixNano()
		key := strconv.FormatInt(nano, 10)
		return nano ^ int64(hash), key
	case domain.RefreshModeManual:
		nano := now.UnixNano()
		key := now.UTC().Format(time.RFC3339Nano)
		return nano ^ int64(hash), key
	default:
		nano := now.UnixNano()
		return nano ^ int64(hash), strconv.FormatInt(nano, 10)
	}
}

func fnv32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
