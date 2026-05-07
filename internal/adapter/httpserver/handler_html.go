package httpserver

import "net/http"

// newHTMLHandler は GET /{slug} ハンドラ。本実装は Task 13。
func newHTMLHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
