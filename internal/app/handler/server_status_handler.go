package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// ServerStatusDTO はフロントエンドに返す JSON 構造体。
type ServerStatusDTO struct {
	State     string `json:"state"`
	Port      int    `json:"port"`
	StartedAt string `json:"startedAt"`
	LastError string `json:"lastError"`
}

// ServerStatusHandler は Wails Bind 経由で HTTP サーバ操作 API を公開する。
type ServerStatusHandler struct {
	uc  *usecase.ServerUseCase
	ctx context.Context
}

// NewServerStatusHandler は新しい ServerStatusHandler を作る。
func NewServerStatusHandler(uc *usecase.ServerUseCase) *ServerStatusHandler {
	return &ServerStatusHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *ServerStatusHandler) SetContext(ctx context.Context) { h.ctx = ctx }

func toServerStatusDTO(s domain.ServerStatus) ServerStatusDTO {
	out := ServerStatusDTO{
		State: string(s.State), Port: s.Port, LastError: s.LastError,
	}
	if s.StartedAt != nil {
		out.StartedAt = s.StartedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// GetServerStatus は現在のサーバステータスを返す。
func (h *ServerStatusHandler) GetServerStatus() ServerStatusDTO {
	return toServerStatusDTO(h.uc.Status())
}

// StartServer は HTTP サーバを起動する。
func (h *ServerStatusHandler) StartServer() error {
	return h.uc.Start(h.ctx)
}

// StopServer は HTTP サーバを停止する。
func (h *ServerStatusHandler) StopServer() error {
	return h.uc.Stop(h.ctx)
}

// RestartServer は HTTP サーバを再起動する（停止 → 起動）。
func (h *ServerStatusHandler) RestartServer() error {
	return h.uc.Restart(h.ctx)
}
