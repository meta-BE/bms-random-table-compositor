package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// SongdataAttachStatusDTO は GetSongdataAttachStatus が返す DTO。
type SongdataAttachStatusDTO struct {
	Attached   bool   `json:"attached"`
	Path       string `json:"path"`
	SongCount  int    `json:"songCount"`
	AttachedAt string `json:"attachedAt"`
	LastError  string `json:"lastError"`
}

// SongdataHandler は Wails Bind 経由で songdata.db の ATTACH 状態と再アタッチ API を公開する。
type SongdataHandler struct {
	attacher *persistence.SongdataAttacher
	configUC *usecase.ConfigUseCase
	pickUC   *usecase.PickUseCase
	ctx      context.Context
}

// NewSongdataHandler は新しい SongdataHandler を作る。
func NewSongdataHandler(
	attacher *persistence.SongdataAttacher,
	configUC *usecase.ConfigUseCase,
	pickUC *usecase.PickUseCase,
) *SongdataHandler {
	return &SongdataHandler{attacher: attacher, configUC: configUC, pickUC: pickUC, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *SongdataHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// GetSongdataAttachStatus は現在のアタッチ状態を返す。
func (h *SongdataHandler) GetSongdataAttachStatus() SongdataAttachStatusDTO {
	st := h.attacher.Status()
	out := SongdataAttachStatusDTO{
		Attached:  st.Attached,
		Path:      st.Path,
		SongCount: st.SongCount,
		LastError: st.LastError,
	}
	if st.AttachedAt != nil {
		out.AttachedAt = st.AttachedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ReattachSongdata は現在の songdata_db_path 設定で ATTACH をやり直す。
// GUI の「再アタッチ」ボタンから呼ばれる。
func (h *SongdataHandler) ReattachSongdata() error {
	path, err := h.configUC.GetSongdataDBPath(h.ctx)
	if err != nil {
		return err
	}
	if err := h.attacher.ReAttach(h.ctx, path); err != nil {
		return err
	}
	h.pickUC.InvalidateAll()
	return nil
}
