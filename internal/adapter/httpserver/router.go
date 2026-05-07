package httpserver

import "net/http"

// NewMux は 4 ルートを登録した http.ServeMux を返す。
// 各ハンドラの実装は handler_*.go に分かれている。
func NewMux(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", newHTMLHandler(deps))
	mux.HandleFunc("GET /{slug}/header.json", newHeaderHandler(deps))
	mux.HandleFunc("GET /{slug}/data.json", newDataHandler(deps))
	mux.HandleFunc("POST /{slug}/_refresh", newRefreshHandler(deps))
	return mux
}
