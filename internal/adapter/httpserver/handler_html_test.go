package httpserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestHandlerHTML_Success(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0HTML0000000000A", "Satellite", []string{"0", "1"}, []domain.SourceChart{
		{SourceID: "01JSRC0HTML0000000000A", Position: 0, MD5: "aaa", Level: "0", Title: "T0", Artist: "A0", Raw: map[string]any{"md5": "aaa"}},
		{SourceID: "01JSRC0HTML0000000000A", Position: 1, MD5: "bbb", Level: "1", Title: "T1", Artist: "A1", Raw: map[string]any{"md5": "bbb"}},
	})
	f.seedPublished(t, "html-ok", "01JSRC0HTML0000000000A", domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(f.mux.URL + "/html-ok")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}

func TestHandlerHTML_NotFoundReturns404(t *testing.T) {
	f := newHTTPFixture(t)
	resp, err := http.Get(f.mux.URL + "/no-such-slug")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHandlerHTML_SourceNotFetchedReturns503(t *testing.T) {
	f := newHTTPFixture(t)
	// SourceTable を作るが SaveFetched しない → status は never のまま
	_, err := f.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: "01JSRC0HTML0000000000B", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
	f.seedPublished(t, "html-503", "01JSRC0HTML0000000000B", domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(f.mux.URL + "/html-503")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
