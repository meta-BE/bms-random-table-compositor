package httpserver

import "net/http"

// newDataHandler は GET /{slug}/data.json ハンドラ。本実装は Task 13。
func newDataHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
