package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// SourceTableHandler は Wails Bind 経由でソース表 API を公開する。
type SourceTableHandler struct {
	uc  *usecase.SourceTableUseCase
	ctx context.Context
}

// NewSourceTableHandler は新しい SourceTableHandler を作る。
func NewSourceTableHandler(uc *usecase.SourceTableUseCase) *SourceTableHandler {
	return &SourceTableHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *SourceTableHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// SourceTableDTO はフロントエンドに返す JSON 構造体。
type SourceTableDTO struct {
	ID              string   `json:"id"`
	InputURL        string   `json:"inputUrl"`
	InputKind       string   `json:"inputKind"`
	DisplayName     string   `json:"displayName"`
	Name            string   `json:"name"`
	Symbol          string   `json:"symbol"`
	LevelOrder      []string `json:"levelOrder"`
	DataURL         string   `json:"dataUrl"`
	LastFetchedAt   string   `json:"lastFetchedAt"`
	LastFetchStatus string   `json:"lastFetchStatus"`
	LastFetchError  string   `json:"lastFetchError"`
}

func toSourceTableDTO(st domain.SourceTable) SourceTableDTO {
	var lastFetchedAt string
	if st.LastFetchedAt != nil {
		lastFetchedAt = st.LastFetchedAt.UTC().Format(time.RFC3339)
	}
	levelOrder := st.LevelOrder
	if levelOrder == nil {
		levelOrder = []string{}
	}
	return SourceTableDTO{
		ID: st.ID, InputURL: st.InputURL, InputKind: string(st.InputKind),
		DisplayName: st.DisplayName, Name: st.Name, Symbol: st.Symbol,
		LevelOrder: levelOrder, DataURL: st.DataURL,
		LastFetchedAt:   lastFetchedAt,
		LastFetchStatus: string(st.LastFetchStatus),
		LastFetchError:  st.LastFetchError,
	}
}

// ListSourceTables は登録済みソース表をすべて返す。
func (h *SourceTableHandler) ListSourceTables() ([]SourceTableDTO, error) {
	list, err := h.uc.List(h.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceTableDTO, 0, len(list))
	for _, st := range list {
		out = append(out, toSourceTableDTO(st))
	}
	return out, nil
}

// AddSourceTableRequest は AddSourceTable のリクエスト DTO。
// InputKind は URL から自動判別、DisplayName は取得後に Name で
// フォールバック表示するため、入力は URL のみ。
type AddSourceTableRequest struct {
	URL string `json:"url"`
}

// AddSourceTable は新規ソース表を登録し、ID を返す（取得は別途 Refresh で行う）。
func (h *SourceTableHandler) AddSourceTable(req AddSourceTableRequest) (string, error) {
	return h.uc.Add(h.ctx, usecase.AddSourceTableInput{URL: req.URL})
}

// RefreshSourceTable は指定 ID のソース表を取得・保存する。
func (h *SourceTableHandler) RefreshSourceTable(id string) error {
	return h.uc.RefreshOne(h.ctx, id)
}

// RefreshAllSourceTables は登録済み全ソース表を並列度 4 で更新する。
func (h *SourceTableHandler) RefreshAllSourceTables() error {
	return h.uc.RefreshAll(h.ctx)
}

// DeleteSourceTable は指定 ID のソース表を削除する（譜面は CASCADE で消える）。
func (h *SourceTableHandler) DeleteSourceTable(id string) error {
	return h.uc.Remove(h.ctx, id)
}

// UpdateSourceTableDisplayName は表示名のみ書き換える。
func (h *SourceTableHandler) UpdateSourceTableDisplayName(id string, displayName string) error {
	return h.uc.UpdateDisplayName(h.ctx, id, displayName)
}
