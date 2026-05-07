package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestHandlerData_ReturnsArrayWithMergedRaw(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0DATA0000000000A", "Satellite", []string{"0"}, []domain.SourceChart{
		{
			SourceID: "01JSRC0DATA0000000000A", Position: 0, MD5: "aaa", SHA256: "sha-aaa",
			Level: "0", Title: "Title A", Artist: "Artist A",
			Raw: map[string]any{"md5": "aaa", "url": "https://example.com/a", "url_diff": "https://example.com/a.diff"},
		},
	})
	f.seedPublished(t, "data-ok", "01JSRC0DATA0000000000A", domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(f.mux.URL + "/data-ok/data.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Len(t, got, 1)
	entry := got[0]
	require.Equal(t, "aaa", entry["md5"])
	require.Equal(t, "sha-aaa", entry["sha256"])
	require.Equal(t, "0", entry["level"])
	require.Equal(t, "Title A", entry["title"])
	require.Equal(t, "Artist A", entry["artist"])
	require.Equal(t, "https://example.com/a", entry["url"])
	require.Equal(t, "https://example.com/a.diff", entry["url_diff"])
}
