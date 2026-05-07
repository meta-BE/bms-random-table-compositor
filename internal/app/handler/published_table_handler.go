package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PublishedTableDTO はフロントエンドに返す JSON 構造体。
type PublishedTableDTO struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
	SortOrder     int    `json:"sortOrder"`
}

// CreatePublishedTableRequest は CreatePublishedTable のリクエスト DTO。
type CreatePublishedTableRequest struct {
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
}

// UpdatePublishedTableRequest は UpdatePublishedTable のリクエスト DTO。
type UpdatePublishedTableRequest struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
	SortOrder     int    `json:"sortOrder"`
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

func toPublishedTableDTO(t domain.PublishedTable) PublishedTableDTO {
	return PublishedTableDTO{
		ID: t.ID, Slug: t.Slug, DisplayName: t.DisplayName, Symbol: t.Symbol,
		SourceTableID: t.SourceTableID, OwnedOnly: t.OwnedOnly,
		PickPerLevel: t.Pick.PerLevel, RefreshMode: string(t.Pick.RefreshMode),
		SortOrder: t.SortOrder,
	}
}

// ListPublishedTables は登録済み公開表をすべて返す。
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

// CreatePublishedTable は新規公開表を作成し、ID を返す。
func (h *PublishedTableHandler) CreatePublishedTable(req CreatePublishedTableRequest) (string, error) {
	return h.uc.Create(h.ctx, usecase.CreatePublishedTableInput{
		Slug: req.Slug, DisplayName: req.DisplayName, Symbol: req.Symbol,
		SourceTableID: req.SourceTableID, OwnedOnly: req.OwnedOnly,
		PickPerLevel: req.PickPerLevel,
		RefreshMode:  domain.RefreshMode(req.RefreshMode),
	})
}

// UpdatePublishedTable は公開表を更新する。
func (h *PublishedTableHandler) UpdatePublishedTable(req UpdatePublishedTableRequest) error {
	return h.uc.Update(h.ctx, usecase.UpdatePublishedTableInput{
		ID: req.ID, Slug: req.Slug, DisplayName: req.DisplayName, Symbol: req.Symbol,
		SourceTableID: req.SourceTableID, OwnedOnly: req.OwnedOnly,
		PickPerLevel: req.PickPerLevel,
		RefreshMode:  domain.RefreshMode(req.RefreshMode),
		SortOrder:    req.SortOrder,
	})
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
