package main

import (
	"context"
	"os"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	"github.com/meta-BE/bms-random-table-compositor/internal/app"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App は Wails のメインアプリオブジェクト。
type App struct {
	ctx      context.Context
	services *app.Services
	tray     *tray.Tray
}

// NewApp は services を保持した App を作る。
func NewApp(services *app.Services) *App {
	return &App{services: services}
}

// startup は OnStartup で呼ばれる。ハンドラに ctx を引き渡し、ソース表の
// バックグラウンド更新と HTTP サーバの自動起動を行う。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.services.ConfigHandler.SetContext(ctx)
	a.services.SourceTableHandler.SetContext(ctx)
	a.services.PublishedTableHandler.SetContext(ctx)
	a.services.PickHandler.SetContext(ctx)
	a.services.ServerStatusHandler.SetContext(ctx)
	a.services.OwnedChartHandler.SetContext(ctx)
	a.services.DashboardHandler.SetContext(ctx)
	a.services.SongdataHandler.SetContext(ctx)
	a.services.Logger.Info("wails startup")

	// ServerStatus 変化を Wails event 経由でフロントへ流す + トレイ状態を同期
	a.services.ServerUseCase.OnStatusChange(func(s domain.ServerStatus) {
		wailsruntime.EventsEmit(ctx, "server_status:changed", s)
		if a.tray != nil && a.tray.IsRunning() {
			switch s.State {
			case domain.ServerStateRunning:
				a.tray.SetState(tray.StateRunning)
			case domain.ServerStateError:
				a.tray.SetState(tray.StateError)
			default:
				a.tray.SetState(tray.StateIdle)
			}
		}
	})

	// ダッシュボード event 配信
	a.services.DashboardUseCase.OnRequest(func(e domain.RequestLogEntry) {
		wailsruntime.EventsEmit(ctx, "dashboard:request_logged", e)
	})
	a.services.DashboardUseCase.OnFetch(func(e domain.FetchLogEntry) {
		wailsruntime.EventsEmit(ctx, "dashboard:fetch_logged", e)
	})
	a.services.DashboardUseCase.OnPickChanged(func(publishedID string) {
		wailsruntime.EventsEmit(ctx, "dashboard:pick_changed", publishedID)
	})

	// 起動時のソース表バックグラウンド更新
	go func() {
		a.services.Logger.Info("startup refresh all begin")
		if err := a.services.SourceTableUseCase.RefreshAll(ctx); err != nil {
			a.services.Logger.Warn("startup refresh all failed", "err", err)
		}
		a.services.Logger.Info("startup refresh all done")
		wailsruntime.EventsEmit(ctx, "source_table:refresh_all_done")
	}()

	// HTTP サーバ自動起動
	go func() {
		if err := a.services.ServerUseCase.Start(ctx); err != nil {
			a.services.Logger.Warn("auto-start http server failed", "err", err)
		}
	}()
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

// onSecondInstance は二重起動を検知した時に Wails から呼ばれる。
// 既存インスタンスのウィンドウを前面化する。
func (a *App) onSecondInstance(_ options.SecondInstanceData) {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
		wailsruntime.Show(a.ctx)
	}
}

// SetTray はトレイインスタンスを保持する。
func (a *App) SetTray(t *tray.Tray) {
	a.tray = t
}

// ShowWindow はトレイメニュー「設定を開く」から呼ばれ、ウィンドウを再表示する。
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
	}
}

// Quit はトレイメニュー「終了」から呼ばれる。
// wailsruntime.Quit はトレイ goroutine から呼ぶと Windows で正常に効かないことがあるため、
// services.Close() でリソース解放してから os.Exit で確実にプロセス終了させる。
func (a *App) Quit() {
	if a.services != nil {
		a.services.Close()
	}
	os.Exit(0)
}
