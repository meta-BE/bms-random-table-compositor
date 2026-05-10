package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
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

func TestHandlerHTML_ColumnsAndLinks(t *testing.T) {
	fx := newHTTPFixture(t)

	srcID := "01JSRC0HTML000000000COL"
	_, err := fx.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: srcID, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, DisplayName: "T", Name: "T",
		LevelOrder:      []string{"0"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, fx.srcRepo.SaveFetched(context.Background(), srcID, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "T", Symbol: "sl", LevelOrder: []string{"0"}},
		Charts: []domain.SourceChart{
			{
				Position: 0, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Level: "0", Title: "Full", Artist: "ArtFull",
				Raw: map[string]any{
					"md5":      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"url":      "https://example.com/song-a.zip",
					"url_diff": "https://example.com/diff-a.zip",
				},
			},
			{
				Position: 1, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Level: "0", Title: "NoUrl", Artist: "ArtNoUrl",
				Raw: map[string]any{
					"md5":      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					"url":      "",
					"url_diff": "",
				},
			},
			{
				Position: 2, MD5: "",
				Level: "0", Title: "NoMD5", Artist: "ArtNoMD5",
				Raw: map[string]any{},
			},
		},
	}, time.Now()))

	// pub.Symbol を source.Symbol("sl") と区別するため "X" にする。
	// 行頭セルは source 由来 ("sl0")、<h2> 見出しは pub 由来 ("X0") になる想定。
	_ = fx.seedPublished(t, "html-cols", srcID, domain.RefreshModePerRequest, 0, false, "X")

	resp, err := http.Get(fx.mux.URL + "/html-cols")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// md5 セル class が出ないこと
	assert.NotContains(t, bodyStr, `class="md5"`)
	// md5 がセル本文として表示されないこと (LR2IR リンクの href 内に出るのは OK、
	// 表セルのテキストとして出ることだけ禁止する)
	assert.NotContains(t, bodyStr, ">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa<")

	// レベル列 (source.Symbol + Level)
	assert.Contains(t, bodyStr, ">sl0<")
	// <h2> 見出しは pub.Symbol + Level
	assert.Contains(t, bodyStr, ">X0 (")

	// Full 行: タイトルが LR2IR リンク, アーティストが url リンク, 差分DLリンク
	assert.Contains(t, bodyStr, `href="http://www.dream-pro.info/~lavalse/LR2IR/search.cgi?mode=ranking&amp;bmsmd5=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
	assert.Contains(t, bodyStr, `>Full<`)
	assert.Contains(t, bodyStr, `href="https://example.com/song-a.zip"`)
	assert.Contains(t, bodyStr, `>ArtFull<`)
	assert.Contains(t, bodyStr, `href="https://example.com/diff-a.zip"`)
	assert.Contains(t, bodyStr, `>差分DL<`)

	// NoUrl 行: タイトルは LR2IR リンクあり、アーティストは平文、差分セルは空
	assert.Contains(t, bodyStr, `href="http://www.dream-pro.info/~lavalse/LR2IR/search.cgi?mode=ranking&amp;bmsmd5=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`)
	// アーティスト ArtNoUrl はリンクで囲まれていない: その直前に <a href= が無いことを大まかにチェック
	assert.Contains(t, bodyStr, "ArtNoUrl")
	assert.NotContains(t, bodyStr, `<a href="">`)

	// NoMD5 行: タイトルが LR2IR リンクで囲まれない
	// (md5 が空のためリンク URL は生成されない)
	assert.Contains(t, bodyStr, "NoMD5")
}

// 行頭セルはソースレベル (EnrichedChart.Level) を Symbol と組み合わせる。
// 公開レベル名 (PublicLevel) がソースと異なっていても、行頭セルにはソースレベルが入る。
// 一方で <h2> 見出しは公開レベル名でグルーピングされる。
func TestHandlerHTML_RowCellUsesSourceLevel_HeadingUsesPublicLevel(t *testing.T) {
	fx := newHTTPFixture(t)
	srcID := "01JSRC0HTML000000000SRCLV"
	_, err := fx.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: srcID, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, DisplayName: "T", Name: "T",
		LevelOrder:      []string{"5"},
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, fx.srcRepo.SaveFetched(context.Background(), srcID, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "T", Symbol: "sl", LevelOrder: []string{"5"}},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Level: "5", Title: "Song5", Artist: "A", Raw: map[string]any{"md5": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		},
	}, time.Now()))

	// 公開レベル名はソースとは異なる "5-mix"
	_, err = fx.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "src-vs-public", DisplayName: "X", Symbol: "★",
		RefreshMode: domain.RefreshModePerRequest,
		Levels: []usecase.PublishedTableLevelInput{
			{
				Name: "5-mix", PerMappingPick: 1, TotalPick: 0,
				Mappings: []usecase.PublishedTableLevelMappingInput{
					{SourceTableID: srcID, SourceLevel: "5"},
				},
			},
		},
	})
	require.NoError(t, err)

	resp, err := http.Get(fx.mux.URL + "/src-vs-public")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// 行頭セル (td.level) は Source Symbol("sl") + Source Level("5") = "sl5"。
	// 公開レベル名 "5-mix" は行頭セルに混ざってはいけない。
	assert.Contains(t, bodyStr, ">sl5<", "行頭セルは Source Symbol + Source Level のはず")
	assert.NotContains(t, bodyStr, ">sl5-mix<", "行頭セルに公開レベル名が紛れてはいけない")
	// <h2> 見出しは公開レベル名でグルーピング (pub.Symbol="★" + PublicLevel="5-mix")
	assert.Contains(t, bodyStr, "★5-mix")
}
