package httpserver

import (
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// newRefreshHandler は POST /{slug}/_refresh ハンドラ。
// manual モードのみ受け付け、再ピック後 GET /{slug} へ 303 リダイレクトする。
func newRefreshHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		_, pub, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}
		if pub.Pick.RefreshMode != domain.RefreshModeManual {
			http.Error(w, "manual モード以外では再ピック不可", http.StatusMethodNotAllowed)
			return
		}
		if err := deps.Pick.ManualRefresh(r.Context(), pub.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/"+pub.Slug, http.StatusSeeOther)
	}
}
