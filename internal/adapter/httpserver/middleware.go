package httpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// statusCapturingWriter はステータスコードをキャプチャする ResponseWriter ラッパ。
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// LoggingMiddleware は DashboardUseCase にリクエスト履歴を記録するミドルウェアを返す。
func LoggingMiddleware(d *usecase.DashboardUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			scw := &statusCapturingWriter{ResponseWriter: w}
			next.ServeHTTP(scw, r)
			status := scw.status
			if status == 0 {
				status = http.StatusOK
			}
			d.AppendRequest(domain.RequestLogEntry{
				At:         start,
				Method:     r.Method,
				Path:       r.URL.Path,
				Slug:       firstPathSegment(r.URL.Path),
				StatusCode: status,
				DurationMs: time.Since(start).Milliseconds(),
			})
		})
	}
}

// firstPathSegment は "/sl-random/data.json" から "sl-random" を抽出する。
// 先頭が "/" でない、または segment が無い場合は空文字を返す。
func firstPathSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
