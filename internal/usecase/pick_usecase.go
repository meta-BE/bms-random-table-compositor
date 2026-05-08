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

// PickUseCase はピック生成を担う。spec §7.2 のフローを実装。
type PickUseCase struct {
	pubRepo port.PublishedTableRepo
	srcRepo port.SourceTableRepo
	store   *PickResultStore
	clock   port.Clock
	randNew port.RandSourceFactory
	log     *slog.Logger
}

// NewPickUseCase は新しい PickUseCase を作る。
func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, store: store,
		clock: clock, randNew: randNew, log: log,
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
// 設定変更（songdata_db_path 変更等）で OwnedMD5Cache が invalidate された後に呼ばれる想定。
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
func (u *PickUseCase) regenerate(ctx context.Context, pub domain.PublishedTable) (domain.PickResult, error) {
	src, err := u.srcRepo.Get(ctx, pub.SourceTableID)
	if err != nil {
		return domain.PickResult{}, fmt.Errorf("get source table %q: %w", pub.SourceTableID, err)
	}
	if src.LastFetchStatus == domain.FetchStatusNever {
		return domain.PickResult{}, ErrSourceNotFetched
	}
	all, err := u.srcRepo.LoadCharts(ctx, pub.SourceTableID, port.ChartQuery{
		OwnedOnly: pub.OwnedOnly,
	})
	if err != nil {
		return domain.PickResult{}, fmt.Errorf("load charts %q: %w", pub.SourceTableID, err)
	}

	// レベル別グルーピング
	byLevel := map[string][]domain.EnrichedChart{}
	for _, c := range all {
		byLevel[c.Level] = append(byLevel[c.Level], c)
	}

	// シード生成
	seed, seedKey := u.makeSeed(pub)

	// レベル並び順を先に確定する。
	// rng の消費順を Go の map 反復順（仕様上ランダム化される）に依存させると、
	// 同一シードでも実行ごとに各レベルに割り当たる乱数列がズレて
	// daily モードでも再起動ごとに結果が変わる。これを避けるため、
	// シャッフル前に決定論的なレベル順序を組み立てる。
	levelOrder := buildLevelOrder(src.LevelOrder, byLevel)

	// レベル別シャッフル + 先頭 N 曲（または全件）。levelOrder の順に rng を消費する。
	rng := rand.New(u.randNew(seed))
	var finalCharts []domain.SourceChart
	var finalLevelOrder []string
	for _, level := range levelOrder {
		charts, ok := byLevel[level]
		if !ok || len(charts) == 0 {
			continue
		}
		// position 昇順でいったん並べ替え（決定論性保証）
		sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		if pub.Pick.PerLevel > 0 && len(charts) > pub.Pick.PerLevel {
			rng.Shuffle(len(charts), func(i, j int) { charts[i], charts[j] = charts[j], charts[i] })
			charts = charts[:pub.Pick.PerLevel]
			sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		}
		for _, ec := range charts {
			finalCharts = append(finalCharts, ec.SourceChart)
		}
		finalLevelOrder = append(finalLevelOrder, level)
	}

	r := domain.PickResult{
		PublishedTableID: pub.ID,
		GeneratedAt:      u.clock.Now(),
		SeedKey:          seedKey,
		Charts:           finalCharts,
		LevelOrder:       finalLevelOrder,
	}
	return r, nil
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

// buildLevelOrder は決定論的なレベル並び順を返す。
// srcOrder（ソース難易度表が宣言した並び）を最優先し、そこに含まれない実在レベルを
// 自然順で末尾に並べる。srcOrder が空の場合は全レベルを自然順で並べる。
// シャッフルループより前にこの順序を確定することで、
// rng 消費順が Go map の反復順ランダム化に左右されないようにする。
func buildLevelOrder(srcOrder []string, byLevel map[string][]domain.EnrichedChart) []string {
	if len(srcOrder) == 0 {
		order := make([]string, 0, len(byLevel))
		for k := range byLevel {
			order = append(order, k)
		}
		sortLevelsNatural(order)
		return order
	}
	known := make(map[string]struct{}, len(srcOrder))
	for _, l := range srcOrder {
		known[l] = struct{}{}
	}
	extra := make([]string, 0, len(byLevel))
	for k := range byLevel {
		if _, ok := known[k]; !ok {
			extra = append(extra, k)
		}
	}
	sortLevelsNatural(extra)
	order := make([]string, 0, len(srcOrder)+len(extra))
	order = append(order, srcOrder...)
	order = append(order, extra...)
	return order
}

// sortLevelsNatural は BMS 難易度表のレベル列を自然順に並べる。
// 数値として解釈できるレベル（"0", "12", "1.5" 等）を数値昇順で先に置き、
// 解釈不能な文字列（"段位1", "?" 等）を文字列昇順で末尾に置く。
// 同じ数値（"1" と "1.0" 等）は文字列で安定整列。
// bms-elsa の `ORDER BY CAST(level AS INTEGER) = 0 AND level != '0', CAST(level AS INTEGER), level`
// と同じ意図で、SQLite CAST AS INTEGER の代わりに float64 解釈を使う。
func sortLevelsNatural(levels []string) {
	sort.SliceStable(levels, func(i, j int) bool {
		ai, aok := parseLevelNumeric(levels[i])
		bj, bok := parseLevelNumeric(levels[j])
		if aok != bok {
			return aok // 数値解釈できる方が先
		}
		if aok {
			if ai != bj {
				return ai < bj
			}
		}
		return levels[i] < levels[j]
	})
}

// parseLevelNumeric は level 文字列を float64 として解釈する。失敗時は (0, false)。
func parseLevelNumeric(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
