package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
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

// 公開レベル名がソース表側のレベルと異なる場合に、
// header.json の "level_order" が「公開レベル名」で出力されることを検証する。
func TestHandlerHeader_LevelOrderUsesPublicLevelNames(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0HDR0000000000A", "Stella", []string{"a", "b"}, []domain.SourceChart{
		{SourceID: "01JSRC0HDR0000000000A", Position: 0, MD5: "x1", Level: "a", Title: "T1", Artist: "A", Raw: map[string]any{"md5": "x1"}},
		{SourceID: "01JSRC0HDR0000000000A", Position: 1, MD5: "x2", Level: "b", Title: "T2", Artist: "A", Raw: map[string]any{"md5": "x2"}},
	})

	// 公開レベル名は "Lv.Easy" / "Lv.Hard" でソース "a"/"b" にマップ
	_, err := f.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "named-levels", DisplayName: "N", Symbol: "★",
		RefreshMode: domain.RefreshModePerRequest,
		Levels: []usecase.PublishedTableLevelInput{
			{
				Name: "Lv.Easy", PerMappingPick: 1, TotalPick: 0,
				Mappings: []usecase.PublishedTableLevelMappingInput{
					{SourceTableID: "01JSRC0HDR0000000000A", SourceLevel: "a"},
				},
			},
			{
				Name: "Lv.Hard", PerMappingPick: 1, TotalPick: 0,
				Mappings: []usecase.PublishedTableLevelMappingInput{
					{SourceTableID: "01JSRC0HDR0000000000A", SourceLevel: "b"},
				},
			},
		},
	})
	require.NoError(t, err)

	resp, err := http.Get(f.mux.URL + "/named-levels/header.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, []any{"Lv.Easy", "Lv.Hard"}, got["level_order"])
}
