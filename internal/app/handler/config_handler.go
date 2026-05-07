package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ServerConfig は GetServerConfig が返す JSON 構造体。
type ServerConfig struct {
	Port           int    `json:"port"`
	SongdataDBPath string `json:"songdataDbPath"`
}

// ConfigHandler は Wails Bind 経由でフロントエンドから呼ばれる設定ハンドラ。
type ConfigHandler struct {
	uc  *usecase.ConfigUseCase
	ctx context.Context
}

// NewConfigHandler は ConfigHandler を作る。
func NewConfigHandler(uc *usecase.ConfigUseCase) *ConfigHandler {
	return &ConfigHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *ConfigHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// GetServerConfig は現在の設定値（ポート / songdata.db パス）を返す。
func (h *ConfigHandler) GetServerConfig() (ServerConfig, error) {
	port, err := h.uc.GetServerPort(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	dbPath, err := h.uc.GetSongdataDBPath(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	return ServerConfig{Port: port, SongdataDBPath: dbPath}, nil
}

// SetServerPort はサーバポート番号を保存する。範囲外はエラー。
func (h *ConfigHandler) SetServerPort(port int) error {
	return h.uc.SetServerPort(h.ctx, port)
}

// SetSongdataDBPath は beatoraja の songdata.db パスを保存する。
func (h *ConfigHandler) SetSongdataDBPath(path string) error {
	return h.uc.SetSongdataDBPath(h.ctx, path)
}

// PickSongdataDB はユーザーに songdata.db のパスを OS のファイル選択ダイアログで
// 選ばせ、選ばれた絶対パスを返す。キャンセル時は空文字を返す。
// SetContext 前 (Wails OnStartup 前) に呼ばれた場合はランタイム API を呼ばずに
// 空文字を返す (テスト用のセーフガード)。
func (h *ConfigHandler) PickSongdataDB() (string, error) {
	if h.ctx == nil || h.ctx == context.Background() {
		return "", nil
	}
	return wailsruntime.OpenFileDialog(h.ctx, wailsruntime.OpenDialogOptions{
		Title: "songdata.db を選択",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "SQLite データベース (*.db)", Pattern: "*.db"},
			{DisplayName: "すべてのファイル (*.*)", Pattern: "*"},
		},
	})
}
