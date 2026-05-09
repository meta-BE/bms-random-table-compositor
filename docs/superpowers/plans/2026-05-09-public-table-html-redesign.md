# 公開表 HTML 表示の改修 — 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ローカルでホストする公開表 HTML (`GET /{slug}`) のカラム構成と CSS を再設計し、md5 列廃止 / LR2IR・url・url_diff リンク追加 / レベル列追加 / レベル間で列幅統一を行う。

**Architecture:** 既存の `html/template` 経由のサーバーサイドレンダリングを維持。`handler_html.go` の `htmlChart` を再構成し、`buildHTMLPageData` で `Raw["url"]` / `Raw["url_diff"]` を取り出して LR2IR URL を組み立てる。テンプレートは `<colgroup>` + `table-layout: fixed` で全レベルテーブルの列幅を統一。

**Tech Stack:** Go 1.x + `html/template`, embed FS, テストは Go testing + testify (`stretchr/testify/require|assert`)。

**Spec:** `docs/superpowers/specs/2026-05-09-public-table-html-redesign-design.md`

---

## ファイル構成

- 変更:
  - `internal/adapter/httpserver/handler_html.go` — `htmlChart` の構造変更 + `buildHTMLPageData` の組み立てロジック
  - `internal/adapter/httpserver/templates/index.html` — CSS / `<colgroup>` / `<tr>` の各セル
  - `internal/adapter/httpserver/handler_html_test.go` — テスト追加・更新
- 影響なし: `handler_data.go`, `handler_header.go`, フロントエンド, DB スキーマ

## 全体方針

- TDD で進める。各タスクで「失敗するテスト → 実装 → テスト通過 → コミット」を回す
- テストは既存ヘルパ (`newHTTPFixture` / `seedSourceWithCharts` / `seedPublished`) を使う
- コミットは小さく頻繁に (タスク単位)

---

### Task 1: htmlChart 構造の差し替え (赤いテストを書く)

**Files:**
- Modify: `internal/adapter/httpserver/handler_html_test.go`
- Reference: `internal/adapter/httpserver/handler_html.go:11-32`

新カラム要件をテストで定義する。既存の `TestHTMLHandler_OwnedOnlyFalse_ColorsByAttachedSongdata` は維持。新規テストでレベル列・LR2IR・url・url_diff の出力を検証する。

- [ ] **Step 1: 新規テストを追加 (まだ実装してないので失敗する)**

`internal/adapter/httpserver/handler_html_test.go` の末尾に以下を追加:

```go
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

	_ = fx.seedPublished(t, "html-cols", srcID, domain.RefreshModePerRequest, 0, false)

	resp, err := http.Get(fx.mux.URL + "/html-cols")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// md5 セル class が出ないこと
	assert.NotContains(t, bodyStr, `class="md5"`)
	// 生 md5 文字列が表示されないこと
	assert.NotContains(t, bodyStr, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// レベル列 (Symbol+Level)
	assert.Contains(t, bodyStr, ">sl0<")

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
```

- [ ] **Step 2: テストを走らせて失敗を確認**

```
go test ./internal/adapter/httpserver/ -run TestHandlerHTML_ColumnsAndLinks -v
```

Expected: 失敗 (新カラムが未実装のためアサーションが落ちる)。

- [ ] **Step 3: コミット (赤テスト)**

```
git add internal/adapter/httpserver/handler_html_test.go
git commit -m "test(httpserver): 公開表HTMLの新カラム/リンクの期待を追加 (赤)"
```

---

### Task 2: handler_html.go のデータ整形を書き換える

**Files:**
- Modify: `internal/adapter/httpserver/handler_html.go`

`htmlChart` の構造を変更し、`buildHTMLPageData` で url / url_diff / LR2IR URL を組み立てる。

- [ ] **Step 1: 既存ファイルを完全に置き換える**

`internal/adapter/httpserver/handler_html.go` の中身を以下に置換:

```go
package httpserver

import (
	"errors"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// htmlPageData はテンプレに渡すデータ。
type htmlPageData struct {
	Slug         string
	DisplayName  string
	Symbol       string
	GeneratedAt  string
	TotalCount   int
	IsManualMode bool
	Levels       []htmlLevel
}

type htmlLevel struct {
	Level  string
	Charts []htmlChart
}

// htmlChart は1曲分の表示用フィールド。
// Level は Symbol+Level を結合済みの文字列 (例: "sl0", "⭐3")。
// LR2IRURL/URL/URLDiff は空文字列のとき該当リンクを描画しない。
type htmlChart struct {
	Level    string
	Title    string
	Artist   string
	LR2IRURL string
	URL      string
	URLDiff  string
	Owned    bool
}

const lr2irRankingURLPrefix = "http://www.dream-pro.info/~lavalse/LR2IR/search.cgi?mode=ranking&bmsmd5="

// newHTMLHandler は GET /{slug} ハンドラ。
func newHTMLHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		ctx := r.Context()
		result, pub, err := deps.Pick.PickBySlug(ctx, slug)
		if err != nil {
			handleHTMLError(w, err)
			return
		}

		data := buildHTMLPageData(pub, result)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := indexTemplate.Execute(w, data); err != nil {
			deps.Log.Error("html template execute failed", "slug", slug, "err", err)
		}
	}
}

// buildHTMLPageData はピック結果をテンプレ向けに整形する。
// 各譜面の所持状態は EnrichedChart.IsOwned から読む。OwnedOnly 公開表は全件 owned 扱い。
func buildHTMLPageData(pub domain.PublishedTable, r domain.PickResult) htmlPageData {
	levels := make([]htmlLevel, 0, len(r.LevelOrder))
	for _, level := range r.LevelOrder {
		var charts []htmlChart
		for _, c := range r.Charts {
			if c.Level != level {
				continue
			}
			owned := c.IsOwned
			if pub.OwnedOnly {
				owned = true
			}
			charts = append(charts, htmlChart{
				Level:    pub.Symbol + level,
				Title:    c.Title,
				Artist:   c.Artist,
				LR2IRURL: lr2irURL(c.MD5),
				URL:      rawString(c.Raw, "url"),
				URLDiff:  rawString(c.Raw, "url_diff"),
				Owned:    owned,
			})
		}
		if len(charts) == 0 {
			continue
		}
		levels = append(levels, htmlLevel{Level: level, Charts: charts})
	}
	return htmlPageData{
		Slug:         pub.Slug,
		DisplayName:  pub.DisplayName,
		Symbol:       pub.Symbol,
		GeneratedAt:  r.GeneratedAt.Local().Format("2006-01-02 15:04:05 MST"),
		TotalCount:   len(r.Charts),
		IsManualMode: pub.Pick.RefreshMode == domain.RefreshModeManual,
		Levels:       levels,
	}
}

// lr2irURL は md5 が非空のときのみ LR2IR ranking URL を返す。
// md5 は16進固定なので URL エスケープ不要。
func lr2irURL(md5 string) string {
	if md5 == "" {
		return ""
	}
	return lr2irRankingURLPrefix + md5
}

// rawString は data.json パススルーフィールドから安全に文字列を取り出す。
// キー欠如・型不一致・nil はすべて "" を返す。
func rawString(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// handleHTMLError は usecase の sentinel error を HTTP ステータスに変換する。
func handleHTMLError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrPublishedTableNotFound):
		http.Error(w, "公開表が見つかりません", http.StatusNotFound)
	case errors.Is(err, usecase.ErrSourceNotFetched):
		http.Error(w, "ソース表が未取得です。設定画面から更新してください。", http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 2: コンパイル確認**

```
go build ./...
```

Expected: 成功。

- [ ] **Step 3: 既存テストが壊れていないか確認 (template 未変更なのでまだ赤の可能性あり)**

```
go test ./internal/adapter/httpserver/ -v
```

Expected: 既存テストは通る (template の `.MD5` 参照が消えるが、`.MD5` フィールドはまだ存在しないので template が壊れる)。
**ここで失敗する場合は次の Task 3 で template を直すまで待つ。** とりあえずこの段階での失敗内容をメモ。

- [ ] **Step 4: コミット**

```
git add internal/adapter/httpserver/handler_html.go
git commit -m "feat(httpserver): htmlChart にレベル/各種URLフィールドを追加"
```

---

### Task 3: index.html テンプレート差し替え

**Files:**
- Modify: `internal/adapter/httpserver/templates/index.html`

CSS と `<table>` 構造を新仕様に合わせる。

- [ ] **Step 1: テンプレートを完全に置き換える**

`internal/adapter/httpserver/templates/index.html` を以下に置換:

```html
<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="bmstable" content="/{{.Slug}}/header.json">
<title>{{.DisplayName}}</title>
<style>
body { font-family: system-ui, -apple-system, sans-serif; margin: 16px; color: #1b2636; }
h1 { font-size: 1.4em; margin: 0 0 8px; }
.meta { color: #666; font-size: 0.85em; margin-bottom: 16px; }
.refresh { margin-bottom: 16px; }
.refresh button { padding: 6px 14px; cursor: pointer; }
h2 { font-size: 1.1em; margin: 24px 0 4px; border-bottom: 1px solid #ccc; padding-bottom: 2px; }
table.chart { table-layout: fixed; border-collapse: collapse; width: 100%; font-size: 0.9em; }
table.chart col.col-level  { width: 5em; }
table.chart col.col-title  { width: auto; }
table.chart col.col-artist { width: 20em; }
table.chart col.col-diff   { width: 7em; }
table.chart th, table.chart td { padding: 4px 8px; border-bottom: 1px solid #eee; text-align: left; line-height: 1.4; vertical-align: middle; }
table.chart td.title-cell { white-space: normal; word-break: break-word; }
table.chart td.other-cell { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
table.chart a { color: inherit; text-decoration: underline; }
tr.owned td { background: #eaf6ea; }
tr.unowned td { color: #999; }
.empty { color: #999; font-style: italic; padding: 12px 0; }
</style>
</head>
<body>
  <h1>{{.Symbol}} {{.DisplayName}}</h1>
  <div class="meta">生成: {{.GeneratedAt}} / 全 {{.TotalCount}} 曲</div>
  {{if .IsManualMode}}
  <form class="refresh" method="POST" action="/{{.Slug}}/_refresh">
    <button type="submit">再ピック</button>
  </form>
  {{end}}
  {{if .Levels}}
    {{range .Levels}}
      <h2>{{$.Symbol}}{{.Level}} ({{len .Charts}}曲)</h2>
      <table class="chart">
        <colgroup>
          <col class="col-level">
          <col class="col-title">
          <col class="col-artist">
          <col class="col-diff">
        </colgroup>
        <tbody>
        {{range .Charts}}
          <tr class="{{if .Owned}}owned{{else}}unowned{{end}}">
            <td class="other-cell">{{.Level}}</td>
            <td class="title-cell">{{if .LR2IRURL}}<a href="{{.LR2IRURL}}" target="_blank" rel="noopener">{{.Title}}</a>{{else}}{{.Title}}{{end}}</td>
            <td class="other-cell">{{if .URL}}<a href="{{.URL}}" target="_blank" rel="noopener">{{.Artist}}</a>{{else}}{{.Artist}}{{end}}</td>
            <td class="other-cell">{{if .URLDiff}}<a href="{{.URLDiff}}" target="_blank" rel="noopener">差分DL</a>{{end}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
    {{end}}
  {{else}}
    <p class="empty">ピック結果が空です。所持限定の場合は songdata.db の設定を確認してください。</p>
  {{end}}
</body>
</html>
```

- [ ] **Step 2: 全テスト走らせて新しい赤テストを通す**

```
go test ./internal/adapter/httpserver/ -v
```

Expected: 全テスト緑 (Task 1 で書いた `TestHandlerHTML_ColumnsAndLinks` も通る)。

- [ ] **Step 3: 期待が落ちた場合の対処**

落ちたアサーションを1つずつ確認:

- リンクの URL に `&amp;` が出るか `&` のまま出るか — `html/template` は href 内でも `&` を `&amp;` にエスケープする。期待値は `&amp;` で書いてある
- `>差分DL<` などのアサーションは `<a ...>差分DL</a>` の "><" を狙ったもの。間にスペースが入るとマッチしないので注意
- もし html/template がリンクを書き換えていたら、`href` 値を `template.URL` で渡すか、テスト側のアサーション文字列を実出力に合わせる方向で調整 (実装より既存テストの fixture と整合させる)

修正後に再度テスト実行。

- [ ] **Step 4: コミット**

```
git add internal/adapter/httpserver/templates/index.html
git commit -m "feat(httpserver): 公開表HTMLにレベル列とDLリンク列、共通列幅を導入"
```

---

### Task 4: 全体テスト + 手動確認

**Files:** なし

- [ ] **Step 1: プロジェクト全体テスト**

```
go test $(go list ./... | grep -v internal/adapter/persistence)
```

(`testdata/songdata.db` がないクリーン環境だと persistence テストが落ちるため、CLAUDE.md の指示通り除外)

Expected: 全パッケージ緑。

- [ ] **Step 2: lint**

```
make lint
```

Expected: クリーン。

- [ ] **Step 3: dev 起動して目視確認**

```
make dev
```

確認項目 (手動):
- 公開表ページを開き、md5 のハッシュ文字列が消えていること
- レベル列に `sl0` `sl1` 等が表示されていること
- タイトルクリックで LR2IR ページへ飛ぶこと
- アーティストクリックで本体BMSページへ飛ぶこと (url がある場合)
- 差分DLリンクが url_diff 持ち譜面のみ出ること
- 各レベル `<h2>` 配下のテーブルで列の縦位置が揃っていること
- 列幅 (5em / 20em / 7em) が適切か。狭い/広いと感じたら CSS を調整

`make dev` で挙動確認できないままアプリが起動していない/Wails 環境がない場合は、ユーザーに「起動して確認してください」と伝える (これはユーザー操作が必要な手順なので、エージェントは判断できない)。

- [ ] **Step 4: 列幅やリンクテキストの微調整 (必要なら)**

`templates/index.html` の `col.col-*` 幅、または「差分DL」テキストを調整。調整したらまた `make dev` で確認 → コミット。

```
git add internal/adapter/httpserver/templates/index.html
git commit -m "refactor(httpserver): 公開表HTMLの列幅/文言を実データに合わせ調整"
```

(調整不要なら本ステップはスキップ)

---

## 完了条件

- `TestHandlerHTML_ColumnsAndLinks` 含む全テストが緑
- `make lint` が緑
- `make dev` で表示確認し、md5 列が消え、レベル列・LR2IR/url/url_diff リンクが期待通り出ていること
- レベル間で列の縦境界が揃っていること

## ロールバック

各タスクが独立コミットなので、問題があれば `git revert <commit>` で個別に戻せる。テンプレート単独 / handler 単独どちらの戻しも可能。
