//go:build darwin

// Package tray の macOS 向けスタブ実装。
//
// macOS では Cocoa/AppKit のメインスレッド要件と Wails の衝突 (SIGTRAP) を回避するため、
// トレイ常駐を提供しない。tray.New / Run は no-op で、main.go 側は IsRunning() で判定して
// ウィンドウクローズ時の挙動を切り替える（macOS は通常クローズで終了）。
//
// 本格的な macOS トレイ対応は将来 NSStatusBar を直接統合する形で再検討する。
package tray

// State はトレイアイコン色の切替対象となる状態（macOS では使用されない）。
type State int

const (
	StateIdle State = iota
	StateRunning
	StateError
)

// Callbacks は GUI 側へイベントを通知するコールバック群（macOS では使用されない）。
type Callbacks struct {
	OnShowSettings func()
	OnQuit         func()
}

// Tray は macOS 用 no-op スタブ。
type Tray struct{}

// New は no-op の Tray を返す。cb は無視される。
func New(_ Callbacks) *Tray {
	return &Tray{}
}

// Run は no-op。即座に return する。
func (t *Tray) Run(onReady func()) {
	if onReady != nil {
		onReady()
	}
}

// SetState は no-op。
func (t *Tray) SetState(_ State) {}

// Quit は no-op。
func (t *Tray) Quit() {}

// IsRunning は常に false を返す（macOS ではトレイ無効）。
func (t *Tray) IsRunning() bool {
	return false
}
