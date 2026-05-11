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
// spec §6.3 に従い、公開表内に現れる全 source_table_id について
// srcRepo.Get / LoadCharts を 1 回ずつだけ呼んで結果をキャッシュし、各公開レベルへ渡す。
// 各公開レベルのシードは baseSeed XOR fnv32(level.ID) として独立させ、レベル単位の
// 決定論性を確保する（公開レベル順や追加・削除の影響が他レベルに波及しない）。
func (u *PickUseCase) regenerate(ctx context.Context, pub domain.PublishedTable) (domain.PickResult, error) {
	now := u.clock.Now()
	chartsBySrcLevel, err := u.loadSourceCharts(ctx, pub)
	if err != nil {
		return domain.PickResult{}, err
	}

	baseSeed, seedKey := u.makeSeed(pub, now)

	var finalCharts []domain.PickedChart
	var finalLevelOrder []string

	for _, lv := range pub.Levels {
		levelSeed := baseSeed ^ int64(fnv32(lv.ID))
		rng := rand.New(u.randNew(levelSeed))
		picked, err := u.pickLevel(ctx, lv, chartsBySrcLevel, rng, now)
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
		GeneratedAt:      now,
		SeedKey:          seedKey,
		Charts:           finalCharts,
		LevelOrder:       finalLevelOrder,
	}, nil
}

// loadSourceCharts は pub.Levels に現れる全ソース表について
// 存在 / fetch 状態確認 + LoadCharts を 1 回ずつ実行し、結果を (srcID, level) でバケット化して返す。
// バケット化により pickLevel 側のマッピング毎の線形フィルタを O(1) lookup に削減する。
// 同一ソース表が複数の公開レベル・マッピングから参照されても再ロードしない。
func (u *PickUseCase) loadSourceCharts(ctx context.Context, pub domain.PublishedTable) (map[string]map[string][]domain.EnrichedChart, error) {
	out := map[string]map[string][]domain.EnrichedChart{}
	for _, lv := range pub.Levels {
		for _, mp := range lv.Mappings {
			if _, ok := out[mp.SourceTableID]; ok {
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
			byLevel := map[string][]domain.EnrichedChart{}
			for _, c := range cs {
				byLevel[c.Level] = append(byLevel[c.Level], c)
			}
			out[mp.SourceTableID] = byLevel
		}
	}
	return out, nil
}

// pickLevel は 1 公開レベル分のフェーズ 1 + フェーズ 2 を実行する。
// chartsBySrcLevel は regenerate でロード・バケット化済みの「ソース表 ID → ソースレベル → EnrichedChart[]」マップ。
// dedup の主キーは MD5、空なら SHA256 をフォールバック。
// 出力する PickedChart は EnrichedChart (ソース由来) + PublicLevel (公開レベル名 lv.Name) を持つ。
func (u *PickUseCase) pickLevel(
	ctx context.Context, lv domain.PublishedTableLevel,
	chartsBySrcLevel map[string]map[string][]domain.EnrichedChart,
	rng *rand.Rand, now time.Time,
) ([]domain.PickedChart, error) {
	pools := make([][]domain.EnrichedChart, len(lv.Mappings))
	for i, mp := range lv.Mappings {
		pools[i] = chartsBySrcLevel[mp.SourceTableID][mp.SourceLevel]
	}

	keyOf := func(c domain.EnrichedChart) string {
		if c.MD5 != "" {
			return "md5:" + c.MD5
		}
		return "sha:" + c.SHA256
	}

	// unionPool: SortOrder 昇順（= mappings 順）で走査し、重複は最初に出会ったマッピング側のみ採用。
	// chartOriginMapping: フェーズ 2 で取られた譜面を「最初に出現したマッピング」の群に並べるための起源情報。
	seenUnion := map[string]struct{}{}
	chartOriginMapping := map[string]int{}
	var unionPool []domain.EnrichedChart
	for i, p := range pools {
		for _, c := range p {
			k := keyOf(c)
			if _, ok := seenUnion[k]; ok {
				continue
			}
			seenUnion[k] = struct{}{}
			chartOriginMapping[k] = i
			unionPool = append(unionPool, c)
		}
	}

	type pickedItem struct {
		chart      domain.EnrichedChart
		mappingIdx int
	}

	var picked []pickedItem
	pickedKeys := map[string]struct{}{}

	// フェーズ 1: 各マッピングから m 曲。pools[i] は LoadCharts の position 昇順を保つ。
	m := lv.PerMappingPick
	for i := range pools {
		avail := make([]domain.EnrichedChart, 0, len(pools[i]))
		for _, c := range pools[i] {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			avail = append(avail, c)
		}
		taken := weightedSampleWithoutReplacement(ctx, avail, m, u.weighter, rng, now)
		for _, c := range taken {
			pickedKeys[keyOf(c)] = struct{}{}
			picked = append(picked, pickedItem{chart: c, mappingIdx: i})
		}
	}

	// フェーズ 2: sum(picked) < n なら unionPool 残りから補填。
	need := lv.TotalPick - len(picked)
	if need > 0 {
		remaining := make([]domain.EnrichedChart, 0, len(unionPool))
		for _, c := range unionPool {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			remaining = append(remaining, c)
		}
		taken := weightedSampleWithoutReplacement(ctx, remaining, need, u.weighter, rng, now)
		for _, c := range taken {
			k := keyOf(c)
			pickedKeys[k] = struct{}{}
			picked = append(picked, pickedItem{chart: c, mappingIdx: chartOriginMapping[k]})
		}
	}

	// 出力整列: (mappingIdx 昇順, Position 昇順)。フェーズ 2 由来も起源マッピング群に混ぜる。
	sort.SliceStable(picked, func(a, b int) bool {
		if picked[a].mappingIdx != picked[b].mappingIdx {
			return picked[a].mappingIdx < picked[b].mappingIdx
		}
		return picked[a].chart.Position < picked[b].chart.Position
	})

	out := make([]domain.PickedChart, 0, len(picked))
	for _, p := range picked {
		out = append(out, domain.PickedChart{EnrichedChart: p.chart, PublicLevel: lv.Name})
	}
	return out, nil
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
// 呼出側で取得した now を共有することで、regenerate 内の時刻参照を 1 回に揃える。
func (u *PickUseCase) makeSeed(pub domain.PublishedTable, now time.Time) (int64, string) {
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
