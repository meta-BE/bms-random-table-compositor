// Package domain は外部依存を持たない値オブジェクト群を提供する。
package domain

import "time"

// InputKind はソース表の入力 URL 種別。
type InputKind string

const (
	InputKindHTML       InputKind = "html"
	InputKindHeaderJSON InputKind = "header_json"
)

// FetchStatus はソース表の最後の取得結果。
type FetchStatus string

const (
	FetchStatusNever FetchStatus = "never"
	FetchStatusOK    FetchStatus = "ok"
	FetchStatusError FetchStatus = "error"
)

// SourceTable はユーザーが登録した難易度表のメタ情報。
type SourceTable struct {
	ID              string
	InputURL        string
	InputKind       InputKind
	DisplayName     string
	Name            string
	Symbol          string
	LevelOrder      []string
	DataURL         string
	ETag            string
	LastFetchedAt   *time.Time
	LastFetchStatus FetchStatus
	LastFetchError  string
}
