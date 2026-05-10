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

// 公開レベル名がソース表側のレベルと異なる場合に、
// data.json の "level" フィールドが「公開レベル名」で上書きされて出力されることを検証する。
// （Task 7 の pickLevel が公開レベル名で Chart.Level を上書きする仕様の HTTP 出力検証）
func TestHandlerData_LevelIsOverriddenWithPublicLevelName(t *testing.T) {
	f := newHTTPFixture(t)
	// ソース表は "5" レベルを持つ
	f.seedSourceWithCharts(t, "01JSRC0LVL0000000000A", "Satellite", []string{"5"}, []domain.SourceChart{
		{
			SourceID: "01JSRC0LVL0000000000A", Position: 0,
			MD5: "abc", Level: "5", Title: "T", Artist: "A",
			Raw: map[string]any{"md5": "abc", "url": "https://example.com/a"},
		},
	})

	// 公開表のレベル名は "5-mix" (ソースレベル "5" を別名で公開)
	_, err := f.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "level-override", DisplayName: "L", Symbol: "★",
		RefreshMode: domain.RefreshModePerRequest,
		Levels: []usecase.PublishedTableLevelInput{
			{
				Name: "5-mix", PerMappingPick: 1, TotalPick: 0,
				Mappings: []usecase.PublishedTableLevelMappingInput{
					{SourceTableID: "01JSRC0LVL0000000000A", SourceLevel: "5"},
				},
			},
		},
	})
	require.NoError(t, err)

	resp, err := http.Get(f.mux.URL + "/level-override/data.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Len(t, got, 1)
	require.Equal(t, "5-mix", got[0]["level"], "public level name で上書きされているはず")
	// Raw の url 等はパススルーされている
	require.Equal(t, "https://example.com/a", got[0]["url"])
}
