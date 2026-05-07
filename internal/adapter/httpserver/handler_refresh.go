package httpserver

import "net/http"

// newRefreshHandler は POST /{slug}/_refresh ハンドラ。本実装は Task 13。
func newRefreshHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
