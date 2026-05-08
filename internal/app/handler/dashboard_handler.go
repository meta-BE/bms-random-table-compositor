package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// RequestLogDTO は JSON シリアライズ用のリクエストログ。
type RequestLogDTO struct {
	At         string `json:"at"` // RFC3339 (UTC)
	Method     string `json:"method"`
	Path       string `json:"path"`
	Slug       string `json:"slug"`
	StatusCode int    `json:"statusCode"`
	DurationMs int64  `json:"durationMs"`
}

// FetchLogDTO は JSON シリアライズ用のソース表更新ログ。
type FetchLogDTO struct {
	At          string `json:"at"`
	SourceID    string `json:"sourceId"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	Error       string `json:"error"`
}

// PickSnapshotDTO は JSON シリアライズ用のピック結果サマリ。
type PickSnapshotDTO struct {
	PublishedID string         `json:"publishedId"`
	GeneratedAt string         `json:"generatedAt"`
	LevelOrder  []string       `json:"levelOrder"`
	LevelCounts map[string]int `json:"levelCounts"`
	TotalCount  int            `json:"totalCount"`
}

// DashboardSnapshotDTO は Snapshot が返す JSON 構造体。
type DashboardSnapshotDTO struct {
	Requests []RequestLogDTO   `json:"requests"`
	Fetches  []FetchLogDTO     `json:"fetches"`
	Picks    []PickSnapshotDTO `json:"picks"`
}

// DashboardHandler は Wails Bind 経由でフロントエンドから呼ばれる。
type DashboardHandler struct {
	uc  *usecase.DashboardUseCase
	ctx context.Context
}

func NewDashboardHandler(uc *usecase.DashboardUseCase) *DashboardHandler {
	return &DashboardHandler{uc: uc, ctx: context.Background()}
}

func (h *DashboardHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// Snapshot は現在のダッシュボードデータを返す。
func (h *DashboardHandler) Snapshot() (DashboardSnapshotDTO, error) {
	s := h.uc.Snapshot()
	out := DashboardSnapshotDTO{
		Requests: make([]RequestLogDTO, 0, len(s.Requests)),
		Fetches:  make([]FetchLogDTO, 0, len(s.Fetches)),
		Picks:    make([]PickSnapshotDTO, 0, len(s.Picks)),
	}
	for _, r := range s.Requests {
		out.Requests = append(out.Requests, RequestLogDTO{
			At:         r.At.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Method:     r.Method,
			Path:       r.Path,
			Slug:       r.Slug,
			StatusCode: r.StatusCode,
			DurationMs: r.DurationMs,
		})
	}
	for _, f := range s.Fetches {
		out.Fetches = append(out.Fetches, FetchLogDTO{
			At:          f.At.UTC().Format("2006-01-02T15:04:05Z07:00"),
			SourceID:    f.SourceID,
			DisplayName: f.DisplayName,
			Status:      string(f.Status),
			Error:       f.Error,
		})
	}
	for _, p := range s.Picks {
		out.Picks = append(out.Picks, PickSnapshotDTO{
			PublishedID: p.PublishedID,
			GeneratedAt: p.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			LevelOrder:  p.LevelOrder,
			LevelCounts: p.LevelCounts,
			TotalCount:  p.TotalCount,
		})
	}
	return out, nil
}
