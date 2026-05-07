//go:build !darwin

// Package tray は getlantern/systray (互換: fyne.io/systray) を使ったシステムトレイ常駐機能を提供する。
//
// macOS では Cocoa/AppKit のメインスレッド要件と Wails が衝突するため、tray は darwin 向けに
// no-op スタブが提供される（tray_darwin.go 参照）。Windows / Linux では本ファイルの実装が使われる。
package tray

import (
	"sync"

	"fyne.io/systray"
)

// State はトレイアイコン色の切替対象となる状態。
type State int

const (
	// StateIdle はサーバ未起動 (Plan 1 の初期状態)。
	StateIdle State = iota
	// StateRunning はサーバが正常稼働中 (Plan 3 以降で使用)。
	StateRunning
	// StateError はサーバ起動失敗等のエラー状態。
	StateError
)

// Callbacks は GUI 側へイベントを通知するコールバック群。
type Callbacks struct {
	OnShowSettings func()
	OnQuit         func()
}

// Tray はシステムトレイを管理する。Run で起動、SetState で状態変更、Quit で停止。
type Tray struct {
	cb      Callbacks
	mu      sync.Mutex
	state   State
	mShow   *systray.MenuItem
	mQuit   *systray.MenuItem
	tooltip string
	running bool
}

// New は Tray を作る。Run で実際に起動する。
func New(cb Callbacks) *Tray {
	return &Tray{
		cb:      cb,
		state:   StateIdle,
		tooltip: "BMS Random Table Compositor",
	}
}

// Run は systray のメインループを開始する。Windows / Linux ではブロッキング。
// onReady はトレイがUIに登録された後で呼ばれる。
func (t *Tray) Run(onReady func()) {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	systray.Run(func() {
		systray.SetTooltip(t.tooltip)
		systray.SetIcon(IconFor(t.state))

		t.mShow = systray.AddMenuItem("設定を開く", "メインウィンドウを表示")
		systray.AddSeparator()
		t.mQuit = systray.AddMenuItem("終了", "アプリケーションを終了")

		go t.handleClicks()

		if onReady != nil {
			onReady()
		}
	}, func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	})
}

func (t *Tray) handleClicks() {
	for {
		select {
		case <-t.mShow.ClickedCh:
			if t.cb.OnShowSettings != nil {
				t.cb.OnShowSettings()
			}
		case <-t.mQuit.ClickedCh:
			if t.cb.OnQuit != nil {
				t.cb.OnQuit()
			}
			systray.Quit()
			return
		}
	}
}

// SetState はトレイアイコンの状態を切り替える。
func (t *Tray) SetState(s State) {
	t.mu.Lock()
	t.state = s
	t.mu.Unlock()
	systray.SetIcon(IconFor(s))
}

// Quit はトレイメインループを停止する。
func (t *Tray) Quit() {
	systray.Quit()
}

// IsRunning は現在トレイが稼働中かを返す（onBeforeClose の挙動分岐に使用）。
func (t *Tray) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}
