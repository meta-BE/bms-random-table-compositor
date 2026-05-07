# Phase 1 / Plan 2: ソース表取り込み（Fetcher + Repo + UseCase + GUI 管理タブ）実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ユーザーが指定した難易度表 URL（HTML or `header.json`）から `header.json` + `data.json` を取得し、`compositor.db` の `source_table` / `source_table_chart` に正規化保存する。起動時にバックグラウンド全件更新、GUI から手動追加・更新・削除・表示名編集ができる状態まで完成させる。

**Architecture:** spec 7.1 のフローを `BMSTableFetcher`（adapter/gateway）+ `SourceTableRepo`（adapter/persistence）+ `SourceTableUseCase`（usecase）の3層で実装。HTML→`header.json`→`data.json` の3段取得を `golang.org/x/net/html` と標準 `net/http` で行い、`http.Client` のデフォルトリダイレクト追従で GAS 302 を吸収、`If-None-Match` ヘッダで ETag 304 をハンドリングする。`source_table_chart` への保存はトランザクション内で全削除→再挿入し原子性を担保。`raw_json` カラムで data.json エントリの全フィールドをパススルー保持。GUI は `frontend/src/lib/tabs/SourceTablesTab.svelte` を新規追加し、`App.svelte` をタブ切替化。起動時は `app.go` の `OnStartup` から `RefreshAll` を goroutine で発火（並列度4）。

**Tech Stack:** Go 1.24 / Wails v2.11.0 / `modernc.org/sqlite` / 標準 `net/http` / `golang.org/x/net/html`（go.mod に既に indirect で入っているため `go get` で direct に昇格）/ `oklog/ulid/v2`（Plan 2 で初導入。ID 生成器のテスタビリティ確保のため `port.IDGenerator` 経由で注入）/ Svelte + TypeScript

**設計ドキュメント:** `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` の §5（domain型・スキーマ）/ §6（port インタフェース）/ §7.1（取り込みフロー）/ §10.2（fetcher テスト方針）

**Phase 1 全体の Plan 分割:** Plan 1（基盤＝完了） → **Plan 2（本ファイル）** → Plan 3（公開表 + ピック + HTTPサーバ） → Plan 4（GUI 仕上げ + E2E）

**完了条件:**

- 実機で Satellite / Stella / 発狂 / Solomon の各 URL を GUI から登録 → 起動時バックグラウンド更新で `source_table` と `source_table_chart` に行が入る
- 取得成功時 `last_fetch_status='ok'`、失敗時 `last_fetch_status='error'` + `last_fetch_error` にメッセージ。**前回成功時のキャッシュは保持される**
- 再起動後にもう一度 `RefreshAll` が走り、ETag 一致時は 304 → `last_fetched_at` のみ更新、譜面行は変わらない
- 既存の Plan 1 動作（設定タブ・トレイ・SingleInstanceLock）は無回帰
- `go build ./...` と `go test ./...` がすべて pass、`make build` で macOS 成果物が生成される
- `gh workflow run build-windows.yml` で Windows exe が生成され Windows 機での GUI 操作が動く

**スコープ外（Plan 3 以降）:**

- 公開表 (`published_table`) の CRUD と所持絞り込み・ピックロジック
- HTTP サーバ（3エンドポイント＋HTMLビュー）
- ダッシュボード（リクエスト履歴・取得履歴のリングバッファ表示）
- ピック結果のメモリキャッシュ（`PickResultStore`）

**ブランチ運用:** Plan 1 と同様に main 上で直接コミット（`workflow_dispatch` のデフォルトブランチ要件）。

---

## ファイル構造（Plan 2 終了時点で追加・変更されるもの）

新規作成:

```
internal/
├── domain/
│   ├── source_table.go            # SourceTable, InputKind, FetchStatus
│   ├── source_chart.go            # SourceChart
│   ├── bms_table_header.go        # BMSTableHeader + 柔軟な UnmarshalJSON
│   └── bms_table_header_test.go   # level_order の string/number 両対応テスト
├── port/
│   ├── idgen.go                   # IDGenerator インタフェース
│   ├── source_table_fetcher.go    # SourceTableFetcher + FetchedTable
│   └── source_table_repo.go       # SourceTableRepo インタフェース
├── adapter/
│   ├── idgen/
│   │   ├── ulid.go                # ULIDGenerator (oklog/ulid/v2)
│   │   └── ulid_test.go
│   ├── persistence/
│   │   ├── source_table_repo.go   # SourceTableRepoSQL
│   │   └── source_table_repo_test.go
│   └── gateway/
│       ├── bmstable_fetcher.go    # BMSTableFetcher（HTML/header/data の3段取得）
│       └── bmstable_fetcher_test.go
└── usecase/
    ├── source_table_usecase.go
    └── source_table_usecase_test.go

internal/app/handler/
├── source_table_handler.go        # Wails Bind
└── source_table_handler_test.go

frontend/src/lib/tabs/
└── SourceTablesTab.svelte         # ソース表管理タブ

testdata/
├── source_table_fixture.html      # HTML→meta name="bmstable" 抽出テスト用
├── source_table_fixture_header.json  # 最小 header.json
└── source_table_fixture_data.json    # 最小 data.json
```

変更:

```
go.mod / go.sum                    # golang.org/x/net を direct に、oklog/ulid/v2 追加
internal/app/bootstrap.go          # SourceTableRepo / Fetcher / UseCase / Handler 配線
main.go                            # Bind 配列に SourceTableHandler 追加
app.go                             # OnStartup で RefreshAll を goroutine 起動 + SetContext
frontend/src/lib/api.ts            # ソース表 API ラッパ追加
frontend/src/App.svelte            # タブ切替（サーバ / ソース表）
```

各ファイルの責務:

| ファイル | 責務 |
|---|---|
| `internal/domain/source_table.go` | `SourceTable` 構造体、`InputKind`（HTML / HeaderJSON）、`FetchStatus`（Never / OK / Error） |
| `internal/domain/source_chart.go` | `SourceChart` 構造体（Position, MD5, SHA256, Level, Title, Artist, Raw） |
| `internal/domain/bms_table_header.go` | `BMSTableHeader` + 柔軟な `UnmarshalJSON`（`level_order` が string 配列 / 数値配列のいずれでも受ける） |
| `internal/port/idgen.go` | `IDGenerator` インタフェース（`New() string`） |
| `internal/port/source_table_fetcher.go` | `SourceTableFetcher` + `FetchedTable` 構造体（Header, Charts, ETag, NotModified） |
| `internal/port/source_table_repo.go` | `SourceTableRepo` インタフェース（CRUD + SaveFetched/MarkFetchError/LoadCharts） |
| `internal/adapter/idgen/ulid.go` | `oklog/ulid/v2` ベースの ULID 生成器（crypto/rand エントロピー） |
| `internal/adapter/persistence/source_table_repo.go` | `SourceTableRepoSQL`（CRUD は単一クエリ、`SaveFetched` は Tx 内で UPDATE → DELETE 譜面 → INSERT 譜面） |
| `internal/adapter/gateway/bmstable_fetcher.go` | `BMSTableFetcher`（FetchByHTML / FetchByHeader、相対 data_url 絶対化、ETag 対応、HTML パース） |
| `internal/usecase/source_table_usecase.go` | 入力バリデーション、`RefreshOne` の成功/失敗フォーク、`RefreshAll` の並列度4制御 |
| `internal/app/handler/source_table_handler.go` | Wails Bind: `ListSourceTables` / `AddSourceTable` / `RefreshSourceTable` / `RefreshAllSourceTables` / `DeleteSourceTable` / `UpdateSourceTableDisplayName` |
| `frontend/src/lib/tabs/SourceTablesTab.svelte` | 一覧表示（DisplayName / URL / 状態バッジ / lastFetchedAt）+ 追加フォーム + 行操作（更新 / 削除 / 表示名編集） |

---

## 前提条件と注意

- Plan 1 完了済み（main.go / app.go / `internal/app/bootstrap.go` / `internal/app/handler/config_handler.go` / `frontend/src/App.svelte` / `internal/adapter/persistence/migrations.go` 等が存在する）
- `internal/adapter/persistence/migrations.go` に `source_table` / `source_table_chart` の DDL は既に書かれている（Plan 1 で `CREATE IF NOT EXISTS` 済み）。Plan 2 で追加スキーマ変更は無し
- `golang.org/x/net` は go.mod に indirect で入っている（Wails が間接依存）。Plan 2 で `golang.org/x/net/html` を直接 import するため direct に昇格
- `oklog/ulid/v2` は Plan 1 では未導入。Plan 2 で `go get` する
- 作業ブランチは **main**（ユーザー指定）
- テスト用のフィクスチャ HTML / JSON は `testdata/` 直下に配置。既存の `testdata/satellite_*` ファイルは Plan 3 の所持判定テストで使うため Plan 2 では触らない

---

## Task 1: domain 型と `BMSTableHeader.UnmarshalJSON` を追加

**Files:**

- Create: `internal/domain/source_table.go`
- Create: `internal/domain/source_chart.go`
- Create: `internal/domain/bms_table_header.go`
- Test: `internal/domain/bms_table_header_test.go`

domain 層は外部依存ゼロ。`BMSTableHeader.UnmarshalJSON` のみテストを書く（他は値オブジェクトでロジック無し）。`level_order` は実在表で string 配列だが、念のため数値配列にも対応する（spec 第7.1節「fetcher が data_url を絶対化」と同じ精神で、入力の揺れを fetcher 層で吸収する）。

- [ ] **Step 1: 失敗テストを書く**

`internal/domain/bms_table_header_test.go`:

```go
package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBMSTableHeader_UnmarshalJSON_StringLevelOrder(t *testing.T) {
	src := []byte(`{
		"name":"Satellite","symbol":"sl","data_url":"satellite_data.json",
		"level_order":["0","1","2","3"]
	}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Equal(t, "Satellite", h.Name)
	require.Equal(t, "sl", h.Symbol)
	require.Equal(t, "satellite_data.json", h.DataURL)
	require.Equal(t, []string{"0", "1", "2", "3"}, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_NumberLevelOrder(t *testing.T) {
	src := []byte(`{
		"name":"X","symbol":"x","data_url":"data.json",
		"level_order":[0,1,2.5,3]
	}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Equal(t, []string{"0", "1", "2.5", "3"}, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_MissingLevelOrderIsNil(t *testing.T) {
	src := []byte(`{"name":"Y","symbol":"y","data_url":"d.json"}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Nil(t, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_RejectsMixedArray(t *testing.T) {
	src := []byte(`{"name":"Z","symbol":"z","data_url":"d.json","level_order":["0",1]}`)
	var h domain.BMSTableHeader
	require.Error(t, json.Unmarshal(src, &h))
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/domain/...`
Expected: `package domain not declared` 等で FAIL

- [ ] **Step 3: domain 型を実装**

`internal/domain/source_table.go`:

```go
// Package domain は外部依存を持たない値オブジェクト群を提供する。
package domain

import "time"

// InputKind はソース表の入力 URL 種別。
type InputKind string

const (
	InputKindHTML       InputKind = "html"
	InputKindHeaderJSON InputKind = "header_json"
)

// FetchStatus はソース表の最後の取得結果。
type FetchStatus string

const (
	FetchStatusNever FetchStatus = "never"
	FetchStatusOK    FetchStatus = "ok"
	FetchStatusError FetchStatus = "error"
)

// SourceTable はユーザーが登録した難易度表のメタ情報。
type SourceTable struct {
	ID              string
	InputURL        string
	InputKind       InputKind
	DisplayName     string
	Name            string
	Symbol          string
	LevelOrder      []string
	DataURL         string
	ETag            string
	LastFetchedAt   *time.Time
	LastFetchStatus FetchStatus
	LastFetchError  string
}
```

`internal/domain/source_chart.go`:

```go
package domain

// SourceChart はソース表の譜面エントリ。`Raw` には data.json の元エントリを
// パススルー保持する（HTTP 応答時に表固有フィールドをそのまま返すため）。
type SourceChart struct {
	SourceID string
	Position int
	MD5      string
	SHA256   string
	Level    string
	Title    string
	Artist   string
	Raw      map[string]any
}
```

`internal/domain/bms_table_header.go`:

```go
package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// BMSTableHeader は header.json をデコードした構造体。
// `level_order` は string 配列 / 数値配列の両方を受け付け、内部表現は []string に正規化する。
type BMSTableHeader struct {
	Name       string
	Symbol     string
	DataURL    string
	LevelOrder []string
}

type rawBMSTableHeader struct {
	Name       string          `json:"name"`
	Symbol     string          `json:"symbol"`
	DataURL    string          `json:"data_url"`
	LevelOrder json.RawMessage `json:"level_order"`
}

// UnmarshalJSON は level_order の型ゆらぎ（string 配列 / number 配列）を吸収する。
func (h *BMSTableHeader) UnmarshalJSON(data []byte) error {
	var raw rawBMSTableHeader
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	h.Name = raw.Name
	h.Symbol = raw.Symbol
	h.DataURL = raw.DataURL

	if len(raw.LevelOrder) == 0 || string(raw.LevelOrder) == "null" {
		h.LevelOrder = nil
		return nil
	}

	var asStrings []string
	if err := json.Unmarshal(raw.LevelOrder, &asStrings); err == nil {
		h.LevelOrder = asStrings
		return nil
	}
	var asNumbers []float64
	if err := json.Unmarshal(raw.LevelOrder, &asNumbers); err == nil {
		out := make([]string, len(asNumbers))
		for i, n := range asNumbers {
			out[i] = strconv.FormatFloat(n, 'f', -1, 64)
		}
		h.LevelOrder = out
		return nil
	}
	return fmt.Errorf("level_order: 文字列配列または数値配列でなければなりません: %s", string(raw.LevelOrder))
}
```

- [ ] **Step 4: テストが pass することを確認**

Run: `go test ./internal/domain/...`
Expected: PASS（4 件）

- [ ] **Step 5: コミット**

```bash
git add internal/domain/
git commit -m "$(cat <<'EOF'
feat(domain): ソース表まわりの値型を追加

SourceTable / SourceChart / BMSTableHeader / InputKind / FetchStatus
を新設。BMSTableHeader.UnmarshalJSON は level_order が string 配列
でも数値配列でも []string として保持する（実在表での型ゆらぎ吸収）。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: port インタフェースを 3 つ追加

**Files:**

- Create: `internal/port/idgen.go`
- Create: `internal/port/source_table_fetcher.go`
- Create: `internal/port/source_table_repo.go`

port 層はインタフェース定義のみでロジックを持たないため単体テストは不要（既存 `port/config_store.go` も同様の方針）。

- [ ] **Step 1: `internal/port/idgen.go` を作成**

```go
package port

// IDGenerator は ULID 等のユニーク ID を生成する。
// usecase 層はこのインタフェース経由で ID を取得し、テストではフェイクで決定論化する。
type IDGenerator interface {
	New() string
}
```

- [ ] **Step 2: `internal/port/source_table_fetcher.go` を作成**

```go
package port

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// FetchedTable は SourceTableFetcher が返す取得結果。
// NotModified=true の場合 Header / Charts / ETag は意味を持たない（呼び出し側で破棄）。
type FetchedTable struct {
	Header      domain.BMSTableHeader
	Charts      []domain.SourceChart
	ETag        string
	NotModified bool
}

// SourceTableFetcher は外部 URL から難易度表を取得する。
type SourceTableFetcher interface {
	// FetchByHTML は HTML ページから <meta name="bmstable"> を抽出し、
	// header.json → data.json を順に取得する。
	FetchByHTML(ctx context.Context, htmlURL string, etag string) (FetchedTable, error)
	// FetchByHeader は header.json URL 直接指定で取得する。
	FetchByHeader(ctx context.Context, headerURL string, etag string) (FetchedTable, error)
}
```

- [ ] **Step 3: `internal/port/source_table_repo.go` を作成**

```go
package port

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// SourceTableRepo は source_table / source_table_chart を永続化する。
type SourceTableRepo interface {
	List(ctx context.Context) ([]domain.SourceTable, error)
	Get(ctx context.Context, id string) (domain.SourceTable, error)
	// Create は SourceTable を新規挿入する。in.ID は事前に採番済みであること。
	Create(ctx context.Context, in domain.SourceTable) (string, error)
	Update(ctx context.Context, t domain.SourceTable) error
	Delete(ctx context.Context, id string) error
	// SaveFetched は取得結果を Tx 内で保存する。
	// NotModified=true の場合は last_fetched_at と updated_at のみ更新し、譜面行は変更しない。
	SaveFetched(ctx context.Context, sourceID string, ft FetchedTable, fetchedAt time.Time) error
	// MarkFetchError は取得失敗を記録する（譜面行は触らない）。
	MarkFetchError(ctx context.Context, sourceID string, fetchErr error, fetchedAt time.Time) error
	// LoadCharts は source_table_chart を position 昇順で返す。
	LoadCharts(ctx context.Context, sourceID string) ([]domain.SourceChart, error)
}
```

- [ ] **Step 4: ビルド確認**

Run: `go build ./...`
Expected: エラーなし。

- [ ] **Step 5: コミット**

```bash
git add internal/port/
git commit -m "$(cat <<'EOF'
feat(port): IDGenerator / SourceTableFetcher / SourceTableRepo を追加

ソース表の取得・永続化・ID 生成のインタフェースを定義。実装は
adapter 層、注入は usecase 層で行う。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: ULID ベースの IDGenerator を adapter で実装

**Files:**

- Modify: `go.mod`, `go.sum`（`oklog/ulid/v2` 追加）
- Create: `internal/adapter/idgen/ulid.go`
- Test: `internal/adapter/idgen/ulid_test.go`

- [ ] **Step 1: `oklog/ulid/v2` を依存に追加**

Run:
```bash
go get github.com/oklog/ulid/v2@v2.1.0
```

Expected: `go.mod` に `github.com/oklog/ulid/v2 v2.1.0` が direct dependency として追加される。

- [ ] **Step 2: 失敗テストを書く**

`internal/adapter/idgen/ulid_test.go`:

```go
package idgen_test

import (
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/stretchr/testify/require"
)

func TestULIDGenerator_New_NonEmpty(t *testing.T) {
	g := idgen.NewULID()
	id := g.New()
	require.NotEmpty(t, id)
	require.Len(t, id, 26, "ULID は 26 文字")
}

func TestULIDGenerator_New_Unique(t *testing.T) {
	g := idgen.NewULID()
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		id := g.New()
		_, dup := seen[id]
		require.Falsef(t, dup, "重複 ID 検出 i=%d id=%s", i, id)
		seen[id] = struct{}{}
	}
}

func TestULIDGenerator_New_ConcurrentSafe(t *testing.T) {
	g := idgen.NewULID()
	var wg sync.WaitGroup
	out := make(chan string, 200)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				out <- g.New()
			}
		}()
	}
	wg.Wait()
	close(out)
	seen := map[string]struct{}{}
	for id := range out {
		require.NotEmpty(t, id)
		_, dup := seen[id]
		require.Falsef(t, dup, "並行生成で重複: %s", id)
		seen[id] = struct{}{}
	}
}
```

- [ ] **Step 3: テストを走らせて失敗確認**

Run: `go test ./internal/adapter/idgen/...`
Expected: `package idgen` not found で FAIL

- [ ] **Step 4: `internal/adapter/idgen/ulid.go` を実装**

```go
// Package idgen は port.IDGenerator の adapter 実装を提供する。
package idgen

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ULIDGenerator は crypto/rand エントロピーで ULID を生成する。
// MonotonicReader を使うため同一ミリ秒内でも単調増加し、ロック付きで並行安全。
type ULIDGenerator struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// NewULID は本番用の ULIDGenerator を返す。エントロピー源は crypto/rand。
func NewULID() *ULIDGenerator {
	return &ULIDGenerator{
		entropy: ulid.Monotonic(rand.Reader, 0),
	}
}

// New は新しい ULID 文字列（26 文字、Crockford Base32）を返す。
func (g *ULIDGenerator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy).String()
}
```

- [ ] **Step 5: テストが pass することを確認**

Run: `go test ./internal/adapter/idgen/...`
Expected: PASS（3 件）

- [ ] **Step 6: コミット**

```bash
git add go.mod go.sum internal/adapter/idgen/
git commit -m "$(cat <<'EOF'
feat(adapter/idgen): ULID ベースの IDGenerator を追加

oklog/ulid/v2 を direct dependency に追加。Monotonic エントロピーで
同一ミリ秒内の単調増加と並行安全（sync.Mutex）を保証する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: テスト用フィクスチャ（HTML / header.json / data.json）を testdata に追加

**Files:**

- Create: `testdata/source_table_fixture.html`
- Create: `testdata/source_table_fixture_header.json`
- Create: `testdata/source_table_fixture_data.json`

これらは Task 8 (Repo SaveFetched / LoadCharts) と Task 9-10 (Fetcher) で使う最小フィクスチャ。実在表（Satellite 等）は Plan 3 で所持絞り込みのテストデータとして使うため触らない。

- [ ] **Step 1: HTML フィクスチャ作成**

`testdata/source_table_fixture.html`:

```html
<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <meta name="bmstable" content="source_table_fixture_header.json">
  <title>Fixture Table</title>
</head>
<body>
  <h1>Fixture</h1>
</body>
</html>
```

- [ ] **Step 2: header.json フィクスチャ作成**

`testdata/source_table_fixture_header.json`:

```json
{
  "name": "Fixture Table",
  "symbol": "fx",
  "data_url": "source_table_fixture_data.json",
  "level_order": ["0", "1", "2"]
}
```

- [ ] **Step 3: data.json フィクスチャ作成（3 譜面、表固有フィールド入り）**

`testdata/source_table_fixture_data.json`:

```json
[
  {
    "md5": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "sha256": "1111111111111111111111111111111111111111111111111111111111111111",
    "level": "0",
    "title": "First Song",
    "artist": "Artist A",
    "url": "https://example.com/first",
    "url_diff": "https://example.com/first/diff",
    "lr2_bmsid": 1001
  },
  {
    "md5": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "sha256": "",
    "level": "1",
    "title": "Second Song",
    "artist": "Artist B",
    "url": "https://example.com/second"
  },
  {
    "md5": "cccccccccccccccccccccccccccccccc",
    "level": "2",
    "title": "Third Song",
    "artist": "Artist C"
  }
]
```

- [ ] **Step 4: コミット**

```bash
git add testdata/source_table_fixture*.html testdata/source_table_fixture*.json
git commit -m "$(cat <<'EOF'
test(testdata): ソース表取り込みテスト用フィクスチャを追加

最小 HTML / header.json / data.json の3点セット。data.json には
表固有フィールド (url, url_diff, lr2_bmsid) を含めて raw_json
パススルーをテストできるようにする。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: SourceTableRepoSQL の CRUD（Create / Get / List / Update / Delete）

**Files:**

- Create: `internal/adapter/persistence/source_table_repo.go`
- Test: `internal/adapter/persistence/source_table_repo_test.go`

- [ ] **Step 1: 失敗テストを書く**

`internal/adapter/persistence/source_table_repo_test.go`:

```go
package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func setupSourceTableRepo(t *testing.T) *persistence.SourceTableRepoSQL {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))
	return persistence.NewSourceTableRepoSQL(db)
}

func TestSourceTableRepoSQL_CreateThenGet(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()

	in := domain.SourceTable{
		ID:              "01J0000000000000000000A",
		InputURL:        "https://example.com/table.html",
		InputKind:       domain.InputKindHTML,
		DisplayName:     "Example",
		LastFetchStatus: domain.FetchStatusNever,
	}
	id, err := r.Create(ctx, in)
	require.NoError(t, err)
	require.Equal(t, in.ID, id)

	got, err := r.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, in.InputURL, got.InputURL)
	require.Equal(t, domain.InputKindHTML, got.InputKind)
	require.Equal(t, "Example", got.DisplayName)
	require.Equal(t, domain.FetchStatusNever, got.LastFetchStatus)
	require.Nil(t, got.LastFetchedAt)
}

func TestSourceTableRepoSQL_Get_NotFoundError(t *testing.T) {
	r := setupSourceTableRepo(t)
	_, err := r.Get(context.Background(), "missing")
	require.Error(t, err)
}

func TestSourceTableRepoSQL_List_Empty(t *testing.T) {
	r := setupSourceTableRepo(t)
	out, err := r.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestSourceTableRepoSQL_List_OrdersBySortOrderThenCreatedAt(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	for i, id := range []string{"A", "B", "C"} {
		_, err := r.Create(ctx, domain.SourceTable{
			ID: id, InputURL: "u" + id, InputKind: domain.InputKindHeaderJSON,
			LastFetchStatus: domain.FetchStatusNever,
		})
		require.NoError(t, err)
		_ = i
	}
	out, err := r.List(ctx)
	require.NoError(t, err)
	require.Len(t, out, 3)
}

func TestSourceTableRepoSQL_Update_PersistsDisplayName(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "X", InputURL: "u", InputKind: domain.InputKindHeaderJSON,
		DisplayName: "old", LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
	got, err := r.Get(ctx, "X")
	require.NoError(t, err)
	got.DisplayName = "new"
	require.NoError(t, r.Update(ctx, got))
	after, err := r.Get(ctx, "X")
	require.NoError(t, err)
	require.Equal(t, "new", after.DisplayName)
}

func TestSourceTableRepoSQL_Delete_RemovesRow(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "Y", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
	require.NoError(t, r.Delete(ctx, "Y"))
	_, err = r.Get(ctx, "Y")
	require.Error(t, err)
}
```

(Task 6 / 7 で `time` / `errors` / `port` を使う追加テストが入るため、import 句は次タスクで拡張する。)

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/adapter/persistence/...`
Expected: `undefined: persistence.SourceTableRepoSQL` で FAIL

- [ ] **Step 3: `internal/adapter/persistence/source_table_repo.go` を実装（CRUD のみ、SaveFetched/MarkFetchError/LoadCharts は次タスク）**

```go
package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// SourceTableRepoSQL は source_table / source_table_chart を扱う port.SourceTableRepo の実装。
type SourceTableRepoSQL struct {
	db *sql.DB
}

// NewSourceTableRepoSQL は新しい SourceTableRepoSQL を作る。
func NewSourceTableRepoSQL(db *sql.DB) *SourceTableRepoSQL {
	return &SourceTableRepoSQL{db: db}
}

// Create は SourceTable を新規挿入し、ID を返す。
// CHECK 制約に引っかからないよう InputKind / LastFetchStatus はゼロ値時に既定値を補完する。
func (r *SourceTableRepoSQL) Create(ctx context.Context, in domain.SourceTable) (string, error) {
	if in.ID == "" {
		return "", errors.New("ID は必須です")
	}
	kind := in.InputKind
	if kind == "" {
		kind = domain.InputKindHeaderJSON
	}
	status := in.LastFetchStatus
	if status == "" {
		status = domain.FetchStatusNever
	}
	levelOrderJSON, err := json.Marshal(in.LevelOrder)
	if err != nil {
		return "", fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO source_table
		 (id, input_url, input_kind, display_name, name, symbol, level_order_json,
		  data_url, etag, last_fetched_at, last_fetch_status, last_fetch_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.InputURL, string(kind), in.DisplayName, in.Name, in.Symbol,
		string(levelOrderJSON), in.DataURL, in.ETag,
		fetchedAtToNullable(in.LastFetchedAt), string(status), in.LastFetchError,
	)
	if err != nil {
		return "", fmt.Errorf("insert source_table %q: %w", in.ID, err)
	}
	return in.ID, nil
}

// Get は ID で SourceTable を取得する。存在しない場合はエラー。
func (r *SourceTableRepoSQL) Get(ctx context.Context, id string) (domain.SourceTable, error) {
	row := r.db.QueryRowContext(ctx, sourceTableSelectColumns+` WHERE id = ?`, id)
	st, err := scanSourceTable(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SourceTable{}, fmt.Errorf("source_table %q が見つかりません", id)
	}
	if err != nil {
		return domain.SourceTable{}, err
	}
	return st, nil
}

// List は sort_order, created_at 順に SourceTable を返す。
func (r *SourceTableRepoSQL) List(ctx context.Context) ([]domain.SourceTable, error) {
	rows, err := r.db.QueryContext(ctx,
		sourceTableSelectColumns+` ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list source_table: %w", err)
	}
	defer rows.Close()
	var out []domain.SourceTable
	for rows.Next() {
		st, err := scanSourceTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Update は SourceTable のメタ情報を上書きする。fetched 系カラムも上書きするので、
// 通常用途以外で呼ぶ場合は最新の値を読み出してから書き戻すこと。
func (r *SourceTableRepoSQL) Update(ctx context.Context, t domain.SourceTable) error {
	levelOrderJSON, err := json.Marshal(t.LevelOrder)
	if err != nil {
		return fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE source_table SET
		   input_url=?, input_kind=?, display_name=?, name=?, symbol=?,
		   level_order_json=?, data_url=?, etag=?, last_fetched_at=?,
		   last_fetch_status=?, last_fetch_error=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.InputURL, string(t.InputKind), t.DisplayName, t.Name, t.Symbol,
		string(levelOrderJSON), t.DataURL, t.ETag, fetchedAtToNullable(t.LastFetchedAt),
		string(t.LastFetchStatus), t.LastFetchError, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update source_table %q: %w", t.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("source_table %q が見つかりません", t.ID)
	}
	return nil
}

// Delete は ID で行を削除する。存在しなくてもエラーにしない（冪等）。
func (r *SourceTableRepoSQL) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM source_table WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete source_table %q: %w", id, err)
	}
	return nil
}

// ---- ヘルパ ----

const sourceTableSelectColumns = `SELECT
	id, input_url, input_kind, display_name, name, symbol, level_order_json,
	data_url, etag, last_fetched_at, last_fetch_status, last_fetch_error
 FROM source_table`

// scanSourceTable は *sql.Row / *sql.Rows どちらでも使えるように Scanner で受ける。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSourceTable(s rowScanner) (domain.SourceTable, error) {
	var (
		st              domain.SourceTable
		levelOrderJSON  string
		lastFetchedAt   sql.NullString
		lastFetchStatus string
		inputKind       string
	)
	if err := s.Scan(
		&st.ID, &st.InputURL, &inputKind, &st.DisplayName, &st.Name, &st.Symbol,
		&levelOrderJSON, &st.DataURL, &st.ETag, &lastFetchedAt,
		&lastFetchStatus, &st.LastFetchError,
	); err != nil {
		return domain.SourceTable{}, err
	}
	st.InputKind = domain.InputKind(inputKind)
	st.LastFetchStatus = domain.FetchStatus(lastFetchStatus)
	if levelOrderJSON != "" && levelOrderJSON != "null" {
		if err := json.Unmarshal([]byte(levelOrderJSON), &st.LevelOrder); err != nil {
			return domain.SourceTable{}, fmt.Errorf("unmarshal level_order_json: %w", err)
		}
	}
	if lastFetchedAt.Valid && lastFetchedAt.String != "" {
		t, err := time.Parse(time.RFC3339, lastFetchedAt.String)
		if err == nil {
			st.LastFetchedAt = &t
		}
	}
	return st, nil
}

func fetchedAtToNullable(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
```

- [ ] **Step 4: テストが pass することを確認**

Run: `go test ./internal/adapter/persistence/...`
Expected: 既存の config_store / migrations テスト含めて全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/source_table_repo.go internal/adapter/persistence/source_table_repo_test.go
git commit -m "$(cat <<'EOF'
feat(adapter/persistence): SourceTableRepoSQL の CRUD を追加

Create / Get / List / Update / Delete を実装。level_order は JSON
文字列でカラム化、last_fetched_at は RFC3339 で保存。スキャンは
共通ヘルパに切り出し、後続の SaveFetched 等で再利用する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: SourceTableRepoSQL の `SaveFetched` と `MarkFetchError`

**Files:**

- Modify: `internal/adapter/persistence/source_table_repo.go`
- Modify: `internal/adapter/persistence/source_table_repo_test.go`

`SaveFetched` は Tx 内で「ヘッダー UPDATE → 譜面行 DELETE → 譜面行 INSERT」する。`NotModified=true` の場合は last_fetched_at と updated_at のみ更新し、譜面行は触らない。

- [ ] **Step 1: 追加の失敗テストを書く**

`internal/adapter/persistence/source_table_repo_test.go` の末尾に追加:

```go
func TestSourceTableRepoSQL_SaveFetched_UpdatesHeaderAndInsertsCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML,
		DisplayName: "user-name", LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)

	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	ft := port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "Fetched Name", Symbol: "fx",
			DataURL: "https://example.com/data.json", LevelOrder: []string{"0", "1"},
		},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "aaaa", SHA256: "1111", Level: "0", Title: "T0",
				Artist: "A0", Raw: map[string]any{"md5": "aaaa", "url": "u0"}},
			{Position: 1, MD5: "bbbb", Level: "1", Title: "T1",
				Artist: "A1", Raw: map[string]any{"md5": "bbbb"}},
		},
		ETag: `"etag-1"`,
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", ft, now))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, "Fetched Name", got.Name)
	require.Equal(t, "fx", got.Symbol)
	require.Equal(t, "user-name", got.DisplayName, "DisplayName はユーザー編集を維持")
	require.Equal(t, "https://example.com/data.json", got.DataURL)
	require.Equal(t, []string{"0", "1"}, got.LevelOrder)
	require.Equal(t, `"etag-1"`, got.ETag)
	require.Equal(t, domain.FetchStatusOK, got.LastFetchStatus)
	require.Equal(t, "", got.LastFetchError)
	require.NotNil(t, got.LastFetchedAt)
	require.True(t, got.LastFetchedAt.Equal(now))
}

func TestSourceTableRepoSQL_SaveFetched_ReplacesChartsOnSecondCall(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})

	first := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
			{Position: 1, MD5: "b", Level: "0", Raw: map[string]any{"md5": "b"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", first, time.Now()))

	second := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "x", Level: "0", Raw: map[string]any{"md5": "x"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", second, time.Now()))

	charts, err := r.LoadCharts(ctx, "Z")
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Equal(t, "x", charts[0].MD5)
}

func TestSourceTableRepoSQL_SaveFetched_NotModifiedKeepsCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	first := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
		},
		ETag: `"v1"`,
	}
	t0 := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.SaveFetched(ctx, "Z", first, t0))

	t1 := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.SaveFetched(ctx, "Z", port.FetchedTable{NotModified: true}, t1))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, domain.FetchStatusOK, got.LastFetchStatus)
	require.True(t, got.LastFetchedAt.Equal(t1))
	require.Equal(t, `"v1"`, got.ETag, "ETag は維持される")
	charts, _ := r.LoadCharts(ctx, "Z")
	require.Len(t, charts, 1)
	require.Equal(t, "a", charts[0].MD5)
}

func TestSourceTableRepoSQL_MarkFetchError_KeepsPreviousCharts(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, r.SaveFetched(ctx, "Z", port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "a", Level: "0", Raw: map[string]any{"md5": "a"}},
		},
	}, time.Now()))

	errAt := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.MarkFetchError(ctx, "Z", errors.New("boom"), errAt))

	got, err := r.Get(ctx, "Z")
	require.NoError(t, err)
	require.Equal(t, domain.FetchStatusError, got.LastFetchStatus)
	require.Equal(t, "boom", got.LastFetchError)
	require.True(t, got.LastFetchedAt.Equal(errAt))

	charts, _ := r.LoadCharts(ctx, "Z")
	require.Len(t, charts, 1, "失敗時もキャッシュは保持される（spec §8）")
}
```

ファイル先頭の import 句に以下を追加:

```go
import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/adapter/persistence/...`
Expected: `r.SaveFetched undefined` / `r.MarkFetchError undefined` / `r.LoadCharts undefined` で FAIL

- [ ] **Step 3: `SaveFetched` と `MarkFetchError` を `source_table_repo.go` に追加**

ファイル末尾に追加:

```go
// SaveFetched は取得結果を Tx 内で保存する。
// NotModified=true の場合は last_fetched_at / updated_at のみ更新し、譜面行は触らない。
func (r *SourceTableRepoSQL) SaveFetched(
	ctx context.Context, sourceID string, ft port.FetchedTable, fetchedAt time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	fetchedAtStr := fetchedAt.UTC().Format(time.RFC3339)

	if ft.NotModified {
		_, err = tx.ExecContext(ctx,
			`UPDATE source_table SET
			   last_fetched_at=?, last_fetch_status='ok', last_fetch_error='',
			   updated_at=datetime('now')
			 WHERE id=?`,
			fetchedAtStr, sourceID,
		)
		if err != nil {
			return fmt.Errorf("update source_table (not_modified) %q: %w", sourceID, err)
		}
		return tx.Commit()
	}

	levelOrderJSON, err := json.Marshal(ft.Header.LevelOrder)
	if err != nil {
		return fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE source_table SET
		   name=?, symbol=?, level_order_json=?, data_url=?, etag=?,
		   last_fetched_at=?, last_fetch_status='ok', last_fetch_error='',
		   updated_at=datetime('now')
		 WHERE id=?`,
		ft.Header.Name, ft.Header.Symbol, string(levelOrderJSON),
		ft.Header.DataURL, ft.ETag, fetchedAtStr, sourceID,
	)
	if err != nil {
		return fmt.Errorf("update source_table %q: %w", sourceID, err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM source_table_chart WHERE source_id=?`, sourceID); err != nil {
		return fmt.Errorf("delete charts %q: %w", sourceID, err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO source_table_chart
		 (source_id, position, md5, sha256, level, title, artist, raw_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert chart: %w", err)
	}
	defer stmt.Close()

	for _, c := range ft.Charts {
		rawJSON, err := json.Marshal(c.Raw)
		if err != nil {
			return fmt.Errorf("marshal raw[pos=%d]: %w", c.Position, err)
		}
		if _, err := stmt.ExecContext(ctx,
			sourceID, c.Position, c.MD5, c.SHA256, c.Level, c.Title, c.Artist, string(rawJSON),
		); err != nil {
			return fmt.Errorf("insert chart[pos=%d]: %w", c.Position, err)
		}
	}
	return tx.Commit()
}

// MarkFetchError は取得失敗を記録する。譜面行は触らない（前回成功時のキャッシュを保持）。
func (r *SourceTableRepoSQL) MarkFetchError(
	ctx context.Context, sourceID string, fetchErr error, fetchedAt time.Time,
) error {
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE source_table SET
		   last_fetched_at=?, last_fetch_status='error', last_fetch_error=?,
		   updated_at=datetime('now')
		 WHERE id=?`,
		fetchedAt.UTC().Format(time.RFC3339), msg, sourceID,
	)
	if err != nil {
		return fmt.Errorf("mark fetch error %q: %w", sourceID, err)
	}
	return nil
}
```

import 句に `"github.com/meta-BE/bms-random-table-compositor/internal/port"` を追加（既に時刻系で `time` は入っているはず）。

- [ ] **Step 4: テスト確認（LoadCharts はまだ未実装なので関連テストは FAIL のままでよい）**

Run: `go test ./internal/adapter/persistence/... -run 'SaveFetched|MarkFetchError'`
Expected: SaveFetched 系・MarkFetchError 系が PASS。LoadCharts を呼ぶ箇所は次タスクで pass する。

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/source_table_repo.go internal/adapter/persistence/source_table_repo_test.go
git commit -m "$(cat <<'EOF'
feat(adapter/persistence): SaveFetched と MarkFetchError を追加

SaveFetched は Tx 内で UPDATE → DELETE 譜面 → INSERT 譜面 の順で
原子的に保存する。NotModified=true の場合は last_fetched_at と
updated_at のみ更新。MarkFetchError は譜面行を触らないことで
spec §8 の「失敗時もキャッシュは保持」を実現する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: SourceTableRepoSQL の `LoadCharts`

**Files:**

- Modify: `internal/adapter/persistence/source_table_repo.go`
- Modify: `internal/adapter/persistence/source_table_repo_test.go`

- [ ] **Step 1: 追加の失敗テストを書く（既存テスト末尾に追加）**

```go
func TestSourceTableRepoSQL_LoadCharts_OrderByPosition(t *testing.T) {
	r := setupSourceTableRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, domain.SourceTable{
		ID: "Z", InputURL: "u", InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	ft := port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "n", Symbol: "s"},
		Charts: []domain.SourceChart{
			{Position: 2, MD5: "c", Level: "1", Title: "Tc",
				Raw: map[string]any{"md5": "c", "url": "uc"}},
			{Position: 0, MD5: "a", Level: "0", Title: "Ta",
				Raw: map[string]any{"md5": "a", "url": "ua", "lr2_bmsid": float64(7)}},
			{Position: 1, MD5: "b", Level: "0", Title: "Tb",
				Raw: map[string]any{"md5": "b"}},
		},
	}
	require.NoError(t, r.SaveFetched(ctx, "Z", ft, time.Now()))

	out, err := r.LoadCharts(ctx, "Z")
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, []int{0, 1, 2}, []int{out[0].Position, out[1].Position, out[2].Position})
	require.Equal(t, "Z", out[0].SourceID)
	require.Equal(t, "ua", out[0].Raw["url"], "raw_json はパススルー")
	require.Equal(t, float64(7), out[0].Raw["lr2_bmsid"])
}

func TestSourceTableRepoSQL_LoadCharts_EmptyForNoSource(t *testing.T) {
	r := setupSourceTableRepo(t)
	out, err := r.LoadCharts(context.Background(), "missing")
	require.NoError(t, err)
	require.Empty(t, out)
}
```

- [ ] **Step 2: テスト失敗確認**

Run: `go test ./internal/adapter/persistence/... -run LoadCharts`
Expected: `r.LoadCharts undefined` で FAIL

- [ ] **Step 3: `LoadCharts` を実装（ファイル末尾に追加）**

```go
// LoadCharts は source_table_chart を position 昇順で返す。
func (r *SourceTableRepoSQL) LoadCharts(ctx context.Context, sourceID string) ([]domain.SourceChart, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT position, md5, sha256, level, title, artist, raw_json
		 FROM source_table_chart
		 WHERE source_id=?
		 ORDER BY position ASC`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("load charts %q: %w", sourceID, err)
	}
	defer rows.Close()

	var out []domain.SourceChart
	for rows.Next() {
		var (
			c       domain.SourceChart
			rawJSON string
		)
		if err := rows.Scan(
			&c.Position, &c.MD5, &c.SHA256, &c.Level, &c.Title, &c.Artist, &rawJSON,
		); err != nil {
			return nil, err
		}
		c.SourceID = sourceID
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &c.Raw); err != nil {
				return nil, fmt.Errorf("unmarshal raw_json[pos=%d]: %w", c.Position, err)
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: 全テスト pass を確認**

Run: `go test ./internal/adapter/persistence/...`
Expected: 既存・新規テスト全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/source_table_repo.go internal/adapter/persistence/source_table_repo_test.go
git commit -m "$(cat <<'EOF'
feat(adapter/persistence): LoadCharts を追加

position 昇順で source_table_chart を取得し、raw_json を
map[string]any にデコードして Raw フィールドに詰め直す。
Plan 3 のピックエンジンが pass-through で使う想定。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: BMSTableFetcher の `FetchByHeader`（header.json + data.json + ETag）

**Files:**

- Create: `internal/adapter/gateway/bmstable_fetcher.go`
- Test: `internal/adapter/gateway/bmstable_fetcher_test.go`

`FetchByHeader` の責務:

1. `headerURL` を GET、JSON デコードして `BMSTableHeader` を得る
2. `header.DataURL` を `headerURL` ベースで絶対化（相対なら base に対して `ResolveReference`）
3. 絶対化後の `dataURL` を `If-None-Match: <etag>` 付きで GET
4. 304 → `NotModified=true` を返す
5. 200 → JSON デコード（`[]map[string]any`）して `SourceChart` 配列に変換、新 ETag と共に返す

`http.Client` のデフォルトリダイレクト追従（最大 10 回）で GAS 302 は吸収される。`golang.org/x/net/html` は次タスクの `FetchByHTML` で使う。

- [ ] **Step 1: 失敗テストを書く（httptest.Server で header / data / 304 / ETag を再現）**

`internal/adapter/gateway/bmstable_fetcher_test.go`:

```go
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
```

- [ ] **Step 2: 失敗確認**

Run: `go test ./internal/adapter/gateway/...`
Expected: `package gateway not declared` で FAIL

- [ ] **Step 3: `internal/adapter/gateway/bmstable_fetcher.go` を実装**

```go
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
```

- [ ] **Step 4: テスト pass を確認**

Run: `go test ./internal/adapter/gateway/...`
Expected: 5 件 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/gateway/
git commit -m "$(cat <<'EOF'
feat(adapter/gateway): BMSTableFetcher.FetchByHeader を実装

header.json → data.json の2段取得 + 相対 data_url 絶対化 +
If-None-Match による ETag 304 ハンドリング + http.Client の
デフォルトリダイレクト追従で GAS 302 を吸収する。

FetchByHTML は次タスクで実装（現時点では明示エラーを返す）。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: BMSTableFetcher の `FetchByHTML`（HTML→meta→header.json 委譲）

**Files:**

- Modify: `go.mod`, `go.sum`（`golang.org/x/net` を direct 依存に昇格）
- Modify: `internal/adapter/gateway/bmstable_fetcher.go`
- Modify: `internal/adapter/gateway/bmstable_fetcher_test.go`

`FetchByHTML` は HTML をストリーム解析し `<meta name="bmstable" content="...">` の content を取り出す。content が相対なら HTML URL を base に絶対化し、`FetchByHeader` に委譲する。

- [ ] **Step 1: `golang.org/x/net` を direct 依存に昇格**

Run:
```bash
go get golang.org/x/net/html
```

Expected: `go.mod` の `golang.org/x/net` 行から `// indirect` が外れる。

- [ ] **Step 2: テスト追加（既存ファイル末尾に）**

```go
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
```

- [ ] **Step 3: 失敗確認**

Run: `go test ./internal/adapter/gateway/... -run FetchByHTML`
Expected: 「未実装」エラーで FAIL

- [ ] **Step 4: `FetchByHTML` を本実装に差し替える + meta 抽出ヘルパを追加**

`internal/adapter/gateway/bmstable_fetcher.go` の中:

import 句に追加:
```go
"golang.org/x/net/html"
```

`FetchByHTML` の暫定実装を削除して以下に差し替える:

```go
// FetchByHTML は HTML を取得し <meta name="bmstable" content="..."> から
// header.json の URL を抽出して FetchByHeader に委譲する。
func (f *BMSTableFetcher) FetchByHTML(
	ctx context.Context, htmlURL string, etag string,
) (port.FetchedTable, error) {
	body, _, err := f.httpGet(ctx, htmlURL, "")
	if err != nil {
		return port.FetchedTable{}, fmt.Errorf("get html: %w", err)
	}
	defer body.Close()

	headerHref, err := extractBMSTableMeta(body)
	if err != nil {
		return port.FetchedTable{}, err
	}

	htmlBase, err := url.Parse(htmlURL)
	if err != nil {
		return port.FetchedTable{}, fmt.Errorf("parse htmlURL %q: %w", htmlURL, err)
	}
	headerURL, err := resolveURL(htmlBase, headerHref)
	if err != nil {
		return port.FetchedTable{}, err
	}
	return f.FetchByHeader(ctx, headerURL, etag)
}
```

ファイル末尾にヘルパを追加:

```go
// extractBMSTableMeta は HTML ストリームから最初の <meta name="bmstable">
// タグを探し、その content 属性値を返す。
func extractBMSTableMeta(r io.Reader) (string, error) {
	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return "", errors.New(`<meta name="bmstable"> が HTML 内に見つかりません`)
			}
			return "", z.Err()
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		if string(name) != "meta" || !hasAttr {
			continue
		}
		var (
			isBMSTable bool
			content    string
		)
		for {
			attrName, attrValue, more := z.TagAttr()
			switch string(attrName) {
			case "name":
				if string(attrValue) == "bmstable" {
					isBMSTable = true
				}
			case "content":
				content = string(attrValue)
			}
			if !more {
				break
			}
		}
		if isBMSTable && content != "" {
			return content, nil
		}
	}
}
```

`golang.org/x/net/html` の indirect が外れて direct に昇格していることを `go mod tidy` で確実にする:

Run: `go mod tidy`

- [ ] **Step 5: テスト pass を確認**

Run: `go test ./internal/adapter/gateway/...`
Expected: 既存5件 + 新規3件 全 PASS

- [ ] **Step 6: コミット**

```bash
git add go.mod go.sum internal/adapter/gateway/
git commit -m "$(cat <<'EOF'
feat(adapter/gateway): FetchByHTML で meta name=bmstable を抽出

golang.org/x/net/html を direct 依存に昇格して Tokenizer で
ストリーム解析。content が相対 URL の場合は HTML URL ベースで
絶対化し、FetchByHeader に委譲する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: SourceTableUseCase の CRUD（Add / List / Get / Remove / UpdateDisplayName）

**Files:**

- Create: `internal/usecase/source_table_usecase.go`
- Test: `internal/usecase/source_table_usecase_test.go`

UseCase 層は port のフェイクで完結するユニットテストを書く。Refresh 系は次タスクで追加。

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/source_table_usecase_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"io/ioutil"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakeSourceRepo は port.SourceTableRepo のテスト用実装。
type fakeSourceRepo struct {
	mu     sync.Mutex
	rows   map[string]domain.SourceTable
	charts map[string][]domain.SourceChart
	saved  map[string]port.FetchedTable
	errs   map[string]string
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{
		rows: map[string]domain.SourceTable{}, charts: map[string][]domain.SourceChart{},
		saved: map[string]port.FetchedTable{}, errs: map[string]string{},
	}
}

func (r *fakeSourceRepo) List(_ context.Context) ([]domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SourceTable, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeSourceRepo) Get(_ context.Context, id string) (domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return domain.SourceTable{}, errors.New("not found")
	}
	return v, nil
}

func (r *fakeSourceRepo) Create(_ context.Context, in domain.SourceTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[in.ID] = in
	return in.ID, nil
}

func (r *fakeSourceRepo) Update(_ context.Context, t domain.SourceTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[t.ID]; !ok {
		return errors.New("not found")
	}
	r.rows[t.ID] = t
	return nil
}

func (r *fakeSourceRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *fakeSourceRepo) SaveFetched(_ context.Context, id string, ft port.FetchedTable, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved[id] = ft
	row := r.rows[id]
	if !ft.NotModified {
		row.Name = ft.Header.Name
		row.Symbol = ft.Header.Symbol
		row.LevelOrder = ft.Header.LevelOrder
		row.DataURL = ft.Header.DataURL
		row.ETag = ft.ETag
		row.LastFetchError = ""
		r.charts[id] = ft.Charts
	}
	row.LastFetchedAt = &at
	row.LastFetchStatus = domain.FetchStatusOK
	r.rows[id] = row
	return nil
}

func (r *fakeSourceRepo) MarkFetchError(_ context.Context, id string, e error, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	row.LastFetchedAt = &at
	row.LastFetchStatus = domain.FetchStatusError
	row.LastFetchError = e.Error()
	r.rows[id] = row
	r.errs[id] = e.Error()
	return nil
}

func (r *fakeSourceRepo) LoadCharts(_ context.Context, id string) ([]domain.SourceChart, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.charts[id], nil
}

// fakeFetcher は port.SourceTableFetcher のテスト用実装。
type fakeFetcher struct {
	mu        sync.Mutex
	htmlCalls int
	headCalls int
	results   map[string]port.FetchedTable
	errs      map[string]error
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{results: map[string]port.FetchedTable{}, errs: map[string]error{}}
}

func (f *fakeFetcher) FetchByHTML(_ context.Context, u string, _ string) (port.FetchedTable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.htmlCalls++
	if e, ok := f.errs[u]; ok {
		return port.FetchedTable{}, e
	}
	return f.results[u], nil
}

func (f *fakeFetcher) FetchByHeader(_ context.Context, u string, _ string) (port.FetchedTable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headCalls++
	if e, ok := f.errs[u]; ok {
		return port.FetchedTable{}, e
	}
	return f.results[u], nil
}

// fakeIDGen は決定論的に ID を返す。
type fakeIDGen struct {
	ids []string
	i   int
}

func (g *fakeIDGen) New() string {
	v := g.ids[g.i]
	g.i++
	return v
}

func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioutil.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---- CRUD テスト ----

func TestSourceTableUseCase_Add_RejectsEmptyURL(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())
	_, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: ""})
	require.Error(t, err)
}

func TestSourceTableUseCase_Add_RejectsMalformedURL(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())
	_, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: "not-a-url"})
	require.Error(t, err)
}

func TestSourceTableUseCase_Add_DetectsHTMLByDefault(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-X"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/table.html",
	})
	require.NoError(t, err)
	require.Equal(t, "id-X", id)
	require.Equal(t, domain.InputKindHTML, repo.rows[id].InputKind)
	require.Equal(t, "", repo.rows[id].DisplayName,
		"DisplayName は初期値 空。取得後に Name で UI 側がフォールバック表示する")
	require.Equal(t, domain.FetchStatusNever, repo.rows[id].LastFetchStatus)
}

func TestSourceTableUseCase_Add_DetectsHeaderJSONByExtension(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Y"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/header.json",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind)
}

func TestSourceTableUseCase_Add_JSONExtCaseInsensitive(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Z"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/HEADER.JSON",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind)
}

func TestSourceTableUseCase_Add_QueryStringIgnoredForKind(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Q"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/header.json?cb=42",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind,
		"クエリ文字列は path 末尾の判定に影響しない")
}

func TestSourceTableUseCase_List_PassThrough(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a"}
	repo.rows["b"] = domain.SourceTable{ID: "b"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	out, err := uc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
}

func TestSourceTableUseCase_Remove_PassThrough(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.Remove(context.Background(), "a"))
	require.NotContains(t, repo.rows, "a")
}

func TestSourceTableUseCase_UpdateDisplayName_OverwritesField(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a", DisplayName: "old"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.UpdateDisplayName(context.Background(), "a", "new"))
	require.Equal(t, "new", repo.rows["a"].DisplayName)
}
```

- [ ] **Step 2: 失敗確認**

Run: `go test ./internal/usecase/...`
Expected: `undefined: usecase.SourceTableUseCase` で FAIL

- [ ] **Step 3: `internal/usecase/source_table_usecase.go` を実装（CRUD まで）**

```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// SourceTableUseCase はソース表の CRUD と取得（refresh）のビジネスロジックを束ねる。
type SourceTableUseCase struct {
	repo    port.SourceTableRepo
	fetcher port.SourceTableFetcher
	idGen   port.IDGenerator
	log     *slog.Logger
}

// NewSourceTableUseCase は新しい SourceTableUseCase を作る。
func NewSourceTableUseCase(
	repo port.SourceTableRepo,
	fetcher port.SourceTableFetcher,
	idGen port.IDGenerator,
	log *slog.Logger,
) *SourceTableUseCase {
	return &SourceTableUseCase{repo: repo, fetcher: fetcher, idGen: idGen, log: log}
}

// AddSourceTableInput は Add が受け取る入力。InputKind と DisplayName は
// それぞれ URL からの自動判別 / 取得後の Name フォールバックで埋めるため、
// ユーザーには入力させない。
type AddSourceTableInput struct {
	URL string
}

// Add は SourceTable を新規登録する。InputKind は URL の path 拡張子から判別する
// （`.json` で終われば HeaderJSON、それ以外は HTML）。実取得は呼び出し側が
// RefreshOne で行うため、DisplayName / Name / Symbol 等の表メタは初期値（空）
// で挿入される。フロントエンドは取得後に `displayName || name` の優先で表示する。
func (u *SourceTableUseCase) Add(ctx context.Context, in AddSourceTableInput) (string, error) {
	if in.URL == "" {
		return "", errors.New("URL は必須です")
	}
	kind, err := inferInputKind(in.URL)
	if err != nil {
		return "", err
	}
	id := u.idGen.New()
	st := domain.SourceTable{
		ID: id, InputURL: in.URL, InputKind: kind,
		LastFetchStatus: domain.FetchStatusNever,
	}
	return u.repo.Create(ctx, st)
}

// inferInputKind は URL を解析し、path 末尾が ".json"（大文字小文字無視）の場合は
// HeaderJSON、それ以外は HTML として扱う。GAS のような拡張子なしで JSON を返す URL
// は HTML 判定されてしまうが、header.json を返す GAS は実用上ほぼ存在しないため
// Phase 1 ではこの単純ルールで割り切る（必要に応じて将来 Content-Type ベースの
// フォールバックを追加）。
func inferInputKind(rawURL string) (domain.InputKind, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL のパースに失敗 %q: %w", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URL の形式が不正です: %s", rawURL)
	}
	if strings.HasSuffix(strings.ToLower(u.Path), ".json") {
		return domain.InputKindHeaderJSON, nil
	}
	return domain.InputKindHTML, nil
}

// List はすべての SourceTable を返す。
func (u *SourceTableUseCase) List(ctx context.Context) ([]domain.SourceTable, error) {
	return u.repo.List(ctx)
}

// Get は指定 ID の SourceTable を返す。
func (u *SourceTableUseCase) Get(ctx context.Context, id string) (domain.SourceTable, error) {
	return u.repo.Get(ctx, id)
}

// Remove は SourceTable を削除する。譜面行は外部キー ON DELETE CASCADE で連動削除される。
func (u *SourceTableUseCase) Remove(ctx context.Context, id string) error {
	return u.repo.Delete(ctx, id)
}

// UpdateDisplayName は表示名のみ書き換える（他フィールドは fetcher が更新する責務）。
func (u *SourceTableUseCase) UpdateDisplayName(ctx context.Context, id string, displayName string) error {
	st, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	st.DisplayName = displayName
	return u.repo.Update(ctx, st)
}
```

- [ ] **Step 4: テスト pass を確認**

Run: `go test ./internal/usecase/...`
Expected: 既存 ConfigUseCase テスト + 新規 8 件 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/source_table_usecase.go internal/usecase/source_table_usecase_test.go
git commit -m "$(cat <<'EOF'
feat(usecase): SourceTableUseCase の CRUD を追加

Add の入力は URL のみ。InputKind は URL path 拡張子から自動判別
（.json で終われば HeaderJSON、それ以外は HTML）。DisplayName は
初期値 空のまま挿入し、取得後にフロントエンドが Name で
フォールバック表示する責務とする。List / Get / Remove /
UpdateDisplayName は薄いラッパ。Refresh 系は次タスクで追加する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: SourceTableUseCase の `RefreshOne` / `RefreshAll`（並列度 4）

**Files:**

- Modify: `internal/usecase/source_table_usecase.go`
- Modify: `internal/usecase/source_table_usecase_test.go`

`RefreshOne` の挙動:

1. Repo から SourceTable を取得
2. InputKind に応じて Fetcher を呼び分け
3. 成功時: Repo.SaveFetched で保存（DisplayName はユーザー編集を残すため Repo 側で UPDATE 文に含めない）
4. 失敗時: Repo.MarkFetchError でエラーを記録（譜面はそのまま）

`RefreshAll` は List した SourceTable を並列度 4 で `RefreshOne` する。1 つ失敗しても残りは続行（spec §8）。

`Clock` は注入可能だが、Plan 2 では `time.Now` 固定でよい（テストは時刻自体を検証しない）。

- [ ] **Step 1: 失敗テスト追加**

`internal/usecase/source_table_usecase_test.go` 末尾に追加:

```go
func TestSourceTableUseCase_RefreshOne_Success_HTML(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-1"] = domain.SourceTable{
		ID: "id-1", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	}
	fetcher.results["https://x/h.html"] = port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "Hello", Symbol: "h"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "aaaa", Level: "0", Raw: map[string]any{"md5": "aaaa"}},
		},
		ETag: `"e1"`,
	}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-1"))
	require.Equal(t, 1, fetcher.htmlCalls)
	require.Equal(t, 0, fetcher.headCalls)
	require.Equal(t, "Hello", repo.rows["id-1"].Name)
	require.Equal(t, domain.FetchStatusOK, repo.rows["id-1"].LastFetchStatus)
}

func TestSourceTableUseCase_RefreshOne_Success_HeaderJSON(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-2"] = domain.SourceTable{
		ID: "id-2", InputURL: "https://x/header.json", InputKind: domain.InputKindHeaderJSON,
		LastFetchStatus: domain.FetchStatusNever,
	}
	fetcher.results["https://x/header.json"] = port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "By header", Symbol: "b"},
	}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-2"))
	require.Equal(t, 0, fetcher.htmlCalls)
	require.Equal(t, 1, fetcher.headCalls)
	require.Equal(t, "By header", repo.rows["id-2"].Name)
}

func TestSourceTableUseCase_RefreshOne_FetchError_MarksError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-3"] = domain.SourceTable{
		ID: "id-3", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK, // 前回は成功していた
	}
	fetcher.errs["https://x/h.html"] = errors.New("dns failure")
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-3"),
		"取得失敗そのものはエラー扱いにせず、MarkFetchError で記録する")
	require.Equal(t, domain.FetchStatusError, repo.rows["id-3"].LastFetchStatus)
	require.Equal(t, "dns failure", repo.rows["id-3"].LastFetchError)
}

func TestSourceTableUseCase_RefreshOne_NotModified(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-4"] = domain.SourceTable{
		ID: "id-4", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		ETag:            `"prev"`,
		LastFetchStatus: domain.FetchStatusOK,
	}
	fetcher.results["https://x/h.html"] = port.FetchedTable{NotModified: true, ETag: `"prev"`}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-4"))
	saved := repo.saved["id-4"]
	require.True(t, saved.NotModified)
}

func TestSourceTableUseCase_RefreshOne_UnknownIDIsError(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{}, newSilentLogger())
	require.Error(t, uc.RefreshOne(context.Background(), "missing"))
}

func TestSourceTableUseCase_RefreshAll_RunsAllAndContinuesOnError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		repo.rows[id] = domain.SourceTable{
			ID: id, InputURL: "https://x/" + id, InputKind: domain.InputKindHTML,
			LastFetchStatus: domain.FetchStatusNever,
		}
		fetcher.results["https://x/"+id] = port.FetchedTable{
			Header: domain.BMSTableHeader{Name: "n-" + id, Symbol: "s"},
		}
	}
	// 1 件だけわざと失敗させる
	fetcher.results["https://x/c"] = port.FetchedTable{}
	fetcher.errs["https://x/c"] = errors.New("boom")

	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshAll(context.Background()))

	require.Equal(t, domain.FetchStatusOK, repo.rows["a"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["b"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusError, repo.rows["c"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["d"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["e"].LastFetchStatus)
}
```

- [ ] **Step 2: 失敗確認**

Run: `go test ./internal/usecase/... -run RefreshOne`
Expected: `uc.RefreshOne undefined` で FAIL

- [ ] **Step 3: `RefreshOne` / `RefreshAll` を追加**

`internal/usecase/source_table_usecase.go` の末尾に追加 + import を更新（`sync` / `time` を追加）:

```go
// RefreshOne は単一 SourceTable を取得・保存する。
// 取得失敗自体はエラーとして返さず、Repo.MarkFetchError で記録して nil を返す
// （RefreshAll の途中で goroutine を止めないため）。
// Repo の永続化失敗は通常エラーで返す。
func (u *SourceTableUseCase) RefreshOne(ctx context.Context, id string) error {
	st, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()

	var (
		fetched  port.FetchedTable
		fetchErr error
	)
	switch st.InputKind {
	case domain.InputKindHTML:
		fetched, fetchErr = u.fetcher.FetchByHTML(ctx, st.InputURL, st.ETag)
	case domain.InputKindHeaderJSON:
		fetched, fetchErr = u.fetcher.FetchByHeader(ctx, st.InputURL, st.ETag)
	default:
		fetchErr = fmt.Errorf("不正な input_kind %q", st.InputKind)
	}

	if fetchErr != nil {
		u.log.Warn("source table refresh failed",
			"id", id, "url", st.InputURL, "err", fetchErr)
		if mErr := u.repo.MarkFetchError(ctx, id, fetchErr, now); mErr != nil {
			return fmt.Errorf("mark fetch error: %w", mErr)
		}
		return nil
	}

	if err := u.repo.SaveFetched(ctx, id, fetched, now); err != nil {
		u.log.Error("source table save failed", "id", id, "err", err)
		return fmt.Errorf("save fetched: %w", err)
	}
	u.log.Info("source table refreshed",
		"id", id, "name", fetched.Header.Name,
		"charts", len(fetched.Charts), "notModified", fetched.NotModified)
	return nil
}

// RefreshAll は登録済み全 SourceTable を並列度 4 で RefreshOne する。
// 個別失敗は Repo に記録され、RefreshAll 自体はエラーを返さない（List 失敗のみ伝播）。
func (u *SourceTableUseCase) RefreshAll(ctx context.Context) error {
	list, err := u.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list source tables: %w", err)
	}
	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, st := range list {
		id := st.ID
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := u.RefreshOne(ctx, id); err != nil {
				u.log.Warn("refresh all: one failed", "id", id, "err", err)
			}
		}()
	}
	wg.Wait()
	return nil
}
```

import 句に `"sync"` `"time"` を追加。

- [ ] **Step 4: テスト pass を確認**

Run: `go test ./internal/usecase/...`
Expected: 全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/source_table_usecase.go internal/usecase/source_table_usecase_test.go
git commit -m "$(cat <<'EOF'
feat(usecase): RefreshOne と RefreshAll を追加

RefreshOne は InputKind で Fetcher を分岐させ、取得成功は SaveFetched、
失敗は MarkFetchError に記録する。取得失敗そのものは error 扱いせず、
RefreshAll の goroutine を止めないようにする。RefreshAll は並列度 4
の semaphore で全件を取得する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: SourceTableHandler（Wails Bind）

**Files:**

- Create: `internal/app/handler/source_table_handler.go`
- Test: `internal/app/handler/source_table_handler_test.go`

UseCase をラップして Svelte から呼ぶための JSON 化された API を公開する。Wails のバインドは Go のメソッドを直接呼ぶが、フィールド名は exported な PascalCase が JSON では camelCase に変換される（json タグで明示する）。

- [ ] **Step 1: 失敗テストを書く**

`internal/app/handler/source_table_handler_test.go`:

```go
package handler_test

import (
	"context"
	"errors"
	"io/ioutil"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type sourceFakeRepo struct {
	mu   sync.Mutex
	rows map[string]domain.SourceTable
}

func newSourceFakeRepo() *sourceFakeRepo {
	return &sourceFakeRepo{rows: map[string]domain.SourceTable{}}
}
func (r *sourceFakeRepo) List(_ context.Context) ([]domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SourceTable, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}
func (r *sourceFakeRepo) Get(_ context.Context, id string) (domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return domain.SourceTable{}, errors.New("not found")
	}
	return v, nil
}
func (r *sourceFakeRepo) Create(_ context.Context, in domain.SourceTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[in.ID] = in
	return in.ID, nil
}
func (r *sourceFakeRepo) Update(_ context.Context, t domain.SourceTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[t.ID] = t
	return nil
}
func (r *sourceFakeRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}
func (r *sourceFakeRepo) SaveFetched(_ context.Context, _ string, _ port.FetchedTable, _ time.Time) error {
	return nil
}
func (r *sourceFakeRepo) MarkFetchError(_ context.Context, _ string, _ error, _ time.Time) error {
	return nil
}
func (r *sourceFakeRepo) LoadCharts(_ context.Context, _ string) ([]domain.SourceChart, error) {
	return nil, nil
}

type sourceFakeFetcher struct{}

func (sourceFakeFetcher) FetchByHTML(_ context.Context, _ string, _ string) (port.FetchedTable, error) {
	return port.FetchedTable{}, nil
}
func (sourceFakeFetcher) FetchByHeader(_ context.Context, _ string, _ string) (port.FetchedTable, error) {
	return port.FetchedTable{}, nil
}

type sourceFakeIDGen struct{ next string }

func (g *sourceFakeIDGen) New() string { return g.next }

func newSilentHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioutil.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newSourceTableHandler(t *testing.T) (*handler.SourceTableHandler, *sourceFakeRepo) {
	t.Helper()
	repo := newSourceFakeRepo()
	uc := usecase.NewSourceTableUseCase(
		repo, sourceFakeFetcher{}, &sourceFakeIDGen{next: "id-test"}, newSilentHandlerLogger(),
	)
	return handler.NewSourceTableHandler(uc), repo
}

func TestSourceTableHandler_AddSourceTable_DetectsHTML(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	id, err := h.AddSourceTable(handler.AddSourceTableRequest{
		URL: "https://example.com/table.html",
	})
	require.NoError(t, err)
	require.Equal(t, "id-test", id)
	require.Equal(t, domain.InputKindHTML, repo.rows["id-test"].InputKind)
	require.Equal(t, "", repo.rows["id-test"].DisplayName)
}

func TestSourceTableHandler_AddSourceTable_DetectsHeaderJSON(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	_, err := h.AddSourceTable(handler.AddSourceTableRequest{
		URL: "https://example.com/header.json",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows["id-test"].InputKind)
}

func TestSourceTableHandler_AddSourceTable_RejectsEmptyURL(t *testing.T) {
	h, _ := newSourceTableHandler(t)
	_, err := h.AddSourceTable(handler.AddSourceTableRequest{URL: ""})
	require.Error(t, err)
}

func TestSourceTableHandler_ListSourceTables_ReturnsDTOs(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	repo.rows["x"] = domain.SourceTable{
		ID: "x", InputURL: "u", InputKind: domain.InputKindHTML,
		DisplayName: "Disp", Name: "Name", Symbol: "sym",
		LevelOrder: []string{"0", "1"}, DataURL: "https://x/data.json",
		LastFetchedAt: &now, LastFetchStatus: domain.FetchStatusOK,
	}
	out, err := h.ListSourceTables()
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "x", out[0].ID)
	require.Equal(t, "u", out[0].InputURL)
	require.Equal(t, "html", out[0].InputKind)
	require.Equal(t, "Disp", out[0].DisplayName)
	require.Equal(t, []string{"0", "1"}, out[0].LevelOrder)
	require.Equal(t, "ok", out[0].LastFetchStatus)
	require.NotEmpty(t, out[0].LastFetchedAt)
}

func TestSourceTableHandler_DeleteSourceTable(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	repo.rows["x"] = domain.SourceTable{ID: "x"}
	require.NoError(t, h.DeleteSourceTable("x"))
	require.NotContains(t, repo.rows, "x")
}

func TestSourceTableHandler_UpdateSourceTableDisplayName(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	repo.rows["x"] = domain.SourceTable{ID: "x", DisplayName: "old"}
	require.NoError(t, h.UpdateSourceTableDisplayName("x", "new"))
	require.Equal(t, "new", repo.rows["x"].DisplayName)
}
```

- [ ] **Step 2: 失敗確認**

Run: `go test ./internal/app/handler/...`
Expected: `undefined: handler.SourceTableHandler` で FAIL

- [ ] **Step 3: `internal/app/handler/source_table_handler.go` を実装**

```go
package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// SourceTableHandler は Wails Bind 経由でソース表 API を公開する。
type SourceTableHandler struct {
	uc  *usecase.SourceTableUseCase
	ctx context.Context
}

// NewSourceTableHandler は新しい SourceTableHandler を作る。
func NewSourceTableHandler(uc *usecase.SourceTableUseCase) *SourceTableHandler {
	return &SourceTableHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *SourceTableHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// SourceTableDTO はフロントエンドに返す JSON 構造体。
type SourceTableDTO struct {
	ID              string   `json:"id"`
	InputURL        string   `json:"inputUrl"`
	InputKind       string   `json:"inputKind"`
	DisplayName     string   `json:"displayName"`
	Name            string   `json:"name"`
	Symbol          string   `json:"symbol"`
	LevelOrder      []string `json:"levelOrder"`
	DataURL         string   `json:"dataUrl"`
	LastFetchedAt   string   `json:"lastFetchedAt"`
	LastFetchStatus string   `json:"lastFetchStatus"`
	LastFetchError  string   `json:"lastFetchError"`
}

func toSourceTableDTO(st domain.SourceTable) SourceTableDTO {
	var lastFetchedAt string
	if st.LastFetchedAt != nil {
		lastFetchedAt = st.LastFetchedAt.UTC().Format(time.RFC3339)
	}
	levelOrder := st.LevelOrder
	if levelOrder == nil {
		levelOrder = []string{}
	}
	return SourceTableDTO{
		ID: st.ID, InputURL: st.InputURL, InputKind: string(st.InputKind),
		DisplayName: st.DisplayName, Name: st.Name, Symbol: st.Symbol,
		LevelOrder: levelOrder, DataURL: st.DataURL,
		LastFetchedAt:   lastFetchedAt,
		LastFetchStatus: string(st.LastFetchStatus),
		LastFetchError:  st.LastFetchError,
	}
}

// ListSourceTables は登録済みソース表をすべて返す。
func (h *SourceTableHandler) ListSourceTables() ([]SourceTableDTO, error) {
	list, err := h.uc.List(h.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceTableDTO, 0, len(list))
	for _, st := range list {
		out = append(out, toSourceTableDTO(st))
	}
	return out, nil
}

// AddSourceTableRequest は AddSourceTable のリクエスト DTO。
// InputKind は URL から自動判別、DisplayName は取得後に Name で
// フォールバック表示するため、入力は URL のみ。
type AddSourceTableRequest struct {
	URL string `json:"url"`
}

// AddSourceTable は新規ソース表を登録し、ID を返す（取得は別途 Refresh で行う）。
func (h *SourceTableHandler) AddSourceTable(req AddSourceTableRequest) (string, error) {
	return h.uc.Add(h.ctx, usecase.AddSourceTableInput{URL: req.URL})
}

// RefreshSourceTable は指定 ID のソース表を取得・保存する。
func (h *SourceTableHandler) RefreshSourceTable(id string) error {
	return h.uc.RefreshOne(h.ctx, id)
}

// RefreshAllSourceTables は登録済み全ソース表を並列度 4 で更新する。
func (h *SourceTableHandler) RefreshAllSourceTables() error {
	return h.uc.RefreshAll(h.ctx)
}

// DeleteSourceTable は指定 ID のソース表を削除する（譜面は CASCADE で消える）。
func (h *SourceTableHandler) DeleteSourceTable(id string) error {
	return h.uc.Remove(h.ctx, id)
}

// UpdateSourceTableDisplayName は表示名のみ書き換える。
func (h *SourceTableHandler) UpdateSourceTableDisplayName(id string, displayName string) error {
	return h.uc.UpdateDisplayName(h.ctx, id, displayName)
}
```

- [ ] **Step 4: テスト pass を確認**

Run: `go test ./internal/app/handler/...`
Expected: 既存 ConfigHandler テスト + 新規 6 件 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/app/handler/source_table_handler.go internal/app/handler/source_table_handler_test.go
git commit -m "$(cat <<'EOF'
feat(app/handler): SourceTableHandler の Wails Bind を追加

ListSourceTables / AddSourceTable / RefreshSourceTable /
RefreshAllSourceTables / DeleteSourceTable /
UpdateSourceTableDisplayName を公開。AddSourceTable の入力は URL
のみ（InputKind は usecase 側で URL 拡張子から自動判別）。
フロントエンドは camelCase の JSON タグ経由で受け取る。time.Time
は RFC3339 文字列に変換。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: `bootstrap.go` と `main.go` を拡張して新しいハンドラを配線

**Files:**

- Modify: `internal/app/bootstrap.go`
- Modify: `main.go`

`Services` 構造体に `SourceTableHandler` と `SourceTableUseCase`（OnStartup から `RefreshAll` を呼ぶため）を追加。`Bootstrap` 内で Repo / Fetcher / IDGen / UseCase / Handler を順に組み立てる。

- [ ] **Step 1: `internal/app/bootstrap.go` を更新**

ファイル全体を以下に置き換える:

```go
// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/gateway"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// Services はアプリ全体で共有する依存を保持する。
type Services struct {
	DB                 *sql.DB
	Logger             *slog.Logger
	LoggerClose        logger.CloseFunc
	ConfigHandler      *handler.ConfigHandler
	SourceTableHandler *handler.SourceTableHandler
	SourceTableUseCase *usecase.SourceTableUseCase
}

// Bootstrap は Services を構築する（DB接続・マイグレーション・ロガー・各UseCase初期化）。
// シングルインスタンス制御は Wails の SingleInstanceLock オプションに任せる。
func Bootstrap() (*Services, error) {
	// 1. Logger
	logDir, err := paths.LogDir()
	if err != nil {
		return nil, fmt.Errorf("log dir: %w", err)
	}
	lg, closeLog, err := logger.New(logger.Options{
		LogDir:     logDir,
		MaxSizeMB:  50,
		MaxBackups: 7,
		MaxAgeDays: 7,
	})
	if err != nil {
		return nil, fmt.Errorf("logger init: %w", err)
	}

	// 2. DB と マイグレーション
	dbPath, err := paths.DBPath()
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("db path: %w", err)
	}
	db, err := persistence.OpenDB(dbPath)
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := persistence.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = closeLog()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	// 3. UseCase / Handler 配線
	configStore := persistence.NewConfigStoreSQL(db)
	configUC := usecase.NewConfigUseCase(configStore)
	configHandler := handler.NewConfigHandler(configUC)

	sourceRepo := persistence.NewSourceTableRepoSQL(db)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	fetcher := gateway.NewBMSTableFetcher(httpClient, lg)
	idGen := idgen.NewULID()
	sourceUC := usecase.NewSourceTableUseCase(sourceRepo, fetcher, idGen, lg)
	sourceHandler := handler.NewSourceTableHandler(sourceUC)

	lg.Info("bootstrap complete", "db", dbPath, "logDir", logDir)

	return &Services{
		DB:                 db,
		Logger:             lg,
		LoggerClose:        closeLog,
		ConfigHandler:      configHandler,
		SourceTableHandler: sourceHandler,
		SourceTableUseCase: sourceUC,
	}, nil
}

// Close は Services が保持する全リソースを開放する。
func (s *Services) Close() {
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
```

- [ ] **Step 2: `main.go` の Bind 配列に `SourceTableHandler` を追加**

`Bind: []any{` ブロックを以下に変更:

```go
		Bind: []any{
			myApp,
			services.ConfigHandler,
			services.SourceTableHandler,
		},
```

- [ ] **Step 3: ビルド確認**

Run: `go build ./...`
Expected: エラーなし。

Run: `go test ./...`
Expected: 全 PASS。

- [ ] **Step 4: コミット**

```bash
git add internal/app/bootstrap.go main.go
git commit -m "$(cat <<'EOF'
feat(app): Bootstrap で SourceTable 配線を追加

http.Client (30s timeout) + BMSTableFetcher + ULIDGenerator +
SourceTableUseCase + SourceTableHandler を配線。Services に
SourceTableUseCase も保持し、OnStartup から RefreshAll を呼べる
ようにする（次タスクで利用）。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: `app.go` の `OnStartup` でバックグラウンド `RefreshAll` を起動

**Files:**

- Modify: `app.go`

ウィンドウが開いた直後に全ソース表を非同期更新する。Svelte 側は完了通知を `EventsOn("source_table:refresh_all_done", ...)` で受けて再ロードする。

- [ ] **Step 1: `app.go` の `startup` を更新**

`app.go` の `startup` メソッドを以下に置き換える:

```go
// startup は OnStartup で呼ばれる。ハンドラに ctx を引き渡し、ソース表の
// バックグラウンド更新を起動する。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.services.ConfigHandler.SetContext(ctx)
	a.services.SourceTableHandler.SetContext(ctx)
	a.services.Logger.Info("wails startup")

	go func() {
		a.services.Logger.Info("startup refresh all begin")
		if err := a.services.SourceTableUseCase.RefreshAll(ctx); err != nil {
			a.services.Logger.Warn("startup refresh all failed", "err", err)
		}
		a.services.Logger.Info("startup refresh all done")
		wailsruntime.EventsEmit(ctx, "source_table:refresh_all_done")
	}()
}
```

- [ ] **Step 2: ビルド確認**

Run: `go build ./...`
Expected: エラーなし。

- [ ] **Step 3: コミット**

```bash
git add app.go
git commit -m "$(cat <<'EOF'
feat(app): OnStartup で RefreshAll をバックグラウンド起動

ウィンドウ表示直後に goroutine で全ソース表を更新する。
完了は wails EventsEmit("source_table:refresh_all_done") で
通知し、フロントエンドが受信して一覧を再ロードする。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: フロントエンド `api.ts` 拡張

**Files:**

- Modify: `frontend/src/lib/api.ts`

- [ ] **Step 1: `frontend/src/lib/api.ts` を以下に置き換える**

```typescript
// Wails Bind のラッパ。生成型の細かい変動を吸収するために薄く包む。
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
} from '../../wailsjs/go/handler/ConfigHandler';
import {
  ListSourceTables,
  AddSourceTable,
  RefreshSourceTable,
  RefreshAllSourceTables,
  DeleteSourceTable,
  UpdateSourceTableDisplayName,
} from '../../wailsjs/go/handler/SourceTableHandler';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

export type ServerConfig = {
  port: number;
  songdataDbPath: string;
};

export type SourceTableDTO = {
  id: string;
  inputUrl: string;
  inputKind: 'html' | 'header_json';
  displayName: string;
  name: string;
  symbol: string;
  levelOrder: string[];
  dataUrl: string;
  lastFetchedAt: string;
  lastFetchStatus: 'never' | 'ok' | 'error';
  lastFetchError: string;
};

// AddSourceTable の入力は URL のみ。InputKind はバックエンドで URL 拡張子から
// 自動判別し、DisplayName は取得後に Name でフォールバック表示する責務を
// フロントエンドが持つ。
export type AddSourceTableRequest = {
  url: string;
};

export const api = {
  // ---- 設定 ----
  getServerConfig(): Promise<ServerConfig> {
    return GetServerConfig() as Promise<ServerConfig>;
  },
  setServerPort(port: number): Promise<void> {
    return SetServerPort(port);
  },
  setSongdataDBPath(path: string): Promise<void> {
    return SetSongdataDBPath(path);
  },
  // ---- ソース表 ----
  listSourceTables(): Promise<SourceTableDTO[]> {
    return ListSourceTables() as Promise<SourceTableDTO[]>;
  },
  addSourceTable(req: AddSourceTableRequest): Promise<string> {
    return AddSourceTable(req) as Promise<string>;
  },
  refreshSourceTable(id: string): Promise<void> {
    return RefreshSourceTable(id);
  },
  refreshAllSourceTables(): Promise<void> {
    return RefreshAllSourceTables();
  },
  deleteSourceTable(id: string): Promise<void> {
    return DeleteSourceTable(id);
  },
  updateSourceTableDisplayName(id: string, displayName: string): Promise<void> {
    return UpdateSourceTableDisplayName(id, displayName);
  },
  // ---- イベント ----
  onSourceTableRefreshAllDone(cb: () => void): () => void {
    EventsOn('source_table:refresh_all_done', cb);
    return () => EventsOff('source_table:refresh_all_done');
  },
};
```

- [ ] **Step 2: TypeScript 型チェック**

Run: `cd frontend && npx tsc --noEmit && cd ..`
Expected: エラーなし。Wails Bind 生成物 (`frontend/wailsjs/`) は `wails dev` または `wails build` 実行時に生成されるため、初回はそちらを先に走らせる必要がある。生成物が無い状態で型チェックすると `Cannot find module` になるので、Step 3 の確認手順を先に実行してもよい。

- [ ] **Step 3: コミット**

```bash
git add frontend/src/lib/api.ts
git commit -m "$(cat <<'EOF'
feat(frontend): api.ts にソース表 API ラッパを追加

ListSourceTables / AddSourceTable / RefreshSourceTable /
RefreshAllSourceTables / DeleteSourceTable /
UpdateSourceTableDisplayName + 起動時更新完了イベントの購読を
api オブジェクトに追加する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: ソース表管理タブ + App.svelte のタブ切替

**Files:**

- Create: `frontend/src/lib/tabs/SourceTablesTab.svelte`
- Modify: `frontend/src/App.svelte`

最低限のリスト+追加+操作 UI。Plan 4 でデザイン整備予定。

- [ ] **Step 1: `frontend/src/lib/tabs/SourceTablesTab.svelte` を新規作成**

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type SourceTableDTO, type AddSourceTableRequest } from '../api';

  let rows: SourceTableDTO[] = [];
  let loading = false;
  let listError = '';
  let addError = '';
  let newUrl = '';
  let busy: Record<string, boolean> = {};
  let unsubscribe: (() => void) | null = null;

  async function load() {
    loading = true;
    listError = '';
    try {
      rows = await api.listSourceTables();
    } catch (e: any) {
      listError = `読み込みエラー: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function add() {
    addError = '';
    if (!newUrl) {
      addError = 'URL を入力してください';
      return;
    }
    const req: AddSourceTableRequest = { url: newUrl };
    try {
      const id = await api.addSourceTable(req);
      newUrl = '';
      await load();
      // 追加直後に取得を試みる（バックグラウンド）
      busy = { ...busy, [id]: true };
      api
        .refreshSourceTable(id)
        .catch((e) => console.warn('initial refresh failed', e))
        .finally(async () => {
          busy = { ...busy, [id]: false };
          await load();
        });
    } catch (e: any) {
      addError = String(e);
    }
  }

  async function refresh(id: string) {
    busy = { ...busy, [id]: true };
    try {
      await api.refreshSourceTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = { ...busy, [id]: false };
    }
  }

  async function refreshAll() {
    loading = true;
    try {
      await api.refreshAllSourceTables();
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      loading = false;
    }
  }

  async function remove(id: string) {
    if (!confirm('このソース表を削除しますか？（紐づく譜面キャッシュも消えます）')) {
      return;
    }
    try {
      await api.deleteSourceTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    }
  }

  async function renameRow(id: string, displayName: string) {
    try {
      await api.updateSourceTableDisplayName(id, displayName);
      await load();
    } catch (e: any) {
      listError = String(e);
    }
  }

  function statusLabel(s: SourceTableDTO['lastFetchStatus']): string {
    switch (s) {
      case 'ok':
        return '取得済み';
      case 'error':
        return 'エラー';
      default:
        return '未取得';
    }
  }

  onMount(() => {
    load();
    unsubscribe = api.onSourceTableRefreshAllDone(() => {
      load();
    });
  });

  onDestroy(() => {
    if (unsubscribe) unsubscribe();
  });
</script>

<section class="tab">
  <h2>ソース表</h2>

  <div class="add-form">
    <label class="row">
      <span class="label">URL（HTML / header.json はパス末尾の拡張子で自動判別）</span>
      <input type="text" bind:value={newUrl} placeholder="https://example.com/table.html" />
    </label>
    <div class="actions">
      <button on:click={add} disabled={!newUrl}>追加</button>
      <button on:click={refreshAll} disabled={loading}>すべて再取得</button>
    </div>
    {#if addError}<p class="message err">{addError}</p>{/if}
  </div>

  {#if loading}
    <p>読み込み中...</p>
  {/if}
  {#if listError}<p class="message err">{listError}</p>{/if}

  <table>
    <thead>
      <tr>
        <th>表示名</th>
        <th>表名 / 略称</th>
        <th>状態</th>
        <th>最終取得</th>
        <th>URL</th>
        <th>操作</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as r (r.id)}
        <tr class:row-error={r.lastFetchStatus === 'error'}>
          <td>
            <input
              type="text"
              value={r.displayName}
              placeholder={r.name || '(取得中)'}
              on:change={(e) => renameRow(r.id, (e.currentTarget as HTMLInputElement).value)}
            />
          </td>
          <td>{r.name || '(取得中)'} / {r.symbol || ''}</td>
          <td>
            <span class="badge badge-{r.lastFetchStatus}">{statusLabel(r.lastFetchStatus)}</span>
            {#if r.lastFetchStatus === 'error'}
              <span class="err-detail" title={r.lastFetchError}>?</span>
            {/if}
          </td>
          <td>{r.lastFetchedAt || '-'}</td>
          <td class="url-cell" title={r.inputUrl}>{r.inputUrl}</td>
          <td>
            <button on:click={() => refresh(r.id)} disabled={busy[r.id]}>更新</button>
            <button on:click={() => remove(r.id)}>削除</button>
          </td>
        </tr>
      {/each}
      {#if rows.length === 0 && !loading}
        <tr>
          <td colspan="6" class="empty">登録なし</td>
        </tr>
      {/if}
    </tbody>
  </table>
</section>

<style>
  .tab { padding: 16px; }
  h2 { margin-top: 0; font-size: 16px; }
  .add-form { border: 1px solid #e0e0e0; padding: 12px; border-radius: 6px; margin-bottom: 16px; }
  .row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
  .label { font-size: 13px; color: #555; }
  input[type="text"], select { padding: 6px 8px; font-size: 13px; }
  .actions { display: flex; gap: 8px; margin-top: 8px; }
  button { padding: 4px 10px; font-size: 13px; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: 0.6; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { border-bottom: 1px solid #eee; padding: 6px 8px; text-align: left; vertical-align: middle; }
  th { background: #fafafa; }
  .url-cell { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { padding: 2px 8px; border-radius: 3px; font-size: 12px; }
  .badge-never { background: #eee; color: #666; }
  .badge-ok    { background: #e8f5e9; color: #2e7d32; }
  .badge-error { background: #ffebee; color: #b71c1c; }
  .err-detail  { color: #b71c1c; cursor: help; margin-left: 4px; }
  .row-error td { background: #fff8f8; }
  .empty { color: #999; text-align: center; padding: 16px; }
  .message.err { color: #b71c1c; margin: 8px 0 0; }
</style>
```

- [ ] **Step 2: `frontend/src/App.svelte` をタブ切替化**

```svelte
<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
  import SourceTablesTab from './lib/tabs/SourceTablesTab.svelte';

  type TabKey = 'server' | 'source-tables';
  let active: TabKey = 'server';
</script>

<main>
  <header>
    <h1>BMS Random Table Compositor</h1>
    <nav>
      <button class:active={active === 'server'} on:click={() => (active = 'server')}>サーバ設定</button>
      <button class:active={active === 'source-tables'} on:click={() => (active = 'source-tables')}>ソース表</button>
    </nav>
  </header>
  {#if active === 'server'}
    <ServerTab />
  {:else if active === 'source-tables'}
    <SourceTablesTab />
  {/if}
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    color: #1b2636;
    min-height: 100vh;
  }
  header {
    padding: 12px 16px;
    border-bottom: 1px solid #e0e0e0;
  }
  header h1 { margin: 0 0 8px; font-size: 18px; }
  nav { display: flex; gap: 4px; }
  nav button {
    padding: 4px 12px;
    border: 1px solid #ccc;
    background: #fafafa;
    cursor: pointer;
    font-size: 13px;
  }
  nav button.active {
    background: #1b2636;
    color: #fff;
    border-color: #1b2636;
  }
</style>
```

- [ ] **Step 3: フロントエンドビルド確認**

Run:
```bash
cd frontend && npm run build && cd ..
```

Expected: エラーなし。

`wails build` を走らせて Bind 自動生成された `wailsjs/go/handler/SourceTableHandler.ts` が出ることも確認する:

Run: `wails build`
Expected: 成功。`build/bin/bms-random-table-compositor.app` が生成される。

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/tabs/SourceTablesTab.svelte frontend/src/App.svelte
git commit -m "$(cat <<'EOF'
feat(frontend): ソース表管理タブを追加

App.svelte をタブ切替（サーバ設定 / ソース表）に変更。
SourceTablesTab の登録フォームは URL のみ。一覧の表示名カラムは
displayName が空なら placeholder で取得済み name を表示し、編集も
可能にする。状態は never/ok/error のバッジで色分けし、起動時
RefreshAll の完了イベントで自動リロードする。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: 実機検証（4 表取り込み + Windows ビルド）

このタスクはコード変更を伴わない手動検証 + 必要に応じての修正コミット。

- [ ] **Step 1: macOS 開発機での `wails dev` 検証**

Run: `wails dev`

確認項目:

- [ ] 起動 → サーバ設定タブが表示される（既存の Plan 1 動作）
- [ ] 「ソース表」タブに切替 → 登録なしの空表
- [ ] 以下 4 表を順に登録（URL のみ入力。`.html` 終端なので自動判別で全て HTML 扱い）。実際の URL は spec の冒頭セクション参照の有名表（Satellite / Stella / 発狂 / Solomon）を使う:
  - Satellite: `https://stellabms.xyz/sl/table.html`
  - Stella: `https://stellabms.xyz/st/table.html`
  - 発狂: `http://www.ribbit.xyz/bms/tables/insane.html`
  - Solomon: `https://rattoto10.web.fc2.com/sol1/table.html`
  - **注**: URL が変わっている場合は実在 URL に差し替える
- [ ] 各登録直後にバックグラウンド更新が走り、`badge-ok` か `badge-error` に切り替わる
- [ ] 取得成功した表は表示名（or 取得 name）と最終取得時刻が表示される
- [ ] 失敗した表は赤バッジと `?` ホバーでエラー文が見える、しかし**前回成功時のキャッシュは保持される**（→ もう一度更新してもキャッシュが消えていない）
- [ ] 「すべて再取得」ボタン → 全表が並列で再取得される
- [ ] 1 件削除 → 行が消える
- [ ] 表示名インラインを編集 → 値が保存される（タブ離脱→戻ると保持）
- [ ] アプリ再起動 → 起動時 `RefreshAll` が走り、ETag が一致する表は `last_fetched_at` だけ更新（譜面行は変わらない、`compositor.db` を SQLite クライアントで `SELECT COUNT(*) FROM source_table_chart WHERE source_id = ?` で確認可能）
- [ ] `logs/YYYY-MM-DD.log` に `source table refreshed` / `source table refresh failed` の行が記録される
- [ ] 既存の Plan 1 動作（ポート保存・トレイ常駐・SingleInstanceLock）が無回帰で動く

- [ ] **Step 2: 不具合修正があればコミット**

不具合があれば該当タスクのファイルを編集 → テスト追加 → コミット。フィックスタスクは個別にコミットメッセージを書く（例 `fix(usecase): NotModified 時の ETag 上書きを防ぐ`）。

- [ ] **Step 3: Windows ビルドを発火**

Run:
```bash
gh workflow run build-windows.yml --ref main
sleep 3
gh run list --workflow build-windows.yml --branch main --limit 1 --json databaseId,status
```

実行 ID をメモ（以下 `<run-id>`）。

- [ ] **Step 4: 完了待機 + Artifact 取得**

Run:
```bash
gh run watch <run-id> --exit-status
mkdir -p tmp/plan2-windows
gh run download <run-id> --name bms-random-table-compositor-windows-amd64 --dir tmp/plan2-windows
ls tmp/plan2-windows/
```

Expected: `bms-random-table-compositor.exe` が tmp 配下に存在。

- [ ] **Step 5: Windows 実機/VM で動作確認**

Step 1 のチェックリストを Windows で再実施。Plan 1 でトレイ動作が確認できているので、Plan 2 の追加観点として「ソース表タブで 4 表登録→更新→削除のサイクル」を中心に確認する。

- [ ] **Step 6: 最終確認とコミット（必要なら）**

Plan 2 完了基準（Plan 冒頭参照）をすべて満たしているか自己診断:

- [ ] 4 表が DB に取り込まれる（GUI で `badge-ok` を確認）
- [ ] 失敗時に `badge-error` + `last_fetch_error` が見える
- [ ] 失敗時もキャッシュは保持される
- [ ] 再起動で ETag 304 経路が動く（log に `notModified=true` が出る）
- [ ] 既存 Plan 1 動作の無回帰
- [ ] `go test ./...` 全 PASS、`go build ./...` 成功
- [ ] Windows exe で同様の操作が動く

すべて OK なら Plan 2 完了。Plan 3（公開表 + ピックエンジン + HTTP サーバ）の `writing-plans` セッションへ。

---

## 最終チェックリスト

Plan 2 完了の自己診断:

- [ ] `make test`（または `go test ./...`）が全 pass
- [ ] `make build` が成功（macOS .app 生成）
- [ ] GitHub Actions の `build-windows.yml` が成功（exe Artifact 生成）
- [ ] Windows 実機/VM で動作確認 OK（Plan 2 観点で 4 表登録 / 更新 / 削除）
- [ ] `compositor.db` の `source_table` / `source_table_chart` に行が入る
- [ ] ETag 304 経路が動作する（再起動後 `notModified=true` のログ）
- [ ] 失敗時のキャッシュ保持を実機で確認
- [ ] 既存 Plan 1 動作の無回帰（ポート保存 / トレイ常駐 / SingleInstanceLock）
- [ ] すべての変更が main ブランチに push 済み

完了後、Plan 3（公開表 + ピックエンジン + HTTP サーバ）の writing-plans セッションへ。Plan 3 では `internal/app/bootstrap.go` に `PublishedTableRepo` / `PickUseCase` / `OwnedChartRepo` / `httpserver` を追加する形で拡張する想定。
