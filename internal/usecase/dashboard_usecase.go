package usecase

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

const (
	dashboardRequestsCap = 100
	dashboardFetchesCap  = 100
)

// DashboardUseCase はダッシュボード表示用の最近のリクエスト / ソース表更新履歴を
// メモリリングバッファで保持し、ピック結果は PickResultStore から取得する。
type DashboardUseCase struct {
	requests  *RingBuffer[domain.RequestLogEntry]
	fetches   *RingBuffer[domain.FetchLogEntry]
	pickStore *PickResultStore

	requestListeners []func(domain.RequestLogEntry)
	fetchListeners   []func(domain.FetchLogEntry)
	pickListeners    []func(publishedID string)
}

// NewDashboardUseCase は新しい DashboardUseCase を作る。
func NewDashboardUseCase(pickStore *PickResultStore) *DashboardUseCase {
	return &DashboardUseCase{
		requests:  NewRingBuffer[domain.RequestLogEntry](dashboardRequestsCap),
		fetches:   NewRingBuffer[domain.FetchLogEntry](dashboardFetchesCap),
		pickStore: pickStore,
	}
}

// AppendRequest はリクエスト履歴を 1 件追加する。
func (d *DashboardUseCase) AppendRequest(e domain.RequestLogEntry) {
	d.requests.Append(e)
	for _, fn := range d.requestListeners {
		fn(e)
	}
}

// AppendFetch はソース表更新履歴を 1 件追加する。
func (d *DashboardUseCase) AppendFetch(e domain.FetchLogEntry) {
	d.fetches.Append(e)
	for _, fn := range d.fetchListeners {
		fn(e)
	}
}

// NotifyPickChanged は PickResultStore の OnChange から呼ばれる。
// イベント転送のみ行う (実体は PickResultStore が持つ)。
func (d *DashboardUseCase) NotifyPickChanged(publishedID string) {
	for _, fn := range d.pickListeners {
		fn(publishedID)
	}
}

// OnRequest は AppendRequest のたびに呼ばれるリスナーを登録する。
func (d *DashboardUseCase) OnRequest(fn func(domain.RequestLogEntry)) {
	d.requestListeners = append(d.requestListeners, fn)
}

// OnFetch は AppendFetch のたびに呼ばれるリスナーを登録する。
func (d *DashboardUseCase) OnFetch(fn func(domain.FetchLogEntry)) {
	d.fetchListeners = append(d.fetchListeners, fn)
}

// OnPickChanged は NotifyPickChanged 経由で発火するリスナーを登録する。
func (d *DashboardUseCase) OnPickChanged(fn func(publishedID string)) {
	d.pickListeners = append(d.pickListeners, fn)
}

// Snapshot は現在の全データを返す。
func (d *DashboardUseCase) Snapshot() domain.DashboardSnapshot {
	picks := d.snapshotPicks()
	return domain.DashboardSnapshot{
		Requests: d.requests.Snapshot(),
		Fetches:  d.fetches.Snapshot(),
		Picks:    picks,
	}
}

func (d *DashboardUseCase) snapshotPicks() []domain.PickSnapshotEntry {
	raw := d.pickStore.Snapshot()
	out := make([]domain.PickSnapshotEntry, 0, len(raw))
	for id, r := range raw {
		counts := map[string]int{}
		for _, c := range r.Charts {
			counts[c.PublicLevel]++
		}
		out = append(out, domain.PickSnapshotEntry{
			PublishedID: id,
			GeneratedAt: r.GeneratedAt,
			LevelOrder:  r.LevelOrder,
			LevelCounts: counts,
			TotalCount:  len(r.Charts),
		})
	}
	return out
}
