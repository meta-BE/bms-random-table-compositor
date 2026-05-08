package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	appinternal "github.com/meta-BE/bms-random-table-compositor/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version はビルド時に -ldflags "-X main.version=..." で注入される。
// 未注入の場合 (例: wails dev) は "dev" のまま。
var version = "dev"

func main() {
	services, err := appinternal.Bootstrap()
	if err != nil {
		fmt.Fprintf(os.Stderr, "起動エラー: %v\n", err)
		os.Exit(1)
	}
	defer services.Close()

	myApp := NewApp(services)

	tr := tray.New(tray.Callbacks{
		OnShowSettings: myApp.ShowWindow,
		OnQuit:         myApp.Quit,
	})
	myApp.SetTray(tr)

	go tr.Run(nil)

	if err := wails.Run(&options.App{
		Title:  "BMS Random Table Compositor " + version,
		Width:  900,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "bms-random-table-compositor.meta-BE.io",
			OnSecondInstanceLaunch: myApp.onSecondInstance,
		},
		OnStartup:     myApp.startup,
		OnBeforeClose: myApp.onBeforeClose,
		OnShutdown:    myApp.shutdown,
		Bind: []any{
			myApp,
			services.ConfigHandler,
			services.SourceTableHandler,
			services.PublishedTableHandler,
			services.PickHandler,
			services.ServerStatusHandler,
			services.SongdataHandler,
			services.DashboardHandler,
		},
	}); err != nil {
		services.Logger.Error("wails run failed", "err", err)
		fmt.Fprintf(os.Stderr, "Wails Error: %v\n", err)
		os.Exit(1)
	}

	tr.Quit()
}
