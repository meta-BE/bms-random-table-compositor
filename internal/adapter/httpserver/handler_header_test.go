package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestHandlerHeader_ReturnsJSONWithLevelOrder(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0HEAD0000000000A", "Satellite", []string{"0", "1", "2"}, []domain.SourceChart{
		{SourceID: "01JSRC0HEAD0000000000A", Position: 0, MD5: "a", Level: "0", Title: "T0", Artist: "A", Raw: map[string]any{}},
		{SourceID: "01JSRC0HEAD0000000000A", Position: 1, MD5: "b", Level: "2", Title: "T1", Artist: "A", Raw: map[string]any{}},
	})
	f.seedPublished(t, "header-ok", "01JSRC0HEAD0000000000A", domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(f.mux.URL + "/header-ok/header.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "header-ok", got["name"])
	require.Equal(t, "data.json", got["data_url"])
	// level_order に "1" は含まれない（譜面が無いレベルは除外）
	levels, ok := got["level_order"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"0", "2"}, levels)
}

func TestHandlerHeader_NotFoundJSON(t *testing.T) {
	f := newHTTPFixture(t)
	resp, err := http.Get(f.mux.URL + "/missing/header.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var got map[string]string
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "not_found", got["error"])
}
