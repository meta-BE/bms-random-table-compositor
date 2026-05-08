package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestLoggingMiddleware_AppendsRequest(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("teapot"))
	})
	wrapped := httpserver.LoggingMiddleware(d)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sl-random/header.json", nil)
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)

	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 1)
	assert.Equal(t, "/sl-random/header.json", snap.Requests[0].Path)
	assert.Equal(t, "GET", snap.Requests[0].Method)
	assert.Equal(t, http.StatusTeapot, snap.Requests[0].StatusCode)
	assert.Equal(t, "sl-random", snap.Requests[0].Slug)
}

func TestLoggingMiddleware_RootPathHasEmptySlug(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	wrapped := httpserver.LoggingMiddleware(d)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 1)
	assert.Equal(t, "", snap.Requests[0].Slug)
}

func TestLoggingMiddleware_DefaultStatus200WhenNotWritten(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	wrapped := httpserver.LoggingMiddleware(d)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/sl/data.json", nil))
	snap := d.Snapshot()
	assert.Equal(t, 200, snap.Requests[0].StatusCode)
}
