// Package singleinstance はファイルロックによるシングルインスタンス保証を提供する。
//
// 動作: 起動時に Acquire(path) を呼び、ロックファイルへ排他ロックを取る。
// 既にロックされていれば ErrAlreadyRunning を返す。
package singleinstance

import "errors"

// ErrAlreadyRunning は別プロセスが既にロックを保持している場合に返される。
var ErrAlreadyRunning = errors.New("別のインスタンスが既に実行中です")

// Lock はロック取得状態を表すハンドル。Release で解放する。
type Lock interface {
	Release() error
}
