// Package gateway は外部 HTTP サービスからの取得を担う adapter 実装。
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// BMSTableFetcher は spec §7.1 のフローで難易度表を取得する。
type BMSTableFetcher struct {
	client *http.Client
	log    *slog.Logger
}

// NewBMSTableFetcher は新しい BMSTableFetcher を作る。
// client が nil の場合は http.DefaultClient を使う。
func NewBMSTableFetcher(client *http.Client, log *slog.Logger) *BMSTableFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &BMSTableFetcher{client: client, log: log}
}

// FetchByHeader は header.json URL から header と data.json を取得する。
func (f *BMSTableFetcher) FetchByHeader(
	ctx context.Context, headerURL string, etag string,
) (port.FetchedTable, error) {
	base, err := url.Parse(headerURL)
	if err != nil {
		return port.FetchedTable{}, fmt.Errorf("parse headerURL %q: %w", headerURL, err)
	}

	header, err := f.fetchHeader(ctx, headerURL)
	if err != nil {
		return port.FetchedTable{}, err
	}

	if header.DataURL == "" {
		return port.FetchedTable{}, fmt.Errorf("header.json に data_url がありません: %s", headerURL)
	}
	dataURL, err := resolveURL(base, header.DataURL)
	if err != nil {
		return port.FetchedTable{}, err
	}

	rawCharts, newETag, notModified, err := f.fetchData(ctx, dataURL, etag)
	if err != nil {
		return port.FetchedTable{}, err
	}
	if notModified {
		return port.FetchedTable{NotModified: true, ETag: etag}, nil
	}

	charts := make([]domain.SourceChart, 0, len(rawCharts))
	for i, raw := range rawCharts {
		c, err := chartFromRaw(i, raw)
		if err != nil {
			return port.FetchedTable{}, fmt.Errorf("chart[%d]: %w", i, err)
		}
		charts = append(charts, c)
	}

	header.DataURL = dataURL
	return port.FetchedTable{Header: header, Charts: charts, ETag: newETag}, nil
}

// FetchByHTML は次タスクで実装する（プレースホルダではなく未定義のままにし、
// 呼ばれたら明示エラーを返す）。
func (f *BMSTableFetcher) FetchByHTML(
	ctx context.Context, htmlURL string, etag string,
) (port.FetchedTable, error) {
	return port.FetchedTable{}, errors.New("FetchByHTML は未実装（Plan 2 / Task 9 で実装）")
}

// ---- 内部ヘルパ ----

func (f *BMSTableFetcher) fetchHeader(ctx context.Context, headerURL string) (domain.BMSTableHeader, error) {
	body, _, err := f.httpGet(ctx, headerURL, "")
	if err != nil {
		return domain.BMSTableHeader{}, fmt.Errorf("get header.json: %w", err)
	}
	defer body.Close()
	var h domain.BMSTableHeader
	if err := json.NewDecoder(body).Decode(&h); err != nil {
		return domain.BMSTableHeader{}, fmt.Errorf("decode header.json: %w", err)
	}
	return h, nil
}

// fetchData は dataURL を GET し、JSON 配列としてデコードする。
// 戻り値: rawCharts, 新 ETag, NotModified フラグ, エラー。
func (f *BMSTableFetcher) fetchData(
	ctx context.Context, dataURL string, etag string,
) ([]map[string]any, string, bool, error) {
	body, resp, err := f.httpGet(ctx, dataURL, etag)
	if err != nil {
		return nil, "", false, fmt.Errorf("get data.json: %w", err)
	}
	if resp.StatusCode == http.StatusNotModified {
		_ = body.Close()
		return nil, etag, true, nil
	}
	defer body.Close()
	var raw []map[string]any
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, "", false, fmt.Errorf("decode data.json: %w", err)
	}
	return raw, resp.Header.Get("ETag"), false, nil
}

// httpGet は GET を実行し、200/304 以外を error にする。返した Body は呼び出し側でクローズすること。
func (f *BMSTableFetcher) httpGet(
	ctx context.Context, rawURL string, ifNoneMatch string,
) (io.ReadCloser, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request %q: %w", rawURL, err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	req.Header.Set("Accept", "*/*")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("do request %q: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		_ = resp.Body.Close()
		return nil, resp, fmt.Errorf("status %d for %s", resp.StatusCode, rawURL)
	}
	return resp.Body, resp, nil
}

func resolveURL(base *url.URL, ref string) (string, error) {
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref %q: %w", ref, err)
	}
	return base.ResolveReference(refURL).String(), nil
}

// chartFromRaw は data.json の 1 エントリを SourceChart に変換する。
// md5 が空の場合はエラー。level は string / number どちらでも受ける。
func chartFromRaw(position int, raw map[string]any) (domain.SourceChart, error) {
	md5, _ := raw["md5"].(string)
	if md5 == "" {
		return domain.SourceChart{}, errors.New("md5 が空または欠落")
	}
	sha256, _ := raw["sha256"].(string)
	title, _ := raw["title"].(string)
	artist, _ := raw["artist"].(string)
	var level string
	switch v := raw["level"].(type) {
	case string:
		level = v
	case float64:
		level = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return domain.SourceChart{
		Position: position,
		MD5:      md5,
		SHA256:   sha256,
		Level:    level,
		Title:    title,
		Artist:   artist,
		Raw:      raw,
	}, nil
}
