package httpserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

// newRefreshClient は 303 リダイレクトを自動追従しないクライアントを返す。
func newRefreshClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestHandlerRefresh_Manual_Returns303(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0REF0000000000AA", "X", []string{"0"}, []domain.SourceChart{
		{SourceID: "01JSRC0REF0000000000AA", Position: 0, MD5: "a", Level: "0", Title: "T", Artist: "A", Raw: map[string]any{}},
	})
	f.seedPublished(t, "ref-manual", "01JSRC0REF0000000000AA", domain.RefreshModeManual, 0, false)

	c := newRefreshClient()
	resp, err := c.Post(f.mux.URL+"/ref-manual/_refresh", "", strings.NewReader(""))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/ref-manual", resp.Header.Get("Location"))
}

func TestHandlerRefresh_NonManualReturns405(t *testing.T) {
	f := newHTTPFixture(t)
	f.seedSourceWithCharts(t, "01JSRC0REF0000000000BB", "X", []string{"0"}, []domain.SourceChart{
		{SourceID: "01JSRC0REF0000000000BB", Position: 0, MD5: "a", Level: "0", Title: "T", Artist: "A", Raw: map[string]any{}},
	})
	f.seedPublished(t, "ref-daily", "01JSRC0REF0000000000BB", domain.RefreshModeDaily, 0, false)

	resp, err := http.Post(f.mux.URL+"/ref-daily/_refresh", "", strings.NewReader(""))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHandlerRefresh_NotFound(t *testing.T) {
	f := newHTTPFixture(t)
	resp, err := http.Post(f.mux.URL+"/no-slug/_refresh", "", strings.NewReader(""))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
