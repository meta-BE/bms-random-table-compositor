package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/stretchr/testify/assert"
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

func TestHTMLHandler_OwnedOnlyFalse_ColorsByAttachedSongdata(t *testing.T) {
	fx := newHTTPFixture(t)
	fx.seedAttachedSongdata(t, "ownedmd5")

	srcID := "01J0SRC000000000000000000A"
	_, err := fx.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: srcID, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, DisplayName: "T", Name: "T",
		LevelOrder: []string{"sl0"}, LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, fx.srcRepo.SaveFetched(context.Background(), srcID, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "T", LevelOrder: []string{"sl0"}},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "ownedmd5", Level: "sl0", Title: "owned-song", Raw: map[string]any{"md5": "ownedmd5"}},
			{Position: 1, MD5: "othermd5", Level: "sl0", Title: "other-song", Raw: map[string]any{"md5": "othermd5"}},
		},
	}, time.Now()))

	_ = fx.seedPublished(t, "t", srcID, domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(fx.mux.URL + "/t")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	bodyStr := string(body)
	assert.Contains(t, bodyStr, "owned-song")
	assert.Contains(t, bodyStr, "other-song")
	assert.Contains(t, bodyStr, `class="owned"`)
	assert.Contains(t, bodyStr, `class="unowned"`)
}
