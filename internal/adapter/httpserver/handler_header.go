package httpserver

import "net/http"

// newHeaderHandler は GET /{slug}/header.json ハンドラ。本実装は Task 13。
func newHeaderHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
