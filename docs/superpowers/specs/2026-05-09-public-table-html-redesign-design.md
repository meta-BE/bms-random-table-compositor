# 公開表 HTML 表示の改修

- 作成日: 2026-05-09
- 対象: `internal/adapter/httpserver/templates/index.html` および `handler_html.go`
- スコープ: ローカルでホストする公開表ページ (`GET /{slug}`) の HTML レンダリングのみ。`data.json` / `header.json` (beatoraja 互換) は変更しない。

## 背景

現状の公開表 HTML は title / artist / md5 の3カラムをレベルごと別 `<table>` で表示している。以下の課題がある:

1. md5 列はユーザーにとって意味がほとんどなく、画面幅を圧迫する
2. レベルごとに独立した `<table>` のため列幅が一致せず、ページを縦に見ると列境界が揃わない
3. data.json には他にも url / url_diff / id / comment 等が来うるが、HTML 側で活用していない

## ゴール

- md5 カラムを廃止する
- title / artist それぞれに有用な外部リンクを貼る
- 差分BMS の DL リンク (url_diff) を1カラム追加する
- レベル+symbol を行頭に表示し、レベル間でも列幅が揃うようにする

## 非ゴール

- `data.json` / `header.json` のスキーマ変更 (beatoraja 互換のため)
- 表のフィルタリング/ソート等の動的UI (現状は静的HTML、これを維持)
- 公開表編集タブ (`PublishedTablesTab.svelte`) の改修

## 表示構造

各レベルの `<h2>` 見出しは現状のまま残す。下に置くテーブルは全レベル共通で以下の4カラム:

| # | カラム名 | 内容 | 空のときの挙動 |
|---|---------|------|----------------|
| 1 | レベル | `{Symbol}{Level}` (例: `st0`, `⭐3`) | 必ず非空 |
| 2 | タイトル | LR2IR ranking ページへのリンク (`<a href>`) | md5 が空なら平文表示 |
| 3 | アーティスト | `Raw["url"]` (本体BMS DLリンク) を `href` に | url が空なら平文表示 |
| 4 | 差分 | `Raw["url_diff"]` を `href` に、表示文言「差分DL」 | url_diff が空ならセル空白 |

LR2IR URL の組み立て規則:
```
http://www.dream-pro.info/~lavalse/LR2IR/search.cgi?mode=ranking&bmsmd5=<md5>
```
md5 は16進固定なのでURLエスケープ不要。Go 側で文字列結合し `template.URL` ではなく通常の string で渡す (`html/template` の href 自動エスケープに任せる)。

## 列幅の統一

全 `<table>` に `table-layout: fixed` を適用し、`<colgroup>` で共通の列幅を指定する:

```css
table.chart { table-layout: fixed; border-collapse: collapse; width: 100%; font-size: 0.9em; }
col.col-level   { width: 5em; }
col.col-title   { width: auto; }      /* 残り */
col.col-artist  { width: 20em; }
col.col-diff    { width: 7em; }

table.chart td { padding: 4px 8px; border-bottom: 1px solid #eee; line-height: 1.4; vertical-align: middle; }
table.chart td.title-cell  { white-space: normal; word-break: break-word; }
table.chart td.other-cell  { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
```

方針:
- **タイトル列のみ折り返し許可** — 長い曲名でも読める。複数行になっても `vertical-align: middle` で他列と縦中央揃え
- **アーティスト/差分/レベルは nowrap + ellipsis** — 列幅オーバーフロー時に "…" で切る
- 列幅 (5em / 20em / 7em) は将来微調整しやすいよう CSS 1箇所に集約

`<colgroup>` はレベル毎テーブルで毎回出力する。テンプレートの重複は避けるため `{{define "chart-cols"}}` ブロックを使い `{{template "chart-cols"}}` で呼び出す。

## データ構造の変更

`internal/adapter/httpserver/handler_html.go`:

```go
type htmlChart struct {
    Level    string  // 新規。{Symbol}{Level} を Go 側で結合
    Title    string
    Artist   string
    LR2IRURL string  // 新規。md5 が空なら ""
    URL      string  // 新規。Raw["url"] from data.json
    URLDiff  string  // 新規。Raw["url_diff"] from data.json
    Owned    bool
}
```

`htmlChart.MD5` は廃止 (LR2IR URL に変換済みのため不要)。

`buildHTMLPageData` の変更点:
- `c.Raw["url"]` / `c.Raw["url_diff"]` を `string` 型アサートで取得。アサート失敗 (型不一致 / nil) は `""` として扱う
- LR2IR URL は `c.MD5` が非空のときのみ組み立て、空なら `""`
- `Level` フィールドには `pub.Symbol + level` を結合した文字列を設定

## テンプレートの変更

`internal/adapter/httpserver/templates/index.html`:

- `<style>` から `.md5` を削除し、上記の `table.chart` / `col.*` / `td.*-cell` を追加
- `<table>` を `<table class="chart">` に変更し、直下に `<colgroup>` (`{{template "chart-cols"}}`) を出力
- `<tr>` の中身を以下に置換:
  ```html
  <td class="other-cell">{{.Level}}</td>
  <td class="title-cell">
    {{if .LR2IRURL}}<a href="{{.LR2IRURL}}" target="_blank" rel="noopener">{{.Title}}</a>{{else}}{{.Title}}{{end}}
  </td>
  <td class="other-cell">
    {{if .URL}}<a href="{{.URL}}" target="_blank" rel="noopener">{{.Artist}}</a>{{else}}{{.Artist}}{{end}}
  </td>
  <td class="other-cell">
    {{if .URLDiff}}<a href="{{.URLDiff}}" target="_blank" rel="noopener">差分DL</a>{{end}}
  </td>
  ```
- 既存の `<h2>{{$.Symbol}}{{.Level}} ({{len .Charts}}曲)</h2>`、`tr.owned` / `tr.unowned`、空ピック時の `<p class="empty">` 等はそのまま

外部リンクには `target="_blank" rel="noopener"` を付ける (ローカルアプリから外部サイトを開く際の典型的な安全側設定)。

## テスト

`internal/adapter/httpserver/handler_html_test.go` の既存テストを更新 + 追加:

1. md5 が HTML 出力に含まれないこと (旧 `class="md5"` セル消失の確認)
2. レベル列に `{Symbol}{Level}` の文字列 (例: `sl0`) が含まれること
3. 各リンクの有無による分岐:
   - md5 あり → タイトルが `<a href="…lavalse…bmsmd5=ownedmd5">` で囲まれる
   - md5 なし → タイトルは平文
   - `Raw["url"]` 非空 → アーティストがリンク
   - `Raw["url"]` 空/欠如 → アーティストは平文
   - `Raw["url_diff"]` 非空 → 「差分DL」リンクが出る
   - `Raw["url_diff"]` 空/欠如 → 差分セルが空白
4. 列幅統一の視覚確認は手動 (`make dev` で表示) — 自動テストの対象外

`Raw` に url を仕込んだ fixture を `handler_html_test.go` 内で組み立てる (既存テストは `Raw: map[string]any{"md5": "aaa"}` のような形)。

## 影響範囲

- 変更ファイル:
  - `internal/adapter/httpserver/templates/index.html`
  - `internal/adapter/httpserver/handler_html.go`
  - `internal/adapter/httpserver/handler_html_test.go`
- 影響なし:
  - `handler_data.go` / `handler_header.go` (beatoraja 用 JSON 出力)
  - フロントエンド Svelte 側 (Wails の編集UI)
  - DB スキーマ / マイグレーション

## オープン項目

- 列幅 (5em / 20em / 7em) は実データを見ながら微調整。実装後 `make dev` でユーザーに確認してもらう
- 「差分DL」表示文言は仮置き。短く `差分` 等が良ければ実装中に変更
