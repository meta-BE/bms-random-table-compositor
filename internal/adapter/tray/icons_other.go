//go:build !darwin && !windows

// Linux 等の非 Windows / 非 darwin 向けアイコン定義 (PNG)。
// Windows は ICO が必要なため icons_windows.go で別途 embed する。

package tray

import _ "embed"

//go:embed icons/idle.png
var iconIdle []byte

//go:embed icons/running.png
var iconRunning []byte

//go:embed icons/error.png
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
