package gateway_test

import (
	"context"
	"io/ioutil"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/gateway"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	wd, _ := os.Getwd()
	root := filepath.Join(wd, "..", "..", "..")
	b, err := ioutil.ReadFile(filepath.Join(root, "testdata", name))
	require.NoError(t, err)
	return b
}

func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioutil.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestBMSTableFetcher_FetchByHeader_Basic(t *testing.T) {
	headerJSON := loadFixture(t, "source_table_fixture_header.json")
	dataJSON := loadFixture(t, "source_table_fixture_data.json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/header.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(headerJSON)
		case "/source_table_fixture_data.json":
			w.Header().Set("ETag", `"etag-A"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(dataJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	ft, err := f.FetchByHeader(context.Background(), ts.URL+"/header.json", "")
	require.NoError(t, err)
	require.False(t, ft.NotModified)
	require.Equal(t, "Fixture Table", ft.Header.Name)
	require.Equal(t, "fx", ft.Header.Symbol)
	require.Equal(t, ts.URL+"/source_table_fixture_data.json", ft.Header.DataURL,
		"DataURL は絶対化される")
	require.Equal(t, []string{"0", "1", "2"}, ft.Header.LevelOrder)
	require.Equal(t, `"etag-A"`, ft.ETag)
	require.Len(t, ft.Charts, 3)
	require.Equal(t, 0, ft.Charts[0].Position)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ft.Charts[0].MD5)
	require.Equal(t, "0", ft.Charts[0].Level)
	require.Equal(t, "First Song", ft.Charts[0].Title)
	require.Equal(t, "https://example.com/first", ft.Charts[0].Raw["url"],
		"raw に表固有フィールドが残る")
	require.Equal(t, float64(1001), ft.Charts[0].Raw["lr2_bmsid"])
}

func TestBMSTableFetcher_FetchByHeader_RespectsIfNoneMatch_304(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/header.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"X","symbol":"x","data_url":"data.json","level_order":[]}`))
		case "/data.json":
			if r.Header.Get("If-None-Match") == `"etag-prev"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"etag-prev"`)
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer ts.Close()

	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	ft, err := f.FetchByHeader(context.Background(), ts.URL+"/header.json", `"etag-prev"`)
	require.NoError(t, err)
	require.True(t, ft.NotModified)
}

func TestBMSTableFetcher_FetchByHeader_FollowsRedirect(t *testing.T) {
	// GAS 風 302: data.json が別オリジンに転送される
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"redir"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"md5":"abc","level":"5","title":"T"}]`))
	}))
	defer dataServer.Close()

	headerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/header.json":
			_, _ = w.Write([]byte(`{"name":"R","symbol":"r","data_url":"data.json"}`))
		case "/data.json":
			http.Redirect(w, r, dataServer.URL+"/", http.StatusFound)
		}
	}))
	defer headerServer.Close()

	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	ft, err := f.FetchByHeader(context.Background(), headerServer.URL+"/header.json", "")
	require.NoError(t, err)
	require.False(t, ft.NotModified)
	require.Len(t, ft.Charts, 1)
	require.Equal(t, "abc", ft.Charts[0].MD5)
	require.Equal(t, `"redir"`, ft.ETag)
}

func TestBMSTableFetcher_FetchByHeader_HeaderJSONStatusError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	_, err := f.FetchByHeader(context.Background(), ts.URL+"/header.json", "")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "500"), "status コードがエラーに含まれる")
}

func TestBMSTableFetcher_FetchByHeader_DataChartMissingMD5IsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/header.json":
			_, _ = w.Write([]byte(`{"name":"E","symbol":"e","data_url":"data.json"}`))
		case "/data.json":
			_, _ = w.Write([]byte(`[{"level":"0","title":"NoMD5"}]`))
		}
	}))
	defer ts.Close()
	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	_, err := f.FetchByHeader(context.Background(), ts.URL+"/header.json", "")
	require.Error(t, err)
}

func TestBMSTableFetcher_FetchByHTML_ResolvesRelativeMeta(t *testing.T) {
	htmlBody := loadFixture(t, "source_table_fixture.html")
	headerJSON := loadFixture(t, "source_table_fixture_header.json")
	dataJSON := loadFixture(t, "source_table_fixture_data.json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/table.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(htmlBody)
		case "/source_table_fixture_header.json":
			_, _ = w.Write(headerJSON)
		case "/source_table_fixture_data.json":
			w.Header().Set("ETag", `"etag-html"`)
			_, _ = w.Write(dataJSON)
		}
	}))
	defer ts.Close()

	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	ft, err := f.FetchByHTML(context.Background(), ts.URL+"/table.html", "")
	require.NoError(t, err)
	require.False(t, ft.NotModified)
	require.Equal(t, "Fixture Table", ft.Header.Name)
	require.Len(t, ft.Charts, 3)
	require.Equal(t, `"etag-html"`, ft.ETag)
}

func TestBMSTableFetcher_FetchByHTML_AbsoluteMetaURL(t *testing.T) {
	// header.json を別オリジンに置いて、HTML 内 meta が絶対 URL のケース
	headerHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/header.json":
			_, _ = w.Write([]byte(`{"name":"Abs","symbol":"a","data_url":"data.json"}`))
		case "/data.json":
			_, _ = w.Write([]byte(`[{"md5":"deadbeef","level":"0"}]`))
		}
	}))
	defer headerHost.Close()

	htmlHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		htmlBody := []byte(`<!doctype html><html><head>` +
			`<meta name="bmstable" content="` + headerHost.URL + `/header.json">` +
			`</head><body></body></html>`)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(htmlBody)
	}))
	defer htmlHost.Close()

	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	ft, err := f.FetchByHTML(context.Background(), htmlHost.URL+"/", "")
	require.NoError(t, err)
	require.Equal(t, "Abs", ft.Header.Name)
	require.Len(t, ft.Charts, 1)
	require.Equal(t, "deadbeef", ft.Charts[0].MD5)
}

func TestBMSTableFetcher_FetchByHTML_NoMetaTagIsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head></head><body>no meta</body></html>`))
	}))
	defer ts.Close()
	f := gateway.NewBMSTableFetcher(http.DefaultClient, newSilentLogger())
	_, err := f.FetchByHTML(context.Background(), ts.URL+"/", "")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "bmstable"))
}
