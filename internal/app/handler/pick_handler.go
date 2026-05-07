package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// PickHandler は Wails Bind 経由で手動再ピック API を公開する。
type PickHandler struct {
	uc  *usecase.PickUseCase
	ctx context.Context
}

// NewPickHandler は新しい PickHandler を作る。
func NewPickHandler(uc *usecase.PickUseCase) *PickHandler {
	return &PickHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *PickHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// ManualRefreshPick は指定 publishedID のピックを手動更新する。
func (h *PickHandler) ManualRefreshPick(publishedID string) error {
	return h.uc.ManualRefresh(h.ctx, publishedID)
}
