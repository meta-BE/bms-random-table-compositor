package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// htmlPageData はテンプレに渡すデータ。
type htmlPageData struct {
	Slug         string
	DisplayName  string
	Symbol       string
	GeneratedAt  string
	TotalCount   int
	IsManualMode bool
	Levels       []htmlLevel
}

type htmlLevel struct {
	Level  string
	Charts []htmlChart
}

type htmlChart struct {
	Title  string
	Artist string
	MD5    string
	Owned  bool
}

// newHTMLHandler は GET /{slug} ハンドラ。
func newHTMLHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		ctx := r.Context()
		result, pub, err := deps.Pick.PickBySlug(ctx, slug)
		if err != nil {
			handleHTMLError(w, err)
			return
		}

		data := buildHTMLPageData(ctx, deps, pub, result)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := indexTemplate.Execute(w, data); err != nil {
			deps.Log.Error("html template execute failed", "slug", slug, "err", err)
		}
	}
}

// buildHTMLPageData はピック結果をテンプレ向けに整形する。
// PickBySlug は OwnedOnly=true 時に既に所持絞り込み済みなので、ここで再 fetch するのは
// 「未絞り込み」の場合に色分けするため。Plan 3 では Deps に owned cache を流していないので、
// MVP として OwnedOnly=true 時は全 owned、false 時は全 unowned 表示とする。
func buildHTMLPageData(ctx context.Context, deps Deps, pub domain.PublishedTable, r domain.PickResult) htmlPageData {
	ownedSet := map[string]struct{}{}

	levels := make([]htmlLevel, 0, len(r.LevelOrder))
	for _, level := range r.LevelOrder {
		var charts []htmlChart
		for _, c := range r.Charts {
			if c.Level != level {
				continue
			}
			_, owned := ownedSet[c.MD5]
			if pub.OwnedOnly {
				owned = true
			}
			charts = append(charts, htmlChart{
				Title: c.Title, Artist: c.Artist, MD5: c.MD5, Owned: owned,
			})
		}
		if len(charts) == 0 {
			continue
		}
		levels = append(levels, htmlLevel{Level: level, Charts: charts})
	}
	return htmlPageData{
		Slug:         pub.Slug,
		DisplayName:  pub.DisplayName,
		Symbol:       pub.Symbol,
		GeneratedAt:  r.GeneratedAt.Local().Format("2006-01-02 15:04:05 MST"),
		TotalCount:   len(r.Charts),
		IsManualMode: pub.Pick.RefreshMode == domain.RefreshModeManual,
		Levels:       levels,
	}
}

// handleHTMLError は usecase の sentinel error を HTTP ステータスに変換する。
func handleHTMLError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrPublishedTableNotFound):
		http.Error(w, "公開表が見つかりません", http.StatusNotFound)
	case errors.Is(err, usecase.ErrSourceNotFetched):
		http.Error(w, "ソース表が未取得です。設定画面から更新してください。", http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
