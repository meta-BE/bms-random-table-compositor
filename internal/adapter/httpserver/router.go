package httpserver

import "net/http"

// NewMux は 4 ルートを登録した http.Handler を返す。LoggingMiddleware でラップ済み。
func NewMux(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", newHTMLHandler(deps))
	mux.HandleFunc("GET /{slug}/header.json", newHeaderHandler(deps))
	mux.HandleFunc("GET /{slug}/data.json", newDataHandler(deps))
	mux.HandleFunc("POST /{slug}/_refresh", newRefreshHandler(deps))
	if deps.Dashboard != nil {
		return LoggingMiddleware(deps.Dashboard)(mux)
	}
	return mux
}
