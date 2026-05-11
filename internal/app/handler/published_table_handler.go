package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PublishedTableLevelDTO はフロントエンドに返す公開レベルの JSON 構造体。
type PublishedTableLevelDTO struct {
	ID             string                          `json:"id"`
	Name           string                          `json:"name"`
	SortOrder      int                             `json:"sortOrder"`
	PerMappingPick int                             `json:"perMappingPick"`
	TotalPick      int                             `json:"totalPick"`
	Mappings       []PublishedTableLevelMappingDTO `json:"mappings"`
}

// PublishedTableLevelMappingDTO はフロントエンドに返す公開レベル内マッピングの JSON 構造体。
type PublishedTableLevelMappingDTO struct {
	ID            string `json:"id"`
	SourceTableID string `json:"sourceTableId"`
	SourceLevel   string `json:"sourceLevel"`
	SortOrder     int    `json:"sortOrder"`
}

// PublishedTableDTO はフロントエンドに返す公開表本体の JSON 構造体。
// Levels は List では空配列、Get では実体込みで返す。
type PublishedTableDTO struct {
	ID          string                   `json:"id"`
	Slug        string                   `json:"slug"`
	DisplayName string                   `json:"displayName"`
	Symbol      string                   `json:"symbol"`
	OwnedOnly   bool                     `json:"ownedOnly"`
	RefreshMode string                   `json:"refreshMode"`
	SortOrder   int                      `json:"sortOrder"`
	Levels      []PublishedTableLevelDTO `json:"levels"`
}

// PublishedTableLevelInputDTO は Create / Update リクエストで受け取る公開レベル入力。
type PublishedTableLevelInputDTO struct {
	Name           string                               `json:"name"`
	PerMappingPick int                                  `json:"perMappingPick"`
	TotalPick      int                                  `json:"totalPick"`
	Mappings       []PublishedTableLevelMappingInputDTO `json:"mappings"`
}

// PublishedTableLevelMappingInputDTO は Create / Update リクエストで受け取るマッピング入力。
type PublishedTableLevelMappingInputDTO struct {
	SourceTableID string `json:"sourceTableId"`
	SourceLevel   string `json:"sourceLevel"`
}

// CreatePublishedTableRequest は CreatePublishedTable のリクエスト DTO。
type CreatePublishedTableRequest struct {
	Slug        string                        `json:"slug"`
	DisplayName string                        `json:"displayName"`
	Symbol      string                        `json:"symbol"`
	OwnedOnly   bool                          `json:"ownedOnly"`
	RefreshMode string                        `json:"refreshMode"`
	Levels      []PublishedTableLevelInputDTO `json:"levels"`
}

// UpdatePublishedTableRequest は UpdatePublishedTable のリクエスト DTO。
type UpdatePublishedTableRequest struct {
	ID          string                        `json:"id"`
	Slug        string                        `json:"slug"`
	DisplayName string                        `json:"displayName"`
	Symbol      string                        `json:"symbol"`
	OwnedOnly   bool                          `json:"ownedOnly"`
	RefreshMode string                        `json:"refreshMode"`
	SortOrder   int                           `json:"sortOrder"`
	Levels      []PublishedTableLevelInputDTO `json:"levels"`
}

// CreateFromSourceRequest は CreatePublishedTableFromSource のリクエスト DTO。
type CreateFromSourceRequest struct {
	SourceTableID string `json:"sourceTableId"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
}

// SlugValidationDTO は ValidateSlug の応答 DTO。
type SlugValidationDTO struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"` // "invalid_format" / "reserved" / "duplicate"
}

// PublishedTableHandler は Wails Bind 経由で公開表 API を公開する。
type PublishedTableHandler struct {
	uc  *usecase.PublishedTableUseCase
	ctx context.Context
}

// NewPublishedTableHandler は新しい PublishedTableHandler を作る。
func NewPublishedTableHandler(uc *usecase.PublishedTableUseCase) *PublishedTableHandler {
	return &PublishedTableHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *PublishedTableHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// toPublishedTableDTO は List 用の DTO 変換（Levels は空配列で返す）。
func toPublishedTableDTO(t domain.PublishedTable) PublishedTableDTO {
	return PublishedTableDTO{
		ID: t.ID, Slug: t.Slug, DisplayName: t.DisplayName, Symbol: t.Symbol,
		OwnedOnly:   t.OwnedOnly,
		RefreshMode: string(t.Pick.RefreshMode),
		SortOrder:   t.SortOrder,
		Levels:      []PublishedTableLevelDTO{},
	}
}

// toPublishedTableDTOWithLevels は Get 用の DTO 変換（Levels / Mappings 込み）。
func toPublishedTableDTOWithLevels(t domain.PublishedTable) PublishedTableDTO {
	out := toPublishedTableDTO(t)
	out.Levels = make([]PublishedTableLevelDTO, 0, len(t.Levels))
	for _, lv := range t.Levels {
		mappings := make([]PublishedTableLevelMappingDTO, 0, len(lv.Mappings))
		for _, mp := range lv.Mappings {
			mappings = append(mappings, PublishedTableLevelMappingDTO{
				ID:            mp.ID,
				SourceTableID: mp.SourceTableID,
				SourceLevel:   mp.SourceLevel,
				SortOrder:     mp.SortOrder,
			})
		}
		out.Levels = append(out.Levels, PublishedTableLevelDTO{
			ID:             lv.ID,
			Name:           lv.Name,
			SortOrder:      lv.SortOrder,
			PerMappingPick: lv.PerMappingPick,
			TotalPick:      lv.TotalPick,
			Mappings:       mappings,
		})
	}
	return out
}

// toLevelInputs は DTO 入力を usecase 入力へ変換する。
func toLevelInputs(in []PublishedTableLevelInputDTO) []usecase.PublishedTableLevelInput {
	out := make([]usecase.PublishedTableLevelInput, 0, len(in))
	for _, lv := range in {
		ms := make([]usecase.PublishedTableLevelMappingInput, 0, len(lv.Mappings))
		for _, mp := range lv.Mappings {
			ms = append(ms, usecase.PublishedTableLevelMappingInput{
				SourceTableID: mp.SourceTableID,
				SourceLevel:   mp.SourceLevel,
			})
		}
		out = append(out, usecase.PublishedTableLevelInput{
			Name:           lv.Name,
			PerMappingPick: lv.PerMappingPick,
			TotalPick:      lv.TotalPick,
			Mappings:       ms,
		})
	}
	return out
}

// ListPublishedTables は登録済み公開表をすべて返す（Levels は空配列）。
func (h *PublishedTableHandler) ListPublishedTables() ([]PublishedTableDTO, error) {
	list, err := h.uc.List(h.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublishedTableDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toPublishedTableDTO(t))
	}
	return out, nil
}

// GetPublishedTable は ID 指定で公開表を返す（Levels / Mappings 込み）。
func (h *PublishedTableHandler) GetPublishedTable(id string) (PublishedTableDTO, error) {
	t, err := h.uc.Get(h.ctx, id)
	if err != nil {
		return PublishedTableDTO{}, err
	}
	return toPublishedTableDTOWithLevels(t), nil
}

// CreatePublishedTable は新規公開表を作成し、ID を返す。
func (h *PublishedTableHandler) CreatePublishedTable(req CreatePublishedTableRequest) (string, error) {
	in := usecase.CreatePublishedTableInput{
		Slug:        req.Slug,
		DisplayName: req.DisplayName,
		Symbol:      req.Symbol,
		OwnedOnly:   req.OwnedOnly,
		RefreshMode: domain.RefreshMode(req.RefreshMode),
		Levels:      toLevelInputs(req.Levels),
	}
	return h.uc.Create(h.ctx, in)
}

// UpdatePublishedTable は公開表を更新する。
func (h *PublishedTableHandler) UpdatePublishedTable(req UpdatePublishedTableRequest) error {
	in := usecase.UpdatePublishedTableInput{
		ID:          req.ID,
		Slug:        req.Slug,
		DisplayName: req.DisplayName,
		Symbol:      req.Symbol,
		OwnedOnly:   req.OwnedOnly,
		RefreshMode: domain.RefreshMode(req.RefreshMode),
		SortOrder:   req.SortOrder,
		Levels:      toLevelInputs(req.Levels),
	}
	return h.uc.Update(h.ctx, in)
}

// CreatePublishedTableFromSource はソース表 1 件をテンプレに、各レベル 1:1 マッピングで公開表を作る。
func (h *PublishedTableHandler) CreatePublishedTableFromSource(req CreateFromSourceRequest) (string, error) {
	return h.uc.CreateFromSourceTable(h.ctx, req.SourceTableID, req.Slug, req.DisplayName, req.Symbol)
}

// DeletePublishedTable は公開表を削除する。
func (h *PublishedTableHandler) DeletePublishedTable(id string) error {
	return h.uc.Delete(h.ctx, id)
}

// ValidateSlug は slug 形式 / 予約語 / 重複を検査する。GUI のリアルタイム判定用。
func (h *PublishedTableHandler) ValidateSlug(slug string, excludeID string) SlugValidationDTO {
	err := h.uc.ValidateSlug(h.ctx, slug, excludeID)
	switch {
	case err == nil:
		return SlugValidationDTO{OK: true}
	case errors.Is(err, usecase.ErrSlugInvalidFormat):
		return SlugValidationDTO{OK: false, Reason: "invalid_format"}
	case errors.Is(err, usecase.ErrSlugReserved):
		return SlugValidationDTO{OK: false, Reason: "reserved"}
	case errors.Is(err, usecase.ErrSlugDuplicated):
		return SlugValidationDTO{OK: false, Reason: "duplicate"}
	default:
		return SlugValidationDTO{OK: false, Reason: err.Error()}
	}
}

// SuggestSlugFromSource はソース表名から slug を生成する。
func (h *PublishedTableHandler) SuggestSlugFromSource(sourceID string) (string, error) {
	return h.uc.SuggestSlugFromSource(h.ctx, sourceID)
}

// OpenPublishedTableURL はブラウザで http://127.0.0.1:<port>/<slug> を開く。
func (h *PublishedTableHandler) OpenPublishedTableURL(slug string, port int) error {
	if h.ctx == nil {
		return errors.New("context が未初期化です")
	}
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/" + slug
	wailsruntime.BrowserOpenURL(h.ctx, url)
	return nil
}
