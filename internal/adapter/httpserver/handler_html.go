package httpserver

import (
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

// htmlChart は1曲分の表示用フィールド。
// Level は Symbol+Level を結合済みの文字列 (例: "sl0", "⭐3")。
// LR2IRURL/URL/URLDiff は空文字列のとき該当リンクを描画しない。
type htmlChart struct {
	Level    string
	Title    string
	Artist   string
	LR2IRURL string
	URL      string
	URLDiff  string
	Owned    bool
}

const lr2irRankingURLPrefix = "http://www.dream-pro.info/~lavalse/LR2IR/search.cgi?mode=ranking&bmsmd5="

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

		data := buildHTMLPageData(pub, result)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := indexTemplate.Execute(w, data); err != nil {
			deps.Log.Error("html template execute failed", "slug", slug, "err", err)
		}
	}
}

// buildHTMLPageData はピック結果をテンプレ向けに整形する。
// 各譜面の所持状態は EnrichedChart.IsOwned から読む。OwnedOnly 公開表は全件 owned 扱い。
func buildHTMLPageData(pub domain.PublishedTable, r domain.PickResult) htmlPageData {
	levels := make([]htmlLevel, 0, len(r.LevelOrder))
	for _, level := range r.LevelOrder {
		var charts []htmlChart
		for _, c := range r.Charts {
			if c.Level != level {
				continue
			}
			owned := c.IsOwned
			if pub.OwnedOnly {
				owned = true
			}
			// 行頭セルは譜面単位 symbol (v2 で複数ソース合成時に区別するため)。
			// SourceChart.Symbol が空の場合 (fetcher 経由構築など) は pub.Symbol で代替。
			symbol := c.Symbol
			if symbol == "" {
				symbol = pub.Symbol
			}
			charts = append(charts, htmlChart{
				Level:    symbol + level,
				Title:    c.Title,
				Artist:   c.Artist,
				LR2IRURL: lr2irURL(c.MD5),
				URL:      rawString(c.Raw, "url"),
				URLDiff:  rawString(c.Raw, "url_diff"),
				Owned:    owned,
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

// lr2irURL は md5 が非空のときのみ LR2IR ranking URL を返す。
// md5 は16進固定なので URL エスケープ不要。
func lr2irURL(md5 string) string {
	if md5 == "" {
		return ""
	}
	return lr2irRankingURLPrefix + md5
}

// rawString は data.json パススルーフィールドから安全に文字列を取り出す。
// キー欠如・型不一致・nil はすべて "" を返す。
func rawString(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
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
