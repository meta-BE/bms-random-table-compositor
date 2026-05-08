package domain

import "time"

// RequestLogEntry はダッシュボードに表示する 1 件の HTTP リクエスト履歴。
type RequestLogEntry struct {
	At         time.Time
	Method     string
	Path       string
	Slug       string // パースできれば slug、できなければ空
	StatusCode int
	DurationMs int64
}

// FetchLogEntry はダッシュボードに表示する 1 件のソース表取得履歴。
type FetchLogEntry struct {
	At          time.Time
	SourceID    string
	DisplayName string
	Status      FetchStatus
	Error       string
}

// PickSnapshotEntry はダッシュボードに表示するピック結果サマリ 1 件。
type PickSnapshotEntry struct {
	PublishedID string
	GeneratedAt time.Time
	LevelOrder  []string
	LevelCounts map[string]int
	TotalCount  int
}

// DashboardSnapshot は DashboardUseCase.Snapshot が返す全データ。
type DashboardSnapshot struct {
	Requests []RequestLogEntry
	Fetches  []FetchLogEntry
	Picks    []PickSnapshotEntry
}
