package main

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	"github.com/meta-BE/bms-random-table-compositor/internal/app"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App は Wails のメインアプリオブジェクト。
type App struct {
	ctx      context.Context
	services *app.Services
	tray     *tray.Tray
}

// NewApp は services を保持した App を作る。
// services は Bootstrap で構築済みのものを渡す。
func NewApp(services *app.Services) *App {
	return &App{services: services}
}

// startup は OnStartup で呼ばれる。ctx 保持と ConfigHandler への ctx 配布。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.services.ConfigHandler.SetContext(ctx)
	a.services.Logger.Info("wails startup")
}

// onBeforeClose はウィンドウクローズ前に呼ばれる。
// トレイが稼働中（Windows/Linux）はトレイ格納に変換、稼働していない（macOS）は通常クローズで終了。
func (a *App) onBeforeClose(ctx context.Context) bool {
	if a.tray != nil && a.tray.IsRunning() {
		wailsruntime.WindowHide(ctx)
		return true
	}
	return false
}

// shutdown はアプリ完全終了時に呼ばれる。
func (a *App) shutdown(ctx context.Context) {
	a.services.Logger.Info("wails shutdown")
}

// SetTray はトレイインスタンスを保持する（main から渡される）。
func (a *App) SetTray(t *tray.Tray) {
	a.tray = t
}

// ShowWindow はトレイメニューから呼ばれ、ウィンドウを再表示する。
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
	}
}

// Quit はトレイメニュー「終了」から呼ばれる。Wails ウィンドウを終了させる。
func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}
