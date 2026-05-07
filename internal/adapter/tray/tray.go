// Package tray は getlantern/systray を使ったシステムトレイ常駐機能を提供する。
package tray

import (
	"sync"

	"github.com/getlantern/systray"
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
}

// New は Tray を作る。Run で実際に起動する。
func New(cb Callbacks) *Tray {
	return &Tray{
		cb:      cb,
		state:   StateIdle,
		tooltip: "BMS Random Table Compositor",
	}
}

// Run は systray のメインループを開始する。**呼び出しスレッドはメインスレッドで実行する必要がある**。
// onReady はトレイがUIに登録された後で呼ばれる。
func (t *Tray) Run(onReady func()) {
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
		// onExit
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
