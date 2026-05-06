package main

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App は Wails のメインアプリオブジェクト。フロントエンドからBind経由で呼ばれる。
type App struct {
	ctx    context.Context
	server *Server
}

// NewApp は新しい App インスタンスを作る。
func NewApp() *App {
	return &App{server: NewServer()}
}

// startup は Wails の OnStartup で呼ばれる。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown は Wails の OnShutdown で呼ばれる。サーバを停止する。
func (a *App) shutdown(ctx context.Context) {
	c, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	_ = a.server.Stop(c)
}

// === フロントエンドにBindされるメソッド ===

// GetConfig は現在の設定を返す。
func (a *App) GetConfig() (Config, error) {
	return LoadConfig()
}

// Status は現在のサーバ状態を返す。
type Status struct {
	Running bool `json:"running"`
	Port    int  `json:"port"`
}

func (a *App) GetStatus() Status {
	running, port := a.server.Running()
	return Status{Running: running, Port: port}
}

// SaveAndStart は新しいポートを保存し、現在のサーバを停止してから新ポートで再起動する。
// エラー（保存失敗 or ポート確保失敗）はそのままフロントに返す。
// Stop がタイムアウトした場合は Close で強制終了済みのため、エラーを無視して Start に進む。
func (a *App) SaveAndStart(port int) error {
	if port < 1 || port > 65535 {
		return errPortRange{}
	}
	if err := SaveConfig(Config{Port: port}); err != nil {
		return err
	}
	c, cancel := context.WithTimeout(a.ctx, 1*time.Second)
	defer cancel()
	_ = a.server.Stop(c)
	return a.server.Start(port)
}

// Stop はサーバを停止する。
func (a *App) Stop() error {
	c, cancel := context.WithTimeout(a.ctx, 1*time.Second)
	defer cancel()
	return a.server.Stop(c)
}

// OpenURL は外部ブラウザで指定URLを開く。
// Wails の webview 内では target="_blank" が機能しないため、ランタイム経由で開く。
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

type errPortRange struct{}

func (errPortRange) Error() string { return "ポート番号は 1〜65535 の範囲で指定してください" }
