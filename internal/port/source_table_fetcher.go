package port

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// FetchedTable は SourceTableFetcher が返す取得結果。
// NotModified=true の場合 Header / Charts / ETag は意味を持たない（呼び出し側で破棄）。
type FetchedTable struct {
	Header      domain.BMSTableHeader
	Charts      []domain.SourceChart
	ETag        string
	NotModified bool
}

// SourceTableFetcher は外部 URL から難易度表を取得する。
type SourceTableFetcher interface {
	// FetchByHTML は HTML ページから <meta name="bmstable"> を抽出し、
	// header.json → data.json を順に取得する。
	FetchByHTML(ctx context.Context, htmlURL string, etag string) (FetchedTable, error)
	// FetchByHeader は header.json URL 直接指定で取得する。
	FetchByHeader(ctx context.Context, headerURL string, etag string) (FetchedTable, error)
}
