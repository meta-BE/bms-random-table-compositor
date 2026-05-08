//go:build windows

// Windows 向けアイコン定義。fyne.io/systray の Windows 実装は ICO 形式を期待するため、
// PNG ではなく ICO ファイル (PNG を内部に埋め込んだ形式) を embed する。

package tray

import _ "embed"

//go:embed icons/idle.ico
var iconIdle []byte

//go:embed icons/running.ico
var iconRunning []byte

//go:embed icons/error.ico
var iconError []byte

// IconFor は与えられた状態に対応するアイコンバイト列を返す。
func IconFor(state State) []byte {
	switch state {
	case StateRunning:
		return iconRunning
	case StateError:
		return iconError
	default:
		return iconIdle
	}
}
