package main

import (
	"embed"
	"errors"
	"fmt"
	"os"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/singleinstance"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	appinternal "github.com/meta-BE/bms-random-table-compositor/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	services, err := appinternal.Bootstrap()
	if err != nil {
		if errors.Is(err, singleinstance.ErrAlreadyRunning) {
			fmt.Fprintln(os.Stderr, "別のインスタンスが既に実行中です。設定を開きたい場合はトレイメニューから操作してください。")
			os.Exit(0)
		}
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

	// systray.Run はブロッキングで、メインスレッドを占有する。
	// Wails が main goroutine を使うので、systray を別 goroutine で起動する。
	// ※ POC で未検証の領域。実機で問題があれば再設計する。
	go tr.Run(nil)

	if err := wails.Run(&options.App{
		Title:  "BMS Random Table Compositor",
		Width:  900,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     myApp.startup,
		OnBeforeClose: myApp.onBeforeClose,
		OnShutdown:    myApp.shutdown,
		Bind: []any{
			myApp,
			services.ConfigHandler,
		},
	}); err != nil {
		services.Logger.Error("wails run failed", "err", err)
		fmt.Fprintf(os.Stderr, "Wails Error: %v\n", err)
		os.Exit(1)
	}

	tr.Quit()
}
