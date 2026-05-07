package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// OwnedCacheStatusDTO は GetOwnedCacheStatus が返す DTO。
type OwnedCacheStatusDTO struct {
	Loaded     bool   `json:"loaded"`
	Count      int    `json:"count"`
	LoadedAt   string `json:"loadedAt"`
	LoadedPath string `json:"loadedPath"`
	LastError  string `json:"lastError"`
}

// OwnedChartHandler は Wails Bind 経由で所持キャッシュの状態と再読み込み API を公開する。
type OwnedChartHandler struct {
	cache *usecase.OwnedMD5Cache
	ctx   context.Context
}

// NewOwnedChartHandler は新しい OwnedChartHandler を作る。
func NewOwnedChartHandler(cache *usecase.OwnedMD5Cache) *OwnedChartHandler {
	return &OwnedChartHandler{cache: cache, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *OwnedChartHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// GetOwnedCacheStatus は現在の所持キャッシュ状態を返す。
func (h *OwnedChartHandler) GetOwnedCacheStatus() OwnedCacheStatusDTO {
	st := h.cache.Status()
	out := OwnedCacheStatusDTO{
		Loaded: st.Loaded, Count: st.Count,
		LoadedPath: st.LoadedPath, LastError: st.LastError,
	}
	if st.LoadedAt != nil {
		out.LoadedAt = st.LoadedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ReloadOwnedCache は songdata.db を再読み込みする。
func (h *OwnedChartHandler) ReloadOwnedCache() error {
	return h.cache.Reload(h.ctx)
}
