package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// newDataHandler は GET /{slug}/data.json ハンドラ。
func newDataHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		result, _, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}

		entries := make([]map[string]any, 0, len(result.Charts))
		for _, c := range result.Charts {
			entries = append(entries, mergeChart(c))
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			deps.Log.Error("data.json encode failed", "slug", slug, "err", err)
		}
	}
}

// mergeChart は SourceChart.Raw をベースに level/md5/sha256/title/artist を上書きしてマップを返す。
// 表固有フィールド（url, url_diff, lr2_bmsid 等）はパススルーされる。
func mergeChart(c domain.SourceChart) map[string]any {
	out := make(map[string]any, len(c.Raw)+5)
	for k, v := range c.Raw {
		out[k] = v
	}
	out["md5"] = c.MD5
	if c.SHA256 != "" {
		out["sha256"] = c.SHA256
	}
	out["level"] = c.Level
	out["title"] = c.Title
	out["artist"] = c.Artist
	return out
}
