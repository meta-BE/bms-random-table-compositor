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
	owned   *OwnedMD5Cache
	store   *PickResultStore
	clock   port.Clock
	randNew port.RandSourceFactory
	log     *slog.Logger
}

// NewPickUseCase は新しい PickUseCase を作る。
func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	owned *OwnedMD5Cache,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, owned: owned, store: store,
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
	all, err := u.srcRepo.LoadCharts(ctx, pub.SourceTableID)
	if err != nil {
		return domain.PickResult{}, fmt.Errorf("load charts %q: %w", pub.SourceTableID, err)
	}

	// 所持絞り込み（OwnedOnly=true 時）
	if pub.OwnedOnly {
		ownedSet, err := u.owned.Get(ctx)
		if err != nil {
			u.log.Warn("owned md5 get failed, falling back to empty set", "err", err)
			ownedSet = map[string]struct{}{}
		}
		filtered := make([]domain.SourceChart, 0, len(all))
		for _, c := range all {
			if _, ok := ownedSet[c.MD5]; ok {
				filtered = append(filtered, c)
			}
		}
		all = filtered
	}

	// レベル別グルーピング
	byLevel := map[string][]domain.SourceChart{}
	for _, c := range all {
		byLevel[c.Level] = append(byLevel[c.Level], c)
	}

	// シード生成
	seed, seedKey := u.makeSeed(pub)

	// レベル別シャッフル + 先頭 N 曲（または全件）
	rng := rand.New(u.randNew(seed))
	for level, charts := range byLevel {
		// position 昇順でいったん並べ替え（決定論性保証）
		sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		if pub.Pick.PerLevel > 0 && len(charts) > pub.Pick.PerLevel {
			rng.Shuffle(len(charts), func(i, j int) { charts[i], charts[j] = charts[j], charts[i] })
			charts = charts[:pub.Pick.PerLevel]
			sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		}
		byLevel[level] = charts
	}

	// レベル順序の決定: ソース表 level_order があればそれに従い、無ければ自然順
	order := src.LevelOrder
	if len(order) == 0 {
		order = make([]string, 0, len(byLevel))
		for k := range byLevel {
			order = append(order, k)
		}
		sort.Strings(order)
	}

	// 最終 Charts と level_order（残ったレベルのみ）を組み立て
	var finalCharts []domain.SourceChart
	var finalLevelOrder []string
	for _, level := range order {
		charts, ok := byLevel[level]
		if !ok || len(charts) == 0 {
			continue
		}
		finalCharts = append(finalCharts, charts...)
		finalLevelOrder = append(finalLevelOrder, level)
	}

	// level_order に無いレベルが残っていれば末尾追加（自然順）
	if len(src.LevelOrder) > 0 {
		known := map[string]struct{}{}
		for _, l := range src.LevelOrder {
			known[l] = struct{}{}
		}
		var extra []string
		for k, v := range byLevel {
			if _, ok := known[k]; !ok && len(v) > 0 {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, l := range extra {
			finalCharts = append(finalCharts, byLevel[l]...)
			finalLevelOrder = append(finalLevelOrder, l)
		}
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
