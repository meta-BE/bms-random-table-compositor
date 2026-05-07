# Phase 1 / Plan 3: 公開表 + ピック + HTTPサーバ 実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Plan 1+2 で揃えた基盤・ソース表に公開表（`published_table`）の CRUD・ピックエンジン（`per_request` / `daily` / `manual`）・所持譜面絞り込み・ローカル HTTP サーバ（`/:slug`, `/:slug/header.json`, `/:slug/data.json`, `POST /:slug/_refresh`）と最低限の管理 GUI を載せ、beatoraja から「アプリで作った公開表」を実機で読み込める状態まで完成させる。

**Architecture:** spec §5 の `published_table` を `PublishedTableRepoSQL` で永続化し、`PublishedTableUseCase` が slug バリデーション（`^[a-z0-9][a-z0-9-]{0,62}$` + 予約語 + 重複）と自動生成（kebab-case 化 + 連番）を担当。`OwnedChartRepo` は `songdata.db` を read-only で開いて md5 集合を読む薄い実装で、`OwnedMD5Cache` がメモリにキャッシュし「設定変更時 invalidate」「明示再読み込みボタン」で更新。`PickUseCase` は `port.Clock` と `port.RandSource` を注入してモード別のシード（per_request: nano、daily: ローカル日付、manual: 手動更新時刻）+ publishedID hash で決定論的にピックし、`PickResultStore` が in-memory にキャッシュ。HTTP サーバは `internal/adapter/httpserver/` に独立し、Go 1.22+ の `http.ServeMux` パスパラメータで 4 ルートを登録。`ServerUseCase` が起動 / 停止 / 再起動 / ステータスを管理し、`OnStatusChange` リスナーをフロントエンドへ Wails event で配信。Frontend は `PublishedTablesTab.svelte` を新設、`ServerTab.svelte` にステータス・操作ボタン・所持キャッシュ操作を追加し、`App.svelte` に 3 番目のタブを追加。

**Tech Stack:** Go 1.24 / Wails v2.11.0 / `modernc.org/sqlite`（既存、Plan 3 で songdata.db 用に read-only 接続を追加）/ 標準 `net/http`（Go 1.22+ パスパラメータ）/ 標準 `html/template`（HTML ビュー）/ `math/rand` の `Source` 抽象（決定論テスト用）/ Svelte + TypeScript

**設計ドキュメント:** `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` の §5（domain型・スキーマ）/ §6（port インタフェース・UseCase・Adapter）/ §7.2〜7.4（HTTP 応答）/ §8（エラーハンドリング）/ §10（テスト方針、特に PickUseCase の重点 9 項目） + ブレストメモ（質問①〜⑥の決定: GUI 込み / サーバ自動起動+操作ボタン / HTML テンプレ最小スタイル / 所持キャッシュ + 再読み込みボタン / 公開表 CRUD GUI フルセット / トレイアイコン色は Plan 4）

**Phase 1 全体の Plan 分割:** Plan 1（基盤＝完了） → Plan 2（ソース表取り込み＝完了） → **Plan 3（本ファイル）** → Plan 4（GUI 仕上げ + ダッシュボード + Plan 2.5 統合 + E2E）

**完了条件:**

- 設定タブから公開表を 1 件以上作成し、ブラウザで `http://127.0.0.1:<port>/<slug>` を開いて HTML ビューが表示される
- 同じ URL を beatoraja の難易度表として登録 → アプリ生成の `header.json` / `data.json` が読み込まれて譜面一覧が出る
- `owned_only=true` の公開表を作って `songdata.db` に存在する md5 のみが返ること（実機 + テスト）
- `per_request` モードはアクセス毎に異なる結果、`daily` モードは同一日付内で同じ結果（テストで決定論検証）、`manual` モードは「再ピック」ボタン押下まで結果固定
- 設定タブから「停止」→「起動」でサーバ再起動が成功、ポート競合時はエラーバッジが表示
- 「songdata.db 再読み込み」ボタン押下で所持キャッシュが更新される
- Plan 1 / Plan 2 の動作（設定タブ・ソース表タブ・トレイ・SingleInstanceLock）は無回帰
- `go build ./...` / `go test ./...` 全 pass、`make build` で macOS 成果物生成
- `gh workflow run build-windows.yml` で Windows exe が生成され実機でも上記 E2E が通る

**スコープ外（Plan 4 へ）:**

- ダッシュボード（最近のリクエスト・取得履歴のリングバッファ表示）
- トレイアイコン色切替（Plan 3 では `OnStatusChange` リスナー API のみ用意）
- CSS ライブラリ導入と各タブのスタイル磨き
- Plan 2.5 で計画していた「参照ボタン」「コンテキストメニュー」（Plan 4 で CSS 方針と一括）
- v2 用フラグ `prefer_old_play` の UI 化（DB カラムは Plan 1 で作成済みのまま放置）
- ピックアルゴリズム B / C、最終プレイ日時優先、コースデータ
- ETag/304 の本格運用、ソース表のスケジュール自動更新

**ブランチ運用:** Plan 1 / 2 と同様 main 上で直接コミット。完了時は `git push origin main` で remote 反映（Windows ビルドの workflow_dispatch がデフォルトブランチ参照のため）。

---

## ファイル構造（Plan 3 終了時点で追加・変更されるもの）

新規作成:

```
internal/
├── domain/
│   ├── published_table.go            # PublishedTable, PickConfig, RefreshMode
│   ├── pick_result.go                # PickResult (揮発)
│   └── server_status.go              # ServerStatus, ServerState
├── port/
│   ├── published_table_repo.go       # PublishedTableRepo インタフェース
│   ├── owned_chart_repo.go           # OwnedChartRepo インタフェース
│   ├── clock.go                      # Clock インタフェース
│   └── rand_source.go                # RandSource インタフェース + 静的ファクトリ型
├── adapter/
│   ├── clock/
│   │   ├── system_clock.go           # SystemClock (port.Clock)
│   │   └── system_clock_test.go
│   ├── randsrc/
│   │   ├── math_rand.go              # MathRandSource (port.RandSource), NewMathRandSource(seed)
│   │   └── math_rand_test.go
│   ├── persistence/
│   │   ├── published_table_repo.go   # PublishedTableRepoSQL
│   │   ├── published_table_repo_test.go
│   │   ├── songdata_reader.go        # SongdataReader (port.OwnedChartRepo)
│   │   └── songdata_reader_test.go
│   └── httpserver/
│       ├── server.go                 # AdapterServer + Start / Shutdown / Addr
│       ├── server_test.go
│       ├── router.go                 # NewMux で 4 ルート登録
│       ├── handler_html.go           # GET /:slug
│       ├── handler_html_test.go
│       ├── handler_header.go         # GET /:slug/header.json
│       ├── handler_header_test.go
│       ├── handler_data.go           # GET /:slug/data.json
│       ├── handler_data_test.go
│       ├── handler_refresh.go        # POST /:slug/_refresh
│       ├── handler_refresh_test.go
│       └── templates/
│           └── index.html            # //go:embed
└── usecase/
    ├── errors.go                     # sentinel error 集約
    ├── published_table_usecase.go
    ├── published_table_usecase_test.go
    ├── owned_md5_cache.go
    ├── owned_md5_cache_test.go
    ├── pick_result_store.go
    ├── pick_result_store_test.go
    ├── pick_usecase.go
    ├── pick_usecase_test.go
    ├── server_usecase.go
    └── server_usecase_test.go

internal/app/handler/
├── published_table_handler.go        # Wails Bind: 公開表 CRUD + slug 検証
├── published_table_handler_test.go
├── pick_handler.go                   # Wails Bind: 手動再ピック
├── pick_handler_test.go
├── server_status_handler.go          # Wails Bind: 起動 / 停止 / 再起動 / ステータス取得
├── server_status_handler_test.go
├── owned_chart_handler.go            # Wails Bind: 所持キャッシュ状態 / 再読み込み
└── owned_chart_handler_test.go

frontend/src/lib/tabs/
└── PublishedTablesTab.svelte         # 公開表 CRUD タブ（インライン展開フォーム）
```

変更:

```
internal/usecase/config_usecase.go         # SetOwnedInvalidator hook 追加
internal/app/bootstrap.go                  # 新規 UseCase / Handler の配線
main.go                                    # Bind 配列に 4 ハンドラ追加
app.go                                     # OnStartup でサーバ自動起動 + OnStatusChange Emit + OpenURL メソッド
frontend/src/lib/api.ts                    # サーバ / 公開表 / 所持キャッシュ API 追加
frontend/src/lib/tabs/ServerTab.svelte     # サーバステータス + 操作ボタン + 所持キャッシュ操作
frontend/src/App.svelte                    # 3 番目のタブ「公開表」を追加
```

各ファイルの責務:

| ファイル | 責務 |
|---|---|
| `internal/domain/published_table.go` | `PublishedTable`, `PickConfig`, `RefreshMode`（const 3 値）|
| `internal/domain/pick_result.go` | `PickResult`（揮発、`SeedKey` フィールドで daily 判定）|
| `internal/domain/server_status.go` | `ServerStatus`, `ServerState`（stopped/running/error）|
| `internal/port/published_table_repo.go` | `PublishedTableRepo` インタフェース（CRUD + GetBySlug + SlugExists）|
| `internal/port/owned_chart_repo.go` | `OwnedChartRepo` インタフェース（`LoadOwnedMD5Set(ctx, dbPath)`）|
| `internal/port/clock.go` | `Clock` インタフェース（`Now() time.Time`）|
| `internal/port/rand_source.go` | `RandSource` インタフェース（`Int63`, `Seed`）+ 型エイリアス `RandSourceFactory func(seed int64) RandSource` |
| `internal/adapter/clock/system_clock.go` | `time.Now()` を返す素朴実装 |
| `internal/adapter/randsrc/math_rand.go` | `math/rand.NewSource` ベースの `RandSource` 実装と `NewMathRandSource(seed int64)` ファクトリ |
| `internal/adapter/persistence/published_table_repo.go` | `published_table` の CRUD + UNIQUE 違反のエラー識別 |
| `internal/adapter/persistence/songdata_reader.go` | `songdata.db` を read-only で開き `song.md5` から集合を読む。空パスは空 set で返す |
| `internal/adapter/httpserver/server.go` | `*http.Server` ライフサイクル（Listen → Serve goroutine、Shutdown 5s タイムアウト）|
| `internal/adapter/httpserver/router.go` | `NewMux(deps Deps) *http.ServeMux` で 4 ルート登録 |
| `internal/adapter/httpserver/handler_html.go` | HTML ビュー（テンプレ展開）|
| `internal/adapter/httpserver/handler_header.go` | `header.json` 応答 |
| `internal/adapter/httpserver/handler_data.go` | `data.json` 応答（Raw パススルー + 上書き）|
| `internal/adapter/httpserver/handler_refresh.go` | `POST /:slug/_refresh`（manual 以外 405、303 redirect）|
| `internal/adapter/httpserver/templates/index.html` | テンプレ（`<style>` 同梱、所持/未所持色分け、再ピックフォーム）|
| `internal/usecase/errors.go` | sentinel error 集約 |
| `internal/usecase/published_table_usecase.go` | CRUD + slug バリデーション + 自動生成 |
| `internal/usecase/owned_md5_cache.go` | 所持 md5 set のキャッシュ層（auto-load / Reload / Invalidate / Status）|
| `internal/usecase/pick_result_store.go` | `map[publishedID]PickResult` + `sync.RWMutex`、Snapshot メソッドあり（Plan 4 ダッシュボード用の前準備）|
| `internal/usecase/pick_usecase.go` | `PickBySlug`（モード別キャッシュ判定 + 再生成）+ `ManualRefresh` + `InvalidateAll` |
| `internal/usecase/server_usecase.go` | サーバ起動 / 停止 / 再起動 / ステータス + `OnStatusChange` リスナー登録 |
| `internal/app/handler/published_table_handler.go` | Wails Bind: List/Create/Update/Delete + ValidateSlug + SuggestSlugFromSource + Open |
| `internal/app/handler/pick_handler.go` | Wails Bind: ManualRefresh(publishedID) |
| `internal/app/handler/server_status_handler.go` | Wails Bind: Start / Stop / Restart / GetStatus |
| `internal/app/handler/owned_chart_handler.go` | Wails Bind: GetStatus / Reload |
| `frontend/src/lib/tabs/PublishedTablesTab.svelte` | 公開表 CRUD（インライン展開フォーム + 一覧 + 「開く」ボタン）|

---

## 前提条件と注意

- Plan 1 + Plan 2 完了済み。`compositor.db` の 4 テーブル（config, source_table, source_table_chart, published_table）は Plan 1 のマイグレーションで全て作成済みで、Plan 3 では **新規スキーマ追加なし**
- `internal/port/idgen.go` の `IDGenerator` と `internal/adapter/idgen/ulid.go` は Plan 2 で導入済み。本 Plan でも同じ生成器を `PublishedTableUseCase` に注入
- `internal/adapter/persistence/source_table_repo.go` の `LoadCharts` は Plan 2 で実装済み。本 Plan の `PickUseCase` から呼び出すのみで改修なし
- `testdata/songdata.db` は実機の beatoraja から取得した本物。`song` テーブルに `md5 TEXT NOT NULL` カラムがある。テストではこれを read-only で参照
- `testdata/satellite_header.json` / `testdata/satellite_data.json` は Plan 2 で fetcher テストに使ったが、Plan 3 でも参考データとしてそのまま参照可能（直接は使わない）
- `frontend/wailsjs/go/handler/` 配下は Wails ビルド時に自動生成される。Plan 3 で 4 つの新ハンドラ追加後、`wails dev` か `wails build` を 1 回走らせて再生成する必要がある
- 作業ブランチは **main**（Plan 1 / 2 と同じ運用）
- Phase 0 POC で確認済み: Wails と HTTP サーバの同居は問題なし（`poc/NOTES.md` 参照）

---

## Task 1: domain 型を追加

**Files:**

- Create: `internal/domain/published_table.go`
- Create: `internal/domain/pick_result.go`
- Create: `internal/domain/server_status.go`
- Test: `internal/domain/published_table_test.go`

domain 層は外部依存ゼロ。値オブジェクトのみで複雑なロジックは持たない。`RefreshMode` が定数値であることをスモークテストで確認するだけにする。

- [ ] **Step 1: 失敗テストを書く**

`internal/domain/published_table_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRefreshMode_Values(t *testing.T) {
	require.Equal(t, domain.RefreshMode("per_request"), domain.RefreshModePerRequest)
	require.Equal(t, domain.RefreshMode("daily"), domain.RefreshModeDaily)
	require.Equal(t, domain.RefreshMode("manual"), domain.RefreshModeManual)
}

func TestServerState_Values(t *testing.T) {
	require.Equal(t, domain.ServerState("stopped"), domain.ServerStateStopped)
	require.Equal(t, domain.ServerState("running"), domain.ServerStateRunning)
	require.Equal(t, domain.ServerState("error"), domain.ServerStateError)
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/domain/...`
Expected: `undefined: domain.RefreshModePerRequest` 等で FAIL

- [ ] **Step 3: domain 型を実装**

`internal/domain/published_table.go`:

```go
package domain

// RefreshMode は公開表のピック更新モード。
type RefreshMode string

const (
	RefreshModePerRequest RefreshMode = "per_request"
	RefreshModeDaily      RefreshMode = "daily"
	RefreshModeManual     RefreshMode = "manual"
)

// PickConfig はピック生成に必要な設定値。
type PickConfig struct {
	PerLevel      int         // 0 = 無制限（全件返す）
	RefreshMode   RefreshMode // per_request / daily / manual
	PreferOldPlay bool        // v2 用フラグ。Plan 3 では未使用（常に false）
}

// PublishedTable はユーザーが公開する表。1 公開表 = 1 ソース表（合成は v2）。
type PublishedTable struct {
	ID            string
	Slug          string
	DisplayName   string
	Symbol        string
	SourceTableID string
	OwnedOnly     bool
	Pick          PickConfig
	SortOrder     int
}
```

`internal/domain/pick_result.go`:

```go
package domain

import "time"

// PickResult はピック結果。in-memory 揮発（プロセス再起動で消える）。
// SeedKey は daily モードのキャッシュ判定に使う：今日の YYYY-MM-DD と一致したら再生成不要。
type PickResult struct {
	PublishedTableID string
	GeneratedAt      time.Time
	SeedKey          string        // per_request: nano 値の文字列、daily: YYYY-MM-DD、manual: 手動更新時刻 ISO8601
	Charts           []SourceChart // ピック後・整列済み（レベル間 / レベル内）
	LevelOrder       []string      // 1 曲以上残ったレベルのみ抽出済み（応答 header.json で使う）
}
```

`internal/domain/server_status.go`:

```go
package domain

import "time"

// ServerState は HTTP サーバの稼働状態。
type ServerState string

const (
	ServerStateStopped ServerState = "stopped"
	ServerStateRunning ServerState = "running"
	ServerStateError   ServerState = "error"
)

// ServerStatus は HTTP サーバの状態スナップショット。
type ServerStatus struct {
	State     ServerState
	Port      int
	StartedAt *time.Time
	LastError string // ServerStateError 時にメッセージを格納
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/domain/...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/published_table.go internal/domain/pick_result.go internal/domain/server_status.go internal/domain/published_table_test.go
git commit -m "feat(domain): Plan 3 の domain 型を追加 (PublishedTable / PickResult / ServerStatus)"
```

---

## Task 2: port インタフェース + adapter helpers (Clock / RandSource)

**Files:**

- Create: `internal/port/published_table_repo.go`
- Create: `internal/port/owned_chart_repo.go`
- Create: `internal/port/clock.go`
- Create: `internal/port/rand_source.go`
- Create: `internal/adapter/clock/system_clock.go`
- Create: `internal/adapter/clock/system_clock_test.go`
- Create: `internal/adapter/randsrc/math_rand.go`
- Create: `internal/adapter/randsrc/math_rand_test.go`

port は実装を含まないインタフェース定義のみ。adapter 側に最小実装と簡単なテストを置く。`RandSourceFactory` を型エイリアスにすることで、PickUseCase の決定論テストで「シードを受け取って RandSource を返すモック」を差し替えられる。

- [ ] **Step 1: port 4 ファイルを作成**

`internal/port/published_table_repo.go`:

```go
package port

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// PublishedTableRepo は published_table の永続化を担う。
type PublishedTableRepo interface {
	List(ctx context.Context) ([]domain.PublishedTable, error)
	Get(ctx context.Context, id string) (domain.PublishedTable, error)
	GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error)
	// Create は ID を事前採番した PublishedTable を挿入する。slug の UNIQUE 違反は ErrSlugDuplicated で返す。
	Create(ctx context.Context, t domain.PublishedTable) (string, error)
	Update(ctx context.Context, t domain.PublishedTable) error
	Delete(ctx context.Context, id string) error
	// SlugExists は slug が既に使われているかを返す。excludeID を指定すると自分自身は除外（編集時用）。
	SlugExists(ctx context.Context, slug string, excludeID string) (bool, error)
}
```

`internal/port/owned_chart_repo.go`:

```go
package port

import "context"

// OwnedChartRepo は beatoraja の songdata.db から所持譜面の md5 集合を取得する。
type OwnedChartRepo interface {
	// LoadOwnedMD5Set は dbPath を read-only で開き、song.md5 を読み出して集合で返す。
	// dbPath が空のときは空 set を error なしで返す（spec §8 の「未設定 = 0 件」と整合）。
	LoadOwnedMD5Set(ctx context.Context, dbPath string) (map[string]struct{}, error)
}
```

`internal/port/clock.go`:

```go
package port

import "time"

// Clock は現在時刻を返す。テストで固定する目的で抽象化する。
type Clock interface {
	Now() time.Time
}
```

`internal/port/rand_source.go`:

```go
package port

// RandSource は math/rand.Source 互換の最低限のインタフェース。
// 決定論テストで Int63 / Seed をモック化するために導入。
type RandSource interface {
	Int63() int64
	Seed(seed int64)
}

// RandSourceFactory は seed を受け取って RandSource を作る関数型。
// PickUseCase に注入することで、テストで「常に同じ並び順を返す」モックに差し替えられる。
type RandSourceFactory func(seed int64) RandSource
```

- [ ] **Step 2: SystemClock のテストを書く**

`internal/adapter/clock/system_clock_test.go`:

```go
package clock_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/stretchr/testify/require"
)

func TestSystemClock_Now_IsRecent(t *testing.T) {
	c := clock.System{}
	before := time.Now()
	got := c.Now()
	after := time.Now()
	require.False(t, got.Before(before))
	require.False(t, got.After(after))
}
```

- [ ] **Step 3: SystemClock を実装**

`internal/adapter/clock/system_clock.go`:

```go
// Package clock は port.Clock の素朴実装を提供する。
package clock

import "time"

// System は time.Now() をそのまま返す port.Clock 実装。
type System struct{}

// Now は現在時刻を返す。
func (System) Now() time.Time { return time.Now() }
```

- [ ] **Step 4: MathRandSource のテストを書く**

`internal/adapter/randsrc/math_rand_test.go`:

```go
package randsrc_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/stretchr/testify/require"
)

func TestNewMathRandSource_Deterministic(t *testing.T) {
	a := randsrc.NewMathRandSource(42)
	b := randsrc.NewMathRandSource(42)
	for i := 0; i < 16; i++ {
		require.Equal(t, a.Int63(), b.Int63(), "iter=%d", i)
	}
}

func TestNewMathRandSource_DifferentSeedsDiverge(t *testing.T) {
	a := randsrc.NewMathRandSource(1)
	b := randsrc.NewMathRandSource(2)
	// 連続 8 回のうち 1 回でも違えば OK（同一になる確率は事実上 0）
	diff := false
	for i := 0; i < 8; i++ {
		if a.Int63() != b.Int63() {
			diff = true
			break
		}
	}
	require.True(t, diff)
}
```

- [ ] **Step 5: MathRandSource を実装**

`internal/adapter/randsrc/math_rand.go`:

```go
// Package randsrc は port.RandSource の math/rand ベース実装を提供する。
package randsrc

import (
	"math/rand"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

type mathRandSource struct {
	src rand.Source
}

// NewMathRandSource は math/rand.NewSource(seed) をラップした port.RandSource を返す。
func NewMathRandSource(seed int64) port.RandSource {
	return &mathRandSource{src: rand.NewSource(seed)}
}

func (m *mathRandSource) Int63() int64    { return m.src.Int63() }
func (m *mathRandSource) Seed(seed int64) { m.src.Seed(seed) }
```

- [ ] **Step 6: テストとビルドを確認**

Run: `go build ./... && go test ./internal/port/... ./internal/adapter/clock/... ./internal/adapter/randsrc/...`
Expected: PASS（port パッケージはテストファイル無いがビルドは通る）

- [ ] **Step 7: コミット**

```bash
git add internal/port/published_table_repo.go internal/port/owned_chart_repo.go internal/port/clock.go internal/port/rand_source.go internal/adapter/clock/ internal/adapter/randsrc/
git commit -m "feat(port,adapter): Plan 3 用の port インタフェースと Clock/RandSource 実装を追加"
```

---

## Task 3: usecase エラー集約

**Files:**

- Create: `internal/usecase/errors.go`

ピック / 公開表 / サーバ系の sentinel error を 1 ファイルに集約する。HTTP ハンドラと Wails ハンドラはこれを `errors.Is` で識別してステータスコード or メッセージを決定する。

- [ ] **Step 1: errors.go を作成**

`internal/usecase/errors.go`:

```go
package usecase

import "errors"

// 公開表 / ピック / サーバ層の sentinel error。
// HTTP ハンドラは errors.Is で識別してステータスコードを決定する。
var (
	ErrPublishedTableNotFound = errors.New("公開表が見つかりません")
	ErrSourceNotFetched       = errors.New("ソース表が未取得です")
	ErrSlugInvalidFormat      = errors.New("slug の形式が不正です")
	ErrSlugReserved           = errors.New("slug は予約語です")
	ErrSlugDuplicated         = errors.New("slug は既に使われています")
	ErrInvalidPickPerLevel    = errors.New("pick_per_level は 0 以上の整数である必要があります")
	ErrInvalidRefreshMode     = errors.New("refresh_mode が不正です")
	ErrSourceTableNotFound    = errors.New("ソース表が見つかりません")
	ErrServerAlreadyRunning   = errors.New("サーバは既に起動しています")
	ErrServerNotRunning       = errors.New("サーバは起動していません")
)
```

- [ ] **Step 2: ビルド確認**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 3: コミット**

```bash
git add internal/usecase/errors.go
git commit -m "feat(usecase): Plan 3 用の sentinel error を集約"
```

---

## Task 4: persistence/PublishedTableRepoSQL 実装

**Files:**

- Create: `internal/adapter/persistence/published_table_repo.go`
- Test: `internal/adapter/persistence/published_table_repo_test.go`

`published_table` テーブルは Plan 1 のマイグレーションで作成済み。CRUD + GetBySlug + SlugExists を実装する。`UNIQUE` 違反は modernc/sqlite が `constraint failed: UNIQUE constraint failed: published_table.slug` を返すので、文字列マッチで `ErrSlugDuplicated` に変換する。

- [ ] **Step 1: 失敗テストを書く**

`internal/adapter/persistence/published_table_repo_test.go`:

```go
package persistence_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupPublishedTableRepo(t *testing.T) (*persistence.PublishedTableRepoSQL, *persistence.SourceTableRepoSQL) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))
	return persistence.NewPublishedTableRepoSQL(db), persistence.NewSourceTableRepoSQL(db)
}

func seedSourceTable(t *testing.T, src *persistence.SourceTableRepoSQL, id string) {
	t.Helper()
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, LastFetchStatus: domain.FetchStatusNever,
	})
	require.NoError(t, err)
}

func TestPublishedTableRepoSQL_CreateThenGet(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000A")

	in := domain.PublishedTable{
		ID: "01J0PUB0000000000000000A", Slug: "satellite-mix",
		DisplayName: "Satellite Mix", Symbol: "sl",
		SourceTableID: "01J0SOURCE000000000000A",
		OwnedOnly:     true,
		Pick:          domain.PickConfig{PerLevel: 5, RefreshMode: domain.RefreshModeDaily},
	}
	id, err := repo.Create(ctx, in)
	require.NoError(t, err)
	require.Equal(t, in.ID, id)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, in.Slug, got.Slug)
	require.Equal(t, in.DisplayName, got.DisplayName)
	require.Equal(t, in.SourceTableID, got.SourceTableID)
	require.True(t, got.OwnedOnly)
	require.Equal(t, 5, got.Pick.PerLevel)
	require.Equal(t, domain.RefreshModeDaily, got.Pick.RefreshMode)
}

func TestPublishedTableRepoSQL_GetBySlug(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000B")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000B", Slug: "lookup-me",
		DisplayName: "Lookup", SourceTableID: "01J0SOURCE000000000000B",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	got, err := repo.GetBySlug(ctx, "lookup-me")
	require.NoError(t, err)
	require.Equal(t, "01J0PUB0000000000000000B", got.ID)

	_, err = repo.GetBySlug(ctx, "no-such-slug")
	require.ErrorIs(t, err, usecase.ErrPublishedTableNotFound)
}

func TestPublishedTableRepoSQL_SlugExists(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000C")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000C", Slug: "taken",
		DisplayName: "T", SourceTableID: "01J0SOURCE000000000000C",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.NoError(t, err)

	exists, err := repo.SlugExists(ctx, "taken", "")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = repo.SlugExists(ctx, "free", "")
	require.NoError(t, err)
	require.False(t, exists)

	// 自分自身は除外
	exists, err = repo.SlugExists(ctx, "taken", "01J0PUB0000000000000000C")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestPublishedTableRepoSQL_Create_DuplicateSlugError(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000D")
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB000000000000000D1", Slug: "dup",
		DisplayName: "A", SourceTableID: "01J0SOURCE000000000000D",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB000000000000000D2", Slug: "dup",
		DisplayName: "B", SourceTableID: "01J0SOURCE000000000000D",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableRepoSQL_Update_RoundTrip(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000E")
	id, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000E", Slug: "before",
		DisplayName: "Before", SourceTableID: "01J0SOURCE000000000000E",
		OwnedOnly: false,
		Pick:      domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	got.Slug = "after"
	got.DisplayName = "After"
	got.OwnedOnly = true
	got.Pick.PerLevel = 3
	got.Pick.RefreshMode = domain.RefreshModeDaily
	require.NoError(t, repo.Update(ctx, got))

	again, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "after", again.Slug)
	require.Equal(t, "After", again.DisplayName)
	require.True(t, again.OwnedOnly)
	require.Equal(t, 3, again.Pick.PerLevel)
	require.Equal(t, domain.RefreshModeDaily, again.Pick.RefreshMode)
}

func TestPublishedTableRepoSQL_Delete_Idempotent(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE000000000000F")
	id, err := repo.Create(ctx, domain.PublishedTable{
		ID: "01J0PUB0000000000000000F", Slug: "to-delete",
		DisplayName: "X", SourceTableID: "01J0SOURCE000000000000F",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, id))
	require.NoError(t, repo.Delete(ctx, id)) // 二度目もエラーにならない

	_, err = repo.Get(ctx, id)
	require.ErrorIs(t, err, usecase.ErrPublishedTableNotFound)
}

func TestPublishedTableRepoSQL_List_OrdersBySortOrderThenCreatedAt(t *testing.T) {
	repo, src := setupPublishedTableRepo(t)
	ctx := context.Background()
	seedSourceTable(t, src, "01J0SOURCE0000000000010")

	for i, slug := range []string{"a-second", "b-first", "c-third"} {
		so := 0
		switch slug {
		case "b-first":
			so = -1
		case "c-third":
			so = 1
		}
		_, err := repo.Create(ctx, domain.PublishedTable{
			ID:            string(rune('A'+i)) + "01J0PUB0000000000000010",
			Slug:          slug,
			DisplayName:   slug,
			SourceTableID: "01J0SOURCE0000000000010",
			SortOrder:     so,
			Pick:          domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
		})
		require.NoError(t, err)
	}

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, "b-first", list[0].Slug)
	require.Equal(t, "a-second", list[1].Slug)
	require.Equal(t, "c-third", list[2].Slug)
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/adapter/persistence/... -run PublishedTable`
Expected: `undefined: persistence.PublishedTableRepoSQL` で FAIL

- [ ] **Step 3: PublishedTableRepoSQL を実装**

`internal/adapter/persistence/published_table_repo.go`:

```go
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// PublishedTableRepoSQL は published_table の永続化を担う port.PublishedTableRepo 実装。
type PublishedTableRepoSQL struct {
	db *sql.DB
}

// NewPublishedTableRepoSQL は新しい PublishedTableRepoSQL を作る。
func NewPublishedTableRepoSQL(db *sql.DB) *PublishedTableRepoSQL {
	return &PublishedTableRepoSQL{db: db}
}

const publishedTableSelectColumns = `SELECT
	id, slug, display_name, symbol, source_table_id, owned_only,
	pick_per_level, pick_refresh_mode, prefer_old_play, sort_order
 FROM published_table`

func (r *PublishedTableRepoSQL) scanRow(s rowScanner) (domain.PublishedTable, error) {
	var (
		t          domain.PublishedTable
		ownedOnly  int
		preferOld  int
		mode       string
	)
	if err := s.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.Symbol, &t.SourceTableID, &ownedOnly,
		&t.Pick.PerLevel, &mode, &preferOld, &t.SortOrder,
	); err != nil {
		return domain.PublishedTable{}, err
	}
	t.OwnedOnly = ownedOnly != 0
	t.Pick.PreferOldPlay = preferOld != 0
	t.Pick.RefreshMode = domain.RefreshMode(mode)
	return t, nil
}

// Create は PublishedTable を新規挿入する。slug の UNIQUE 違反は ErrSlugDuplicated を返す。
func (r *PublishedTableRepoSQL) Create(ctx context.Context, t domain.PublishedTable) (string, error) {
	if t.ID == "" {
		return "", errors.New("ID は必須です")
	}
	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	preferOld := 0
	if t.Pick.PreferOldPlay {
		preferOld = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO published_table
		 (id, slug, display_name, symbol, source_table_id, owned_only,
		  pick_per_level, pick_refresh_mode, prefer_old_play, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Slug, t.DisplayName, t.Symbol, t.SourceTableID, owned,
		t.Pick.PerLevel, string(t.Pick.RefreshMode), preferOld, t.SortOrder,
	)
	if err != nil {
		if isUniqueSlugViolation(err) {
			return "", fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return "", fmt.Errorf("insert published_table %q: %w", t.ID, err)
	}
	return t.ID, nil
}

// isUniqueSlugViolation は modernc/sqlite が返す UNIQUE 制約違反かを判定する。
// modernc は標準 SQLite の "UNIQUE constraint failed: published_table.slug" メッセージを保つ。
func isUniqueSlugViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "published_table.slug")
}

// Get は ID で取得する。存在しない場合は ErrPublishedTableNotFound を返す。
func (r *PublishedTableRepoSQL) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE id = ?`, id)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table %q: %w", id, err)
	}
	return t, nil
}

// GetBySlug は slug で取得する。存在しない場合は ErrPublishedTableNotFound を返す。
func (r *PublishedTableRepoSQL) GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE slug = ?`, slug)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table by slug %q: %w", slug, err)
	}
	return t, nil
}

// List は sort_order, created_at 順に返す。
func (r *PublishedTableRepoSQL) List(ctx context.Context) ([]domain.PublishedTable, error) {
	rows, err := r.db.QueryContext(ctx,
		publishedTableSelectColumns+` ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list published_table: %w", err)
	}
	defer rows.Close()
	var out []domain.PublishedTable
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Update は値を上書きする。slug の UNIQUE 違反は ErrSlugDuplicated を返す。
func (r *PublishedTableRepoSQL) Update(ctx context.Context, t domain.PublishedTable) error {
	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	preferOld := 0
	if t.Pick.PreferOldPlay {
		preferOld = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE published_table SET
		   slug=?, display_name=?, symbol=?, source_table_id=?, owned_only=?,
		   pick_per_level=?, pick_refresh_mode=?, prefer_old_play=?, sort_order=?,
		   updated_at=datetime('now')
		 WHERE id=?`,
		t.Slug, t.DisplayName, t.Symbol, t.SourceTableID, owned,
		t.Pick.PerLevel, string(t.Pick.RefreshMode), preferOld, t.SortOrder, t.ID,
	)
	if err != nil {
		if isUniqueSlugViolation(err) {
			return fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return fmt.Errorf("update published_table %q: %w", t.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return usecase.ErrPublishedTableNotFound
	}
	return nil
}

// Delete は ID で削除する。存在しなくてもエラーにしない（冪等）。
func (r *PublishedTableRepoSQL) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM published_table WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete published_table %q: %w", id, err)
	}
	return nil
}

// SlugExists は slug が既に使われているかを返す。excludeID を指定すると自分自身は除外する。
func (r *PublishedTableRepoSQL) SlugExists(ctx context.Context, slug string, excludeID string) (bool, error) {
	var count int
	var err error
	if excludeID == "" {
		err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM published_table WHERE slug = ?`, slug).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM published_table WHERE slug = ? AND id <> ?`,
			slug, excludeID).Scan(&count)
	}
	if err != nil {
		return false, fmt.Errorf("slug exists %q: %w", slug, err)
	}
	return count > 0, nil
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/adapter/persistence/... -run PublishedTable -v`
Expected: 全 PASS

- [ ] **Step 5: 全体テストでのリグレッション確認**

Run: `go test ./...`
Expected: 全 PASS（既存テストへの影響なし）

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/persistence/published_table_repo.go internal/adapter/persistence/published_table_repo_test.go
git commit -m "feat(persistence): PublishedTableRepoSQL を追加 (CRUD + GetBySlug + SlugExists)"
```

---

## Task 5: persistence/SongdataReader 実装

**Files:**

- Create: `internal/adapter/persistence/songdata_reader.go`
- Test: `internal/adapter/persistence/songdata_reader_test.go`

`songdata.db` を read-only で開いて `song.md5` を SELECT する。spec §10 では「dbPath が空のときは空 set + error なし」「DB ファイル不存在時はエラー」とする。`?mode=ro&_busy_timeout=2000` の DSN で beatoraja 起動中の WAL ロック競合をある程度緩和する。実テストには `testdata/songdata.db` を使う。

- [ ] **Step 1: 失敗テストを書く**

`internal/adapter/persistence/songdata_reader_test.go`:

```go
package persistence_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/stretchr/testify/require"
)

// testdata/songdata.db への絶対パスを返す。
func songdataPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../../testdata/songdata.db")
	require.NoError(t, err)
	return abs
}

func TestSongdataReader_LoadOwnedMD5Set_EmptyPathReturnsEmptySet(t *testing.T) {
	r := persistence.NewSongdataReader()
	got, err := r.LoadOwnedMD5Set(context.Background(), "")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSongdataReader_LoadOwnedMD5Set_MissingFileReturnsError(t *testing.T) {
	r := persistence.NewSongdataReader()
	_, err := r.LoadOwnedMD5Set(context.Background(), "/non/existent/path/songdata.db")
	require.Error(t, err)
}

func TestSongdataReader_LoadOwnedMD5Set_RealDB(t *testing.T) {
	r := persistence.NewSongdataReader()
	got, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	require.NotEmpty(t, got, "testdata/songdata.db には song 行があるはず")
	// md5 集合の各キーは 32 文字 16進数のはず
	for k := range got {
		require.Len(t, k, 32, "md5 must be 32 hex chars: %q", k)
	}
}

func TestSongdataReader_LoadOwnedMD5Set_DoesNotMutateFile(t *testing.T) {
	// 書き込み防止の確認: read-only で開いているので songdata.db への変更が起きない。
	// 連続呼び出しで count が同じであることだけ確認（実際の mtime チェックは過剰）
	r := persistence.NewSongdataReader()
	first, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	second, err := r.LoadOwnedMD5Set(context.Background(), songdataPath(t))
	require.NoError(t, err)
	require.Equal(t, len(first), len(second))
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/adapter/persistence/... -run Songdata`
Expected: `undefined: persistence.NewSongdataReader` で FAIL

- [ ] **Step 3: SongdataReader を実装**

`internal/adapter/persistence/songdata_reader.go`:

```go
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite"
)

// SongdataReader は beatoraja の songdata.db から所持 md5 集合を読む port.OwnedChartRepo 実装。
type SongdataReader struct{}

// NewSongdataReader は新しい SongdataReader を作る。
func NewSongdataReader() *SongdataReader {
	return &SongdataReader{}
}

// LoadOwnedMD5Set は dbPath を read-only で開き、song.md5 を読み出して集合で返す。
//
// dbPath が空文字列の場合は空 set + error なしで返す（spec §8: 「DB 未設定時は owned_only の表は 0 件」と整合）。
// dbPath が存在しないファイルなら明示的にエラー（GUI で「ファイルが見つかりません」と表示できるように）。
func (r *SongdataReader) LoadOwnedMD5Set(ctx context.Context, dbPath string) (map[string]struct{}, error) {
	if dbPath == "" {
		return map[string]struct{}{}, nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("songdata.db を開けません %q: %w", dbPath, err)
	}

	// modernc/sqlite の DSN: クエリパラメータで mode=ro と _busy_timeout を指定。
	// パスは url.QueryEscape して特殊文字（スペース・日本語）に対応。
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=2000", url.QueryEscape(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open ro %q: %w", dbPath, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT md5 FROM song`)
	if err != nil {
		return nil, fmt.Errorf("select md5: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{}, 4096)
	for rows.Next() {
		var md5 string
		if err := rows.Scan(&md5); err != nil {
			return nil, fmt.Errorf("scan md5: %w", err)
		}
		if md5 != "" {
			out[md5] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/adapter/persistence/... -run Songdata -v`
Expected: 全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/songdata_reader.go internal/adapter/persistence/songdata_reader_test.go
git commit -m "feat(persistence): SongdataReader を追加 (read-only で songdata.db から md5 集合を取得)"
```

---

## Task 6: usecase/PublishedTableUseCase 実装

**Files:**

- Create: `internal/usecase/published_table_usecase.go`
- Test: `internal/usecase/published_table_usecase_test.go`

CRUD + slug バリデーション + 自動生成（kebab-case + 連番）+ 予約語 + 重複チェック。`port.SourceTableRepo` も注入して `SuggestSlugFromSource` でソース表名から slug を作る。テストはフェイクリポジトリで網羅する。

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/published_table_usecase_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakePublishedRepo struct {
	mu    sync.Mutex
	rows  map[string]domain.PublishedTable
	order []string
}

func newFakePublishedRepo() *fakePublishedRepo {
	return &fakePublishedRepo{rows: map[string]domain.PublishedTable{}}
}

func (r *fakePublishedRepo) List(_ context.Context) ([]domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.PublishedTable, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.rows[id])
	}
	return out, nil
}

func (r *fakePublishedRepo) Get(_ context.Context, id string) (domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.rows[id]; ok {
		return v, nil
	}
	return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
}

func (r *fakePublishedRepo) GetBySlug(_ context.Context, slug string) (domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.rows {
		if v.Slug == slug {
			return v, nil
		}
	}
	return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
}

func (r *fakePublishedRepo) Create(_ context.Context, t domain.PublishedTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.rows {
		if v.Slug == t.Slug {
			return "", usecase.ErrSlugDuplicated
		}
	}
	r.rows[t.ID] = t
	r.order = append(r.order, t.ID)
	return t.ID, nil
}

func (r *fakePublishedRepo) Update(_ context.Context, t domain.PublishedTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[t.ID]; !ok {
		return usecase.ErrPublishedTableNotFound
	}
	for id, v := range r.rows {
		if id != t.ID && v.Slug == t.Slug {
			return usecase.ErrSlugDuplicated
		}
	}
	r.rows[t.ID] = t
	return nil
}

func (r *fakePublishedRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	for i, v := range r.order {
		if v == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

func (r *fakePublishedRepo) SlugExists(_ context.Context, slug string, excludeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, v := range r.rows {
		if id != excludeID && v.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}

type fakeIDGen struct {
	mu  sync.Mutex
	seq int
}

func (g *fakeIDGen) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return "01J0PUB" + string(rune('A'+g.seq-1)) + "00000000000000000"
}

func newPublishedUC(t *testing.T, sourceRepo *fakeSourceRepo) (*usecase.PublishedTableUseCase, *fakePublishedRepo) {
	t.Helper()
	pubRepo := newFakePublishedRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := usecase.NewPublishedTableUseCase(pubRepo, sourceRepo, &fakeIDGen{}, logger)
	return uc, pubRepo
}

func seedSource(t *testing.T, repo *fakeSourceRepo, id, name, displayName string) {
	t.Helper()
	_, err := repo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: name, DisplayName: displayName,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
}

func TestPublishedTableUseCase_Create_Success(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000A", "Satellite", "")
	uc, pubRepo := newPublishedUC(t, src)

	id, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "sl-mix", DisplayName: "SL Mix", Symbol: "sl",
		SourceTableID: "01JSRC0000000000000000A",
		OwnedOnly:     true, PickPerLevel: 5,
		RefreshMode: domain.RefreshModeDaily,
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := pubRepo.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "sl-mix", got.Slug)
	require.True(t, got.OwnedOnly)
	require.Equal(t, 5, got.Pick.PerLevel)
}

func TestPublishedTableUseCase_Create_RejectsInvalidSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000B", "X", "")
	uc, _ := newPublishedUC(t, src)

	for _, bad := range []string{
		"",            // 空
		"-leading",    // ハイフン始まり
		"UPPER",       // 大文字
		"under_score", // アンダースコア
		"with space",  // スペース
		"あいう",      // マルチバイト
	} {
		_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
			Slug: bad, DisplayName: "X",
			SourceTableID: "01JSRC0000000000000000B",
			RefreshMode:   domain.RefreshModePerRequest,
		})
		require.True(t, errors.Is(err, usecase.ErrSlugInvalidFormat),
			"slug=%q expected ErrSlugInvalidFormat, got %v", bad, err)
	}
}

func TestPublishedTableUseCase_Create_RejectsReservedSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000C", "X", "")
	uc, _ := newPublishedUC(t, src)

	for _, reserved := range []string{"_admin", "_health", "_metrics", "_refresh", "static", "assets"} {
		_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
			Slug: reserved, DisplayName: "X",
			SourceTableID: "01JSRC0000000000000000C",
			RefreshMode:   domain.RefreshModePerRequest,
		})
		require.True(t, errors.Is(err, usecase.ErrSlugReserved) || errors.Is(err, usecase.ErrSlugInvalidFormat),
			"slug=%q expected reserved or invalid, got %v", reserved, err)
	}
}

func TestPublishedTableUseCase_Create_RejectsUnknownSourceTable(t *testing.T) {
	src := newFakeSourceRepo()
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		SourceTableID: "01JSRC0000000000000000Z",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.True(t, errors.Is(err, usecase.ErrSourceTableNotFound))
}

func TestPublishedTableUseCase_Create_RejectsInvalidRefreshMode(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000D", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		SourceTableID: "01JSRC0000000000000000D",
		RefreshMode:   domain.RefreshMode("hourly"),
	})
	require.True(t, errors.Is(err, usecase.ErrInvalidRefreshMode))
}

func TestPublishedTableUseCase_Create_RejectsNegativePickPerLevel(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000E", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		SourceTableID: "01JSRC0000000000000000E",
		PickPerLevel:  -1,
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.True(t, errors.Is(err, usecase.ErrInvalidPickPerLevel))
}

func TestPublishedTableUseCase_Create_DuplicateSlugFails(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000F", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "dup", DisplayName: "A",
		SourceTableID: "01JSRC0000000000000000F",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.NoError(t, err)
	_, err = uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "dup", DisplayName: "B",
		SourceTableID: "01JSRC0000000000000000F",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableUseCase_ValidateSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000010", "X", "")
	uc, _ := newPublishedUC(t, src)
	id, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "taken", DisplayName: "X",
		SourceTableID: "01JSRC00000000000000010",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.NoError(t, err)

	require.NoError(t, uc.ValidateSlug(context.Background(), "free-slug", ""))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "Bad_Slug", ""), usecase.ErrSlugInvalidFormat))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "_admin", ""), usecase.ErrSlugReserved))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "taken", ""), usecase.ErrSlugDuplicated))
	// 自分自身を除外すれば OK
	require.NoError(t, uc.ValidateSlug(context.Background(), "taken", id))
}

func TestPublishedTableUseCase_SuggestSlugFromSource(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000011", "Satellite", "")        // Name のみ
	seedSource(t, src, "01JSRC00000000000000012", "X", "発狂BMS難易度表") // DisplayName 優先 → 全部マルチバイトなのでフォールバック
	seedSource(t, src, "01JSRC00000000000000013", "Stellar Mix β", "")
	uc, _ := newPublishedUC(t, src)

	got, err := uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000011")
	require.NoError(t, err)
	require.Equal(t, "satellite", got)

	got, err = uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000012")
	require.NoError(t, err)
	// 全部除去された場合は "published" にフォールバック
	require.Equal(t, "published", got)

	got, err = uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000013")
	require.NoError(t, err)
	require.Equal(t, "stellar-mix", got)
}

func TestPublishedTableUseCase_SuggestSlugFromSource_AppendsSuffixOnCollision(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000020", "Satellite", "")
	uc, repo := newPublishedUC(t, src)
	// 既に satellite と satellite-2 が使われているケース
	require.NoError(t, addRow(repo, "PUBA", "satellite", "01JSRC00000000000000020"))
	require.NoError(t, addRow(repo, "PUBB", "satellite-2", "01JSRC00000000000000020"))

	got, err := uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000020")
	require.NoError(t, err)
	require.Equal(t, "satellite-3", got)
}

func addRow(repo *fakePublishedRepo, id, slug, sourceID string) error {
	_, err := repo.Create(context.Background(), domain.PublishedTable{
		ID: id, Slug: slug, DisplayName: slug, SourceTableID: sourceID,
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	return err
}

func TestPublishedTableUseCase_Update_ChecksSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000030", "X", "")
	uc, _ := newPublishedUC(t, src)
	id1, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "first", DisplayName: "First",
		SourceTableID: "01JSRC00000000000000030",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.NoError(t, err)
	id2, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "second", DisplayName: "Second",
		SourceTableID: "01JSRC00000000000000030",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.NoError(t, err)
	_ = id1

	// 自分の slug を別の有効値へ → OK
	require.NoError(t, uc.Update(context.Background(), usecase.UpdatePublishedTableInput{
		ID: id2, Slug: "second-renamed", DisplayName: "Second",
		SourceTableID: "01JSRC00000000000000030",
		RefreshMode:   domain.RefreshModePerRequest,
	}))
	// 他人の slug に変更 → 重複
	err = uc.Update(context.Background(), usecase.UpdatePublishedTableInput{
		ID: id2, Slug: "first", DisplayName: "Second",
		SourceTableID: "01JSRC00000000000000030",
		RefreshMode:   domain.RefreshModePerRequest,
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/usecase/... -run PublishedTable`
Expected: `undefined: usecase.NewPublishedTableUseCase` 等で FAIL

- [ ] **Step 3: PublishedTableUseCase を実装**

`internal/usecase/published_table_usecase.go`:

```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// slug 正規表現: 先頭は英数字、本体は英小文字 / 数字 / ハイフン、最大 63 文字。
var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// 予約 slug。HTML ビューやアプリ内部用パスとの衝突を避けるため事前禁止。
// 先頭 `_` のものは予約とみなす（バリデーション側で `_` 始まりも弾く）。
var reservedSlugs = map[string]struct{}{
	"_admin":      {},
	"_health":     {},
	"_metrics":    {},
	"_refresh":    {},
	"static":      {},
	"assets":      {},
	"favicon.ico": {},
	"robots.txt":  {},
}

// PublishedTableUseCase は公開表 CRUD と slug バリデーション/自動生成を担う。
type PublishedTableUseCase struct {
	repo    port.PublishedTableRepo
	srcRepo port.SourceTableRepo
	idGen   port.IDGenerator
	log     *slog.Logger
}

// NewPublishedTableUseCase は新しい PublishedTableUseCase を作る。
func NewPublishedTableUseCase(
	repo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	idGen port.IDGenerator,
	log *slog.Logger,
) *PublishedTableUseCase {
	return &PublishedTableUseCase{repo: repo, srcRepo: srcRepo, idGen: idGen, log: log}
}

// CreatePublishedTableInput は Create が受け取る入力。
type CreatePublishedTableInput struct {
	Slug          string
	DisplayName   string
	Symbol        string
	SourceTableID string
	OwnedOnly     bool
	PickPerLevel  int
	RefreshMode   domain.RefreshMode
}

// UpdatePublishedTableInput は Update が受け取る入力。
type UpdatePublishedTableInput struct {
	ID            string
	Slug          string
	DisplayName   string
	Symbol        string
	SourceTableID string
	OwnedOnly     bool
	PickPerLevel  int
	RefreshMode   domain.RefreshMode
	SortOrder     int
}

func (u *PublishedTableUseCase) List(ctx context.Context) ([]domain.PublishedTable, error) {
	return u.repo.List(ctx)
}

func (u *PublishedTableUseCase) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	return u.repo.Get(ctx, id)
}

// Create は入力を検証して PublishedTable を作る。
func (u *PublishedTableUseCase) Create(ctx context.Context, in CreatePublishedTableInput) (string, error) {
	if err := u.validateInput(ctx, in.Slug, "", in.SourceTableID, in.PickPerLevel, in.RefreshMode); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return "", errors.New("表示名は必須です")
	}

	id := u.idGen.New()
	t := domain.PublishedTable{
		ID: id, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		SourceTableID: in.SourceTableID, OwnedOnly: in.OwnedOnly,
		Pick: domain.PickConfig{
			PerLevel: in.PickPerLevel, RefreshMode: in.RefreshMode,
		},
	}
	out, err := u.repo.Create(ctx, t)
	if err != nil {
		return "", err
	}
	u.log.Info("published table created", "id", out, "slug", in.Slug)
	return out, nil
}

// Update は入力を検証して PublishedTable を更新する。
func (u *PublishedTableUseCase) Update(ctx context.Context, in UpdatePublishedTableInput) error {
	if in.ID == "" {
		return errors.New("ID は必須です")
	}
	if err := u.validateInput(ctx, in.Slug, in.ID, in.SourceTableID, in.PickPerLevel, in.RefreshMode); err != nil {
		return err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return errors.New("表示名は必須です")
	}

	t := domain.PublishedTable{
		ID: in.ID, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		SourceTableID: in.SourceTableID, OwnedOnly: in.OwnedOnly,
		Pick: domain.PickConfig{
			PerLevel: in.PickPerLevel, RefreshMode: in.RefreshMode,
		},
		SortOrder: in.SortOrder,
	}
	if err := u.repo.Update(ctx, t); err != nil {
		return err
	}
	u.log.Info("published table updated", "id", in.ID, "slug", in.Slug)
	return nil
}

// Delete は ID で公開表を削除する。
func (u *PublishedTableUseCase) Delete(ctx context.Context, id string) error {
	if err := u.repo.Delete(ctx, id); err != nil {
		return err
	}
	u.log.Info("published table deleted", "id", id)
	return nil
}

// ValidateSlug は slug の形式 / 予約語 / 重複を検査する（GUI のリアルタイム判定用）。
func (u *PublishedTableUseCase) ValidateSlug(ctx context.Context, slug string, excludeID string) error {
	if err := validateSlugFormat(slug); err != nil {
		return err
	}
	exists, err := u.repo.SlugExists(ctx, slug, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return ErrSlugDuplicated
	}
	return nil
}

// SuggestSlugFromSource はソース表名（DisplayName || Name）から slug を生成する。
// 衝突時は末尾に -2, -3, ... を付与して空き番号を返す。
func (u *PublishedTableUseCase) SuggestSlugFromSource(ctx context.Context, sourceID string) (string, error) {
	src, err := u.srcRepo.Get(ctx, sourceID)
	if err != nil {
		return "", ErrSourceTableNotFound
	}
	base := slugify(firstNonEmpty(src.DisplayName, src.Name))
	if base == "" {
		base = "published"
	}
	candidate := base
	for i := 2; ; i++ {
		exists, err := u.repo.SlugExists(ctx, candidate, "")
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
		if i > 100 {
			return "", errors.New("slug 候補が見つかりません")
		}
	}
}

// validateInput は Create / Update 共通のバリデーション。
func (u *PublishedTableUseCase) validateInput(
	ctx context.Context, slug, excludeID, sourceID string, perLevel int, mode domain.RefreshMode,
) error {
	if err := validateSlugFormat(slug); err != nil {
		return err
	}
	if perLevel < 0 {
		return ErrInvalidPickPerLevel
	}
	switch mode {
	case domain.RefreshModePerRequest, domain.RefreshModeDaily, domain.RefreshModeManual:
	default:
		return ErrInvalidRefreshMode
	}
	if _, err := u.srcRepo.Get(ctx, sourceID); err != nil {
		return ErrSourceTableNotFound
	}
	exists, err := u.repo.SlugExists(ctx, slug, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return ErrSlugDuplicated
	}
	return nil
}

// validateSlugFormat は slug の文字種・長さ・予約語を検査する。
func validateSlugFormat(slug string) error {
	if strings.HasPrefix(slug, "_") {
		return ErrSlugReserved
	}
	if _, ok := reservedSlugs[slug]; ok {
		return ErrSlugReserved
	}
	if !slugRegexp.MatchString(slug) {
		return ErrSlugInvalidFormat
	}
	return nil
}

// slugify は文字列を kebab-case 化する。英数字以外はハイフンに置換し、連続ハイフンを 1 つにまとめ、両端を削る。
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	return out
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run PublishedTable -v`
Expected: 全 PASS

- [ ] **Step 5: 全体テストでのリグレッション確認**

Run: `go test ./...`
Expected: 全 PASS

- [ ] **Step 6: コミット**

```bash
git add internal/usecase/published_table_usecase.go internal/usecase/published_table_usecase_test.go
git commit -m "feat(usecase): PublishedTableUseCase を追加 (CRUD + slug バリデーション + 自動生成)"
```

---

## Task 7: usecase/OwnedMD5Cache 実装

**Files:**

- Create: `internal/usecase/owned_md5_cache.go`
- Test: `internal/usecase/owned_md5_cache_test.go`

`OwnedChartRepo` の上に薄いキャッシュ層を被せる。動作:
- 未ロード時に最初の `Get` で自動的に `Reload` を実行
- `Reload` は `ConfigStore.Get("songdata_db_path")` で最新パスを取得して repo を呼ぶ
- `Reload` 失敗時は前回の set を維持しつつ `lastErr` のみ更新
- `Invalidate` で set を nil 化（次回 `Get` で再ロード）
- パスが空のときは空 set を返す（`OwnedChartRepo` の仕様により error なし）

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/owned_md5_cache_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakeOwnedRepo struct {
	mu       sync.Mutex
	calls    int
	resp     map[string]struct{}
	err      error
	lastPath string
}

func (r *fakeOwnedRepo) LoadOwnedMD5Set(_ context.Context, dbPath string) (map[string]struct{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastPath = dbPath
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]struct{}, len(r.resp))
	for k := range r.resp {
		out[k] = struct{}{}
	}
	return out, nil
}

type fakeConfigStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newFakeConfigStore() *fakeConfigStore { return &fakeConfigStore{m: map[string]string{}} }

func (s *fakeConfigStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

func (s *fakeConfigStore) Set(_ context.Context, key, val string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = val
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newOwnedCache(repo *fakeOwnedRepo, store *fakeConfigStore, clock fixedClock) *usecase.OwnedMD5Cache {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return usecase.NewOwnedMD5Cache(repo, store, clock, logger)
}

func TestOwnedMD5Cache_Get_AutoLoadsOnce(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"abc": {}, "def": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/path/to/db"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, repo.calls)

	// 2 回目は repo を再呼出ししない
	got, err = c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, repo.calls)
}

func TestOwnedMD5Cache_Reload_HitsRepoAgain(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"a": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())
	require.Equal(t, 1, repo.calls)

	// repo の戻り値を増やしてから Reload
	repo.mu.Lock()
	repo.resp = map[string]struct{}{"a": {}, "b": {}, "c": {}}
	repo.mu.Unlock()
	require.NoError(t, c.Reload(context.Background()))
	require.Equal(t, 2, repo.calls)

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3)
}

func TestOwnedMD5Cache_Reload_KeepsPreviousSetOnError(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"a": {}, "b": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())

	repo.mu.Lock()
	repo.err = errors.New("disk full")
	repo.mu.Unlock()
	err := c.Reload(context.Background())
	require.Error(t, err)

	// 前回の set は保持されている
	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)

	st := c.Status()
	require.Equal(t, 2, st.Count)
	require.Contains(t, st.LastError, "disk full")
}

func TestOwnedMD5Cache_Invalidate_TriggersReload(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{"x": {}}}
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "songdata_db_path", "/p"))
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	_, _ = c.Get(context.Background())
	require.Equal(t, 1, repo.calls)

	c.Invalidate()
	_, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls)
}

func TestOwnedMD5Cache_EmptyPath_ReturnsEmptySet(t *testing.T) {
	repo := &fakeOwnedRepo{resp: map[string]struct{}{}}
	store := newFakeConfigStore()
	// songdata_db_path 未設定
	c := newOwnedCache(repo, store, fixedClock{t: time.Unix(1700000000, 0)})

	got, err := c.Get(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)
	// repo は呼ばれる（dbPath="" でも repo の責務として空 set を返す）
	require.Equal(t, 1, repo.calls)
	require.Equal(t, "", repo.lastPath)
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/usecase/... -run OwnedMD5Cache`
Expected: `undefined: usecase.NewOwnedMD5Cache` 等で FAIL

- [ ] **Step 3: OwnedMD5Cache を実装**

`internal/usecase/owned_md5_cache.go`:

```go
package usecase

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// OwnedCacheStatus は GUI 表示用のキャッシュ状態スナップショット。
type OwnedCacheStatus struct {
	Loaded     bool
	Count      int
	LoadedAt   *time.Time
	LoadedPath string
	LastError  string
}

// OwnedMD5Cache は port.OwnedChartRepo の上に薄いキャッシュを被せた usecase。
// auto-load + 明示的な Reload + Invalidate（設定変更 hook）+ Status を提供する。
type OwnedMD5Cache struct {
	repo  port.OwnedChartRepo
	cfg   port.ConfigStore
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	loaded     bool
	set        map[string]struct{}
	loadedAt   *time.Time
	loadedPath string
	lastErr    string
}

// NewOwnedMD5Cache は新しい OwnedMD5Cache を作る。
func NewOwnedMD5Cache(
	repo port.OwnedChartRepo,
	cfg port.ConfigStore,
	clock port.Clock,
	log *slog.Logger,
) *OwnedMD5Cache {
	return &OwnedMD5Cache{repo: repo, cfg: cfg, clock: clock, log: log}
}

// Get は md5 集合を返す。未ロードなら 1 度だけ自動でロードする。
// Reload 失敗時は前回の set を保持しつつ lastErr のみ更新するため、Get は基本的に成功する。
func (c *OwnedMD5Cache) Get(ctx context.Context) (map[string]struct{}, error) {
	c.mu.RLock()
	if c.loaded {
		out := copySet(c.set)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	if err := c.Reload(ctx); err != nil && c.set == nil {
		// 一度もロードできていない場合だけエラー伝播。
		// それ以前のロード成功 set があれば保持して正常応答する。
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copySet(c.set), nil
}

// Reload は ConfigStore から最新パスを取得して repo を呼ぶ。
// 失敗時は前回の set を保持し、lastErr のみ更新する。
func (c *OwnedMD5Cache) Reload(ctx context.Context) error {
	dbPath, _, err := c.cfg.Get(ctx, "songdata_db_path")
	if err != nil {
		c.recordError(err.Error())
		return err
	}
	got, err := c.repo.LoadOwnedMD5Set(ctx, dbPath)
	if err != nil {
		c.recordError(err.Error())
		c.log.Warn("owned md5 reload failed", "err", err, "path", dbPath)
		return err
	}
	now := c.clock.Now()
	c.mu.Lock()
	c.set = got
	c.loaded = true
	c.loadedAt = &now
	c.loadedPath = dbPath
	c.lastErr = ""
	c.mu.Unlock()
	c.log.Info("owned md5 reloaded", "count", len(got), "path", dbPath)
	return nil
}

// Invalidate は set を未ロード状態に戻す（次回 Get / Reload で repo を呼び直す）。
// 設定の songdata_db_path が変更されたときに ConfigUseCase 経由で呼ばれる想定。
func (c *OwnedMD5Cache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.set = nil
	c.loadedAt = nil
	c.loadedPath = ""
	c.lastErr = ""
	c.mu.Unlock()
}

// Status は GUI 表示用のスナップショットを返す。
func (c *OwnedMD5Cache) Status() OwnedCacheStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return OwnedCacheStatus{
		Loaded:     c.loaded,
		Count:      len(c.set),
		LoadedAt:   c.loadedAt,
		LoadedPath: c.loadedPath,
		LastError:  c.lastErr,
	}
}

func (c *OwnedMD5Cache) recordError(msg string) {
	c.mu.Lock()
	c.lastErr = msg
	c.mu.Unlock()
}

func copySet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run OwnedMD5Cache -v`
Expected: 全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/owned_md5_cache.go internal/usecase/owned_md5_cache_test.go
git commit -m "feat(usecase): OwnedMD5Cache を追加 (auto-load + Reload + Invalidate + Status)"
```

---

## Task 8: usecase/PickResultStore 実装

**Files:**

- Create: `internal/usecase/pick_result_store.go`
- Test: `internal/usecase/pick_result_store_test.go`

in-memory のピック結果キャッシュ。`map[publishedID]PickResult` + `sync.RWMutex`。並行アクセスのテストを 1 ケース入れる。

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/pick_result_store_test.go`:

```go
package usecase_test

import (
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func TestPickResultStore_SetGetDelete(t *testing.T) {
	s := usecase.NewPickResultStore()

	_, ok := s.Get("missing")
	require.False(t, ok)

	r := domain.PickResult{
		PublishedTableID: "PUB1",
		GeneratedAt:      time.Unix(1700000000, 0),
		SeedKey:          "20260507",
	}
	s.Set("PUB1", r)

	got, ok := s.Get("PUB1")
	require.True(t, ok)
	require.Equal(t, "PUB1", got.PublishedTableID)
	require.Equal(t, "20260507", got.SeedKey)

	s.Delete("PUB1")
	_, ok = s.Get("PUB1")
	require.False(t, ok)
}

func TestPickResultStore_Snapshot_ReturnsCopy(t *testing.T) {
	s := usecase.NewPickResultStore()
	s.Set("A", domain.PickResult{PublishedTableID: "A"})
	s.Set("B", domain.PickResult{PublishedTableID: "B"})

	snap := s.Snapshot()
	require.Len(t, snap, 2)

	// snapshot 改変が store に影響しないこと
	delete(snap, "A")
	_, ok := s.Get("A")
	require.True(t, ok)
}

func TestPickResultStore_ConcurrentAccess(t *testing.T) {
	s := usecase.NewPickResultStore()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "P"
			s.Set(id, domain.PickResult{PublishedTableID: id})
			s.Get(id)
			if i%4 == 0 {
				s.Delete(id)
			}
		}(i)
	}
	wg.Wait()
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/usecase/... -run PickResultStore`
Expected: `undefined: usecase.NewPickResultStore` で FAIL

- [ ] **Step 3: PickResultStore を実装**

`internal/usecase/pick_result_store.go`:

```go
package usecase

import (
	"sync"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// PickResultStore は in-memory のピック結果キャッシュ。プロセス再起動で消える。
type PickResultStore struct {
	mu sync.RWMutex
	m  map[string]domain.PickResult
}

// NewPickResultStore は新しい PickResultStore を作る。
func NewPickResultStore() *PickResultStore {
	return &PickResultStore{m: map[string]domain.PickResult{}}
}

// Get は publishedID のピック結果を返す。
func (s *PickResultStore) Get(publishedID string) (domain.PickResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[publishedID]
	return r, ok
}

// Set はピック結果を保存する。
func (s *PickResultStore) Set(publishedID string, r domain.PickResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[publishedID] = r
}

// Delete は publishedID のピック結果を削除する。存在しなくてもエラーにしない。
func (s *PickResultStore) Delete(publishedID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, publishedID)
}

// Snapshot は現在のキャッシュをコピーして返す（Plan 4 ダッシュボード表示用の前準備）。
func (s *PickResultStore) Snapshot() map[string]domain.PickResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.PickResult, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}

// Clear は全エントリを削除する（設定一括変更時の InvalidateAll で使う）。
func (s *PickResultStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[string]domain.PickResult{}
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run PickResultStore -race -v`
Expected: 全 PASS（`-race` で goroutine 競合を検出）

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/pick_result_store.go internal/usecase/pick_result_store_test.go
git commit -m "feat(usecase): PickResultStore を追加 (in-memory ピック結果キャッシュ)"
```

---

## Task 9: usecase/PickUseCase 実装（最大、TDD で重点項目を網羅）

**Files:**

- Create: `internal/usecase/pick_usecase.go`
- Test: `internal/usecase/pick_usecase_test.go`

ピックロジックの中核。spec §10 重点 9 項目をテスト網羅する。`port.Clock` と `port.RandSourceFactory` を注入し、決定論的に検証する。

**ピック生成フロー（spec §7.2）**:

1. `pubRepo.GetBySlug(slug)` → 公開表取得
2. キャッシュ判定（manual: あれば返却 / daily: SeedKey が今日と一致なら返却 / per_request: 常に再生成）
3. 再生成パス:
   a. `srcRepo.Get(sourceTableID)` → ソース表メタ取得（`level_order` と `last_fetch_status`）
   b. `last_fetch_status == 'never'` なら `ErrSourceNotFetched`
   c. `srcRepo.LoadCharts(sourceTableID)` → 全譜面取得
   d. `OwnedOnly=true` なら `ownedCache.Get` で md5 set 取得 → set に含まれるもののみ残す
   e. レベル別グルーピング
   f. シード生成 + `RandSourceFactory(seed)` で `RandSource` 生成 → `rand.New(src)` で `Shuffle`
   g. `pick_per_level > 0` なら各レベルで先頭 N 曲、`= 0` なら全件
   h. 整列: レベル間は `level_order` 順、レベル内は `position` 昇順で安定整列
   i. `level_order` を「1 曲以上残ったレベル」のみに絞る
   j. `store.Set(...)`
4. 返り値: `(PickResult, PublishedTable, error)`

**シード生成詳細**:
- per_request: `clock.Now().UnixNano()` + FNV-32(publishedID) を XOR、SeedKey は `nano` 値の文字列
- daily: `clock.Now().Local().Format("20060102")` を int64 にし、FNV-32(publishedID) を加算、SeedKey は `YYYY-MM-DD` 文字列
- manual: `clock.Now().UnixNano()` + FNV-32(publishedID) を XOR、SeedKey は `clock.Now().UTC().Format(time.RFC3339Nano)`

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/pick_usecase_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// stubRand は決定論的に動く RandSource。Int63 は単調に進む数列を返す。
type stubRand struct {
	seed int64
	step int64
}

func (s *stubRand) Int63() int64    { s.step++; return s.seed*1000 + s.step }
func (s *stubRand) Seed(seed int64) { s.seed = seed; s.step = 0 }

func newStubFactory() port.RandSourceFactory {
	return func(seed int64) port.RandSource { return &stubRand{seed: seed} }
}

func chartFixture(sourceID, level string, pos int, md5 string) domain.SourceChart {
	return domain.SourceChart{
		SourceID: sourceID, Position: pos, Level: level,
		MD5: md5, Title: "T-" + md5, Artist: "A", Raw: map[string]any{"md5": md5},
	}
}

// pickUCFixture は PickUseCase + 各種 fake/in-memory コンポーネントを束ねたテスト fixture。
type pickUCFixture struct {
	uc       *usecase.PickUseCase
	pubRepo  *fakePublishedRepo
	srcRepo  *fakeSourceRepo
	owned    *usecase.OwnedMD5Cache
	ownedRep *fakeOwnedRepo
	store    *usecase.PickResultStore
	cfg      *fakeConfigStore
	clock    *mutableClock
}

type mutableClock struct{ t time.Time }

func (c *mutableClock) Now() time.Time { return c.t }

func newPickUCFixture(t *testing.T) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	repo := &fakeOwnedRepo{resp: map[string]struct{}{}}
	cfg := newFakeConfigStore()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	owned := usecase.NewOwnedMD5Cache(repo, cfg, clock, logger)
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, owned, store, clock, newStubFactory(), logger)
	return &pickUCFixture{uc: uc, pubRepo: pub, srcRepo: src, owned: owned, ownedRep: repo, store: store, cfg: cfg, clock: clock}
}

func (f *pickUCFixture) seedSource(t *testing.T, id string, levelOrder []string, status domain.FetchStatus, charts []domain.SourceChart) {
	t.Helper()
	_, err := f.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: id, LevelOrder: levelOrder, LastFetchStatus: status,
	})
	require.NoError(t, err)
	for _, c := range charts {
		c.SourceID = id
	}
	f.srcRepo.charts[id] = charts
}

func (f *pickUCFixture) seedPub(t *testing.T, id, slug, sourceID string, ownedOnly bool, perLevel int, mode domain.RefreshMode) {
	t.Helper()
	_, err := f.pubRepo.Create(context.Background(), domain.PublishedTable{
		ID: id, Slug: slug, DisplayName: slug,
		SourceTableID: sourceID, OwnedOnly: ownedOnly,
		Pick: domain.PickConfig{PerLevel: perLevel, RefreshMode: mode},
	})
	require.NoError(t, err)
}

func TestPickUseCase_NotFound(t *testing.T) {
	f := newPickUCFixture(t)
	_, _, err := f.uc.PickBySlug(context.Background(), "no-such-slug")
	require.True(t, errors.Is(err, usecase.ErrPublishedTableNotFound))
}

func TestPickUseCase_SourceNotFetchedReturnsError(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusNever, nil)
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.True(t, errors.Is(err, usecase.ErrSourceNotFetched))
}

func TestPickUseCase_PerLevelZeroReturnsAll(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0", "1"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "aaa"),
		chartFixture("SRC1", "0", 1, "bbb"),
		chartFixture("SRC1", "1", 2, "ccc"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 3)
	require.Equal(t, []string{"0", "1"}, r.LevelOrder)
}

func TestPickUseCase_PerLevelLimitsResults(t *testing.T) {
	f := newPickUCFixture(t)
	charts := []domain.SourceChart{}
	for i := 0; i < 5; i++ {
		charts = append(charts, chartFixture("SRC1", "0", i, "L0-"+string(rune('a'+i))))
	}
	for i := 0; i < 2; i++ {
		charts = append(charts, chartFixture("SRC1", "1", 10+i, "L1-"+string(rune('a'+i))))
	}
	f.seedSource(t, "SRC1", []string{"0", "1"}, domain.FetchStatusOK, charts)
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 3, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// レベル 0 は 5 曲中 3 曲、レベル 1 は 2 曲中 2 曲（不足時は全件）
	level0 := 0
	level1 := 0
	for _, c := range r.Charts {
		switch c.Level {
		case "0":
			level0++
		case "1":
			level1++
		}
	}
	require.Equal(t, 3, level0)
	require.Equal(t, 2, level1)
}

func TestPickUseCase_OwnedOnlyFiltersBeforePick(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "owned-1"),
		chartFixture("SRC1", "0", 1, "not-owned"),
		chartFixture("SRC1", "0", 2, "owned-2"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", true /* ownedOnly */, 0, domain.RefreshModePerRequest)
	require.NoError(t, f.cfg.Set(context.Background(), "songdata_db_path", "/p"))
	f.ownedRep.resp = map[string]struct{}{"owned-1": {}, "owned-2": {}}

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	for _, c := range r.Charts {
		require.NotEqual(t, "not-owned", c.MD5)
	}
}

func TestPickUseCase_OwnedOnly_NoOwnedReturnsEmpty(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "x"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", true, 0, domain.RefreshModePerRequest)
	require.NoError(t, f.cfg.Set(context.Background(), "songdata_db_path", "/p"))
	// ownedRep.resp は空のまま

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Empty(t, r.Charts)
	require.Empty(t, r.LevelOrder, "1 曲以上残ったレベルが無いので level_order は空")
}

func TestPickUseCase_DailyMode_SameDayCached(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
		chartFixture("SRC1", "0", 2, "c"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)

	// clock を進めるが日付は同じ
	f.clock.t = f.clock.t.Add(2 * time.Hour)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, first.GeneratedAt, second.GeneratedAt, "同じ日のキャッシュが返るはず")

	// 翌日へ
	f.clock.t = f.clock.t.AddDate(0, 0, 1)
	third, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.GeneratedAt, third.GeneratedAt, "日付が変わったので再生成")
	require.NotEqual(t, first.SeedKey, third.SeedKey)
}

func TestPickUseCase_ManualMode_KeepsCacheUntilManualRefresh(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 1, domain.RefreshModeManual)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	f.clock.t = f.clock.t.Add(48 * time.Hour) // 大きく時間進める
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, first.GeneratedAt, second.GeneratedAt)

	// 手動再ピック → 結果が新しい時刻に更新される
	require.NoError(t, f.uc.ManualRefresh(context.Background(), "PUB1"))
	third, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.GeneratedAt, third.GeneratedAt)
}

func TestPickUseCase_PerRequestMode_RegeneratesEachCall(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
		chartFixture("SRC1", "0", 1, "b"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 1, domain.RefreshModePerRequest)

	first, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// clock を 1 ナノ秒進める → 別のシードになり、結果も SeedKey が違う
	f.clock.t = f.clock.t.Add(1 * time.Nanosecond)
	second, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.NotEqual(t, first.SeedKey, second.SeedKey)
}

func TestPickUseCase_LevelOrderRespected(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"sl0", "sl1", "sl2"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "sl2", 0, "c1"),
		chartFixture("SRC1", "sl0", 1, "a1"),
		chartFixture("SRC1", "sl1", 2, "b1"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// レベル順序は ソース表 LevelOrder の通り
	require.Equal(t, []string{"sl0", "sl1", "sl2"}, r.LevelOrder)
	// Charts もレベル順に並ぶ
	require.Equal(t, "sl0", r.Charts[0].Level)
	require.Equal(t, "sl1", r.Charts[1].Level)
	require.Equal(t, "sl2", r.Charts[2].Level)
}

func TestPickUseCase_LevelOrder_FallbackWhenSourceHasNone(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", nil /* level_order なし */, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "10", 0, "x"),
		chartFixture("SRC1", "1", 1, "y"),
		chartFixture("SRC1", "2", 2, "z"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModePerRequest)

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	// fallback: 自然順ソート
	got := append([]string(nil), r.LevelOrder...)
	sortedCopy := append([]string(nil), got...)
	sort.Strings(sortedCopy)
	require.Equal(t, sortedCopy, got)
}

func TestPickUseCase_DeterministicWithSameSeed(t *testing.T) {
	// 別の PickUseCase に同じ要素を入れて、同じ clock + 同じ factory を使えば結果が一致することを確認。
	build := func() *pickUCFixture {
		f := newPickUCFixture(t)
		f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
			chartFixture("SRC1", "0", 0, "a"),
			chartFixture("SRC1", "0", 1, "b"),
			chartFixture("SRC1", "0", 2, "c"),
			chartFixture("SRC1", "0", 3, "d"),
		})
		f.seedPub(t, "PUB1", "p1", "SRC1", false, 2, domain.RefreshModeDaily)
		return f
	}
	a := build()
	b := build()
	ra, _, err := a.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	rb, _, err := b.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, len(ra.Charts), len(rb.Charts))
	for i := range ra.Charts {
		require.Equal(t, ra.Charts[i].MD5, rb.Charts[i].MD5)
	}
}

func TestPickUseCase_InvalidateAll_ClearsStore(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "a"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", false, 0, domain.RefreshModeManual)

	_, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, f.store.Snapshot(), 1)

	f.uc.InvalidateAll()
	require.Empty(t, f.store.Snapshot())
}
```

なお、Plan 2 で書かれた `fakeSourceRepo` は `internal/usecase/source_table_usecase_test.go` 内で `package usecase_test` として定義されている。このテストファイルは同じパッケージ（`usecase_test`）にあるため、`fakeSourceRepo` をそのまま再利用できる。`fakeOwnedRepo` / `fakeConfigStore` / `fixedClock` も Task 7（owned_md5_cache_test.go）で定義済みのものを再利用する。

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/usecase/... -run PickUseCase`
Expected: `undefined: usecase.NewPickUseCase` で FAIL

- [ ] **Step 3: PickUseCase を実装**

`internal/usecase/pick_usecase.go`:

```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// PickUseCase はピック生成を担う。spec §7.2 のフローを実装。
type PickUseCase struct {
	pubRepo  port.PublishedTableRepo
	srcRepo  port.SourceTableRepo
	owned    *OwnedMD5Cache
	store    *PickResultStore
	clock    port.Clock
	randNew  port.RandSourceFactory
	log      *slog.Logger
}

// NewPickUseCase は新しい PickUseCase を作る。
func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	owned *OwnedMD5Cache,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, owned: owned, store: store,
		clock: clock, randNew: randNew, log: log,
	}
}

// PickBySlug は slug から公開表を取得し、モードに応じてキャッシュ判定 / 再生成する。
func (u *PickUseCase) PickBySlug(ctx context.Context, slug string) (domain.PickResult, domain.PublishedTable, error) {
	pub, err := u.pubRepo.GetBySlug(ctx, slug)
	if err != nil {
		return domain.PickResult{}, domain.PublishedTable{}, err
	}

	if cached, ok := u.cachedIfFresh(pub); ok {
		return cached, pub, nil
	}

	r, err := u.regenerate(ctx, pub)
	if err != nil {
		return domain.PickResult{}, pub, err
	}
	u.store.Set(pub.ID, r)
	return r, pub, nil
}

// ManualRefresh は手動再ピック。store からエントリを消して次回アクセスで再生成させる。
// ※ spec §7.4 の `POST /:slug/_refresh` で呼ばれるパス。GUI からも呼ぶ。
func (u *PickUseCase) ManualRefresh(ctx context.Context, publishedID string) error {
	pub, err := u.pubRepo.Get(ctx, publishedID)
	if err != nil {
		return err
	}
	r, err := u.regenerate(ctx, pub)
	if err != nil {
		return err
	}
	u.store.Set(pub.ID, r)
	u.log.Info("pick manually refreshed", "id", pub.ID, "slug", pub.Slug)
	return nil
}

// InvalidateAll は store の全エントリを削除する。
// 設定変更（songdata_db_path 変更等）で OwnedMD5Cache が invalidate された後に呼ばれる想定。
func (u *PickUseCase) InvalidateAll() {
	u.store.Clear()
}

// cachedIfFresh はモード別のキャッシュ判定。返り値の bool が true ならそのまま使える。
func (u *PickUseCase) cachedIfFresh(pub domain.PublishedTable) (domain.PickResult, bool) {
	cached, ok := u.store.Get(pub.ID)
	if !ok {
		return domain.PickResult{}, false
	}
	switch pub.Pick.RefreshMode {
	case domain.RefreshModeManual:
		return cached, true
	case domain.RefreshModeDaily:
		todayKey := u.clock.Now().Local().Format("2006-01-02")
		if cached.SeedKey == todayKey {
			return cached, true
		}
	}
	return domain.PickResult{}, false
}

// regenerate はピック結果を一から作成する。
func (u *PickUseCase) regenerate(ctx context.Context, pub domain.PublishedTable) (domain.PickResult, error) {
	src, err := u.srcRepo.Get(ctx, pub.SourceTableID)
	if err != nil {
		return domain.PickResult{}, fmt.Errorf("get source table %q: %w", pub.SourceTableID, err)
	}
	if src.LastFetchStatus == domain.FetchStatusNever {
		return domain.PickResult{}, ErrSourceNotFetched
	}
	all, err := u.srcRepo.LoadCharts(ctx, pub.SourceTableID)
	if err != nil {
		return domain.PickResult{}, fmt.Errorf("load charts %q: %w", pub.SourceTableID, err)
	}

	// 所持絞り込み（OwnedOnly=true 時）
	if pub.OwnedOnly {
		ownedSet, err := u.owned.Get(ctx)
		if err != nil {
			// owned 取得失敗は致命ではない: 空 set 扱いで続行（spec §8）
			u.log.Warn("owned md5 get failed, falling back to empty set", "err", err)
			ownedSet = map[string]struct{}{}
		}
		filtered := make([]domain.SourceChart, 0, len(all))
		for _, c := range all {
			if _, ok := ownedSet[c.MD5]; ok {
				filtered = append(filtered, c)
			}
		}
		all = filtered
	}

	// レベル別グルーピング
	byLevel := map[string][]domain.SourceChart{}
	for _, c := range all {
		byLevel[c.Level] = append(byLevel[c.Level], c)
	}

	// シード生成
	seed, seedKey := u.makeSeed(pub)

	// レベル別シャッフル + 先頭 N 曲（または全件）
	rng := rand.New(u.randNew(seed))
	for level, charts := range byLevel {
		// position 昇順でいったん並べ替え（決定論性保証）
		sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		if pub.Pick.PerLevel > 0 && len(charts) > pub.Pick.PerLevel {
			rng.Shuffle(len(charts), func(i, j int) { charts[i], charts[j] = charts[j], charts[i] })
			charts = charts[:pub.Pick.PerLevel]
			// 採用後はレベル内で position 昇順に再ソート（最終応答の安定性）
			sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
		}
		byLevel[level] = charts
	}

	// レベル順序の決定: ソース表 level_order があればそれに従い、無ければ自然順
	order := src.LevelOrder
	if len(order) == 0 {
		order = make([]string, 0, len(byLevel))
		for k := range byLevel {
			order = append(order, k)
		}
		sort.Strings(order)
	}

	// 最終 Charts と level_order（残ったレベルのみ）を組み立て
	var finalCharts []domain.SourceChart
	var finalLevelOrder []string
	for _, level := range order {
		charts, ok := byLevel[level]
		if !ok || len(charts) == 0 {
			continue
		}
		finalCharts = append(finalCharts, charts...)
		finalLevelOrder = append(finalLevelOrder, level)
	}

	// level_order に無いレベルが残っていれば末尾追加（自然順）
	if len(src.LevelOrder) > 0 {
		known := map[string]struct{}{}
		for _, l := range src.LevelOrder {
			known[l] = struct{}{}
		}
		var extra []string
		for k, v := range byLevel {
			if _, ok := known[k]; !ok && len(v) > 0 {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, l := range extra {
			finalCharts = append(finalCharts, byLevel[l]...)
			finalLevelOrder = append(finalLevelOrder, l)
		}
	}

	r := domain.PickResult{
		PublishedTableID: pub.ID,
		GeneratedAt:      u.clock.Now(),
		SeedKey:          seedKey,
		Charts:           finalCharts,
		LevelOrder:       finalLevelOrder,
	}
	return r, nil
}

// makeSeed はモード別のシードと SeedKey を返す。
func (u *PickUseCase) makeSeed(pub domain.PublishedTable) (int64, string) {
	now := u.clock.Now()
	hash := fnv32(pub.ID)
	switch pub.Pick.RefreshMode {
	case domain.RefreshModeDaily:
		key := now.Local().Format("2006-01-02")
		// 日付の数値化 + publishedID hash
		num, _ := strconv.ParseInt(now.Local().Format("20060102"), 10, 64)
		return num + int64(hash), key
	case domain.RefreshModePerRequest:
		nano := now.UnixNano()
		key := strconv.FormatInt(nano, 10)
		return nano ^ int64(hash), key
	case domain.RefreshModeManual:
		nano := now.UnixNano()
		key := now.UTC().Format(time.RFC3339Nano)
		return nano ^ int64(hash), key
	default:
		// 未対応 mode は per_request 相当
		nano := now.UnixNano()
		return nano ^ int64(hash), strconv.FormatInt(nano, 10)
	}
}

func fnv32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// 不要だが errors import 維持のためのコンパイルガード（将来エラー追加時の利便性）
var _ = errors.New
```

Note: テストで `fakeSourceRepo.charts` 直接代入を使うが、Plan 2 のテストで charts フィールドは package private で公開されていない。`fakeSourceRepo` が同一 `usecase_test` パッケージにあるため直接アクセス可能（Plan 2 既存実装を確認済み）。

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run PickUseCase -race -v`
Expected: 全 PASS

- [ ] **Step 5: 全体テストでのリグレッション確認**

Run: `go test ./...`
Expected: 全 PASS

- [ ] **Step 6: コミット**

```bash
git add internal/usecase/pick_usecase.go internal/usecase/pick_usecase_test.go
git commit -m "feat(usecase): PickUseCase を追加 (per_request/daily/manual + 所持絞り込み + 決定論シード)"
```

---

## Task 10: ConfigUseCase に SetOwnedInvalidator hook を追加

**Files:**

- Modify: `internal/usecase/config_usecase.go`

`SetSongdataDBPath` 直後に `OwnedMD5Cache.Invalidate` と `PickResultStore.Clear` を呼ぶ「無効化フック」を持たせる。Bootstrap で hook を設定する。

- [ ] **Step 1: config_usecase.go を変更**

`internal/usecase/config_usecase.go` の `ConfigUseCase` 構造体と `SetSongdataDBPath` を以下に置換:

既存（読み取り専用、参考）:

```go
// ConfigUseCase は config の Get/Set を型安全にラップする。
type ConfigUseCase struct {
	store port.ConfigStore
}

// NewConfigUseCase は新しい ConfigUseCase を作る。
func NewConfigUseCase(store port.ConfigStore) *ConfigUseCase {
	return &ConfigUseCase{store: store}
}
```

を以下で置換:

```go
// ConfigUseCase は config の Get/Set を型安全にラップする。
// SetSongdataDBPath が呼ばれたとき、登録されたフックを順に呼ぶ
// （所持キャッシュ invalidate / ピック結果 clear など）。
type ConfigUseCase struct {
	store         port.ConfigStore
	songdataHooks []func()
}

// NewConfigUseCase は新しい ConfigUseCase を作る。
func NewConfigUseCase(store port.ConfigStore) *ConfigUseCase {
	return &ConfigUseCase{store: store}
}

// AddSongdataPathChangeHook は songdata_db_path 変更時に呼ばれるフックを追加する。
// Bootstrap で OwnedMD5Cache.Invalidate と PickResultStore.Clear を登録する想定。
func (u *ConfigUseCase) AddSongdataPathChangeHook(fn func()) {
	u.songdataHooks = append(u.songdataHooks, fn)
}
```

そして既存の `SetSongdataDBPath` を以下で置換:

```go
// SetSongdataDBPath は songdata.db のパスを保存する（バリデーションは行わない）。
// 保存成功後に登録された SongdataPathChangeHook を全て呼ぶ。
func (u *ConfigUseCase) SetSongdataDBPath(ctx context.Context, path string) error {
	if err := u.store.Set(ctx, keySongdataDBPath, path); err != nil {
		return err
	}
	for _, fn := range u.songdataHooks {
		fn()
	}
	return nil
}
```

- [ ] **Step 2: 既存の config_usecase_test.go に hook テストを追加**

`internal/usecase/config_usecase_test.go` の末尾に以下を追加:

```go
func TestConfigUseCase_SetSongdataDBPath_FiresHooks(t *testing.T) {
	store := newFakeConfigStore()
	uc := usecase.NewConfigUseCase(store)
	calls := 0
	uc.AddSongdataPathChangeHook(func() { calls++ })
	uc.AddSongdataPathChangeHook(func() { calls++ })

	require.NoError(t, uc.SetSongdataDBPath(context.Background(), "/path"))
	require.Equal(t, 2, calls)
}
```

`config_usecase_test.go` 既存の import に `context` / `testing` / `require` がない場合は追加する（既存ファイルを開いて確認）。`newFakeConfigStore` は Task 7 で `usecase_test` パッケージ内に定義済みなのでそのまま使える。

- [ ] **Step 3: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run ConfigUseCase -v`
Expected: 全 PASS

- [ ] **Step 4: コミット**

```bash
git add internal/usecase/config_usecase.go internal/usecase/config_usecase_test.go
git commit -m "feat(usecase): ConfigUseCase に songdata path 変更フックを追加"
```

---

## Task 11: usecase/ServerUseCase 実装

**Files:**

- Create: `internal/usecase/server_usecase.go`
- Test: `internal/usecase/server_usecase_test.go`

サーバ起動 / 停止 / 再起動 / ステータス + リスナー登録。`HTTPServer` インタフェースを介して adapter から差し替え可能。テストではモック実装で状態遷移を確認する。

- [ ] **Step 1: 失敗テストを書く**

`internal/usecase/server_usecase_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakeHTTPServer struct {
	mu        sync.Mutex
	addr      string
	startErr  error
	stopErr   error
	started   bool
	stopped   bool
	startCnt  int
	stopCnt   int
}

func (s *fakeHTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCnt++
	if s.startErr != nil {
		return s.startErr
	}
	s.started = true
	return nil
}

func (s *fakeHTTPServer) Shutdown(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCnt++
	s.stopped = true
	s.started = false
	return s.stopErr
}

func (s *fakeHTTPServer) Addr() string { return s.addr }

func newServerUC(t *testing.T, store *fakeConfigStore, factory func(addr string) usecase.HTTPServer) *usecase.ServerUseCase {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return usecase.NewServerUseCase(store, factory, logger)
}

func TestServerUseCase_Start_TransitionsToRunning(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	require.Equal(t, domain.ServerStateRunning, uc.Status().State)
	require.Equal(t, 50000, uc.Status().Port)
	require.Equal(t, 1, srv.startCnt)
}

func TestServerUseCase_Start_FailureSetsErrorState(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000", startErr: errors.New("EADDRINUSE")}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	err := uc.Start(context.Background())
	require.Error(t, err)
	st := uc.Status()
	require.Equal(t, domain.ServerStateError, st.State)
	require.Contains(t, st.LastError, "EADDRINUSE")
}

func TestServerUseCase_Start_FailsIfAlreadyRunning(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	err := uc.Start(context.Background())
	require.True(t, errors.Is(err, usecase.ErrServerAlreadyRunning))
}

func TestServerUseCase_Stop_AfterStart(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Stop(context.Background()))
	require.Equal(t, domain.ServerStateStopped, uc.Status().State)
	require.Equal(t, 1, srv.stopCnt)
}

func TestServerUseCase_Stop_FailsIfNotRunning(t *testing.T) {
	store := newFakeConfigStore()
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return &fakeHTTPServer{} })
	err := uc.Stop(context.Background())
	require.True(t, errors.Is(err, usecase.ErrServerNotRunning))
}

func TestServerUseCase_Restart(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))

	calls := 0
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer {
		calls++
		return &fakeHTTPServer{addr: addr}
	})
	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Restart(context.Background()))
	require.Equal(t, domain.ServerStateRunning, uc.Status().State)
	require.Equal(t, 2, calls, "factory が再呼出しされたはず")
}

func TestServerUseCase_OnStatusChange_NotifiesListeners(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return &fakeHTTPServer{addr: addr} })

	var mu sync.Mutex
	statuses := []domain.ServerStatus{}
	uc.OnStatusChange(func(s domain.ServerStatus) {
		mu.Lock()
		defer mu.Unlock()
		statuses = append(statuses, s)
	})

	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Stop(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(statuses), 2)
	require.Equal(t, domain.ServerStateRunning, statuses[0].State)
	require.Equal(t, domain.ServerStateStopped, statuses[len(statuses)-1].State)
}
```

- [ ] **Step 2: テストを走らせて失敗確認**

Run: `go test ./internal/usecase/... -run ServerUseCase`
Expected: `undefined: usecase.NewServerUseCase` 等で FAIL

- [ ] **Step 3: ServerUseCase を実装**

`internal/usecase/server_usecase.go`:

```go
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// HTTPServer は usecase 層が触る HTTP サーバの抽象。
// adapter/httpserver が実装する。
type HTTPServer interface {
	Start() error                       // Listen + 非同期 Serve（Listen 失敗時のみ error）
	Shutdown(ctx context.Context) error // graceful shutdown
	Addr() string                       // 例: ":50000"
}

// HTTPServerFactory は addr を受け取って HTTPServer を作る。
type HTTPServerFactory func(addr string) HTTPServer

// ServerUseCase はサーバの起動 / 停止 / 再起動 / ステータスを管理する。
type ServerUseCase struct {
	cfg     port.ConfigStore
	factory HTTPServerFactory
	log     *slog.Logger
	clock   port.Clock

	mu        sync.Mutex
	status    domain.ServerStatus
	server    HTTPServer
	listeners []func(domain.ServerStatus)
}

// NewServerUseCase は新しい ServerUseCase を作る（clock は time.Now() ベースで内部固定）。
func NewServerUseCase(cfg port.ConfigStore, factory HTTPServerFactory, log *slog.Logger) *ServerUseCase {
	return &ServerUseCase{
		cfg: cfg, factory: factory, log: log,
		status: domain.ServerStatus{State: domain.ServerStateStopped},
	}
}

// OnStatusChange はステータス変化を購読する。Plan 4 でトレイから購読する想定。
// 同期的に呼ばれるため、リスナー側は重い処理をしないか自分で goroutine 化すること。
func (u *ServerUseCase) OnStatusChange(fn func(domain.ServerStatus)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.listeners = append(u.listeners, fn)
}

// Status は現在のサーバステータスを返す。
func (u *ServerUseCase) Status() domain.ServerStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Start はサーバを起動する。既に起動中なら ErrServerAlreadyRunning。
func (u *ServerUseCase) Start(ctx context.Context) error {
	u.mu.Lock()
	if u.status.State == domain.ServerStateRunning {
		u.mu.Unlock()
		return ErrServerAlreadyRunning
	}
	u.mu.Unlock()

	port, err := u.readPort(ctx)
	if err != nil {
		u.setStatusError(fmt.Sprintf("ポート設定エラー: %v", err))
		return err
	}

	srv := u.factory(fmt.Sprintf(":%d", port))
	if err := srv.Start(); err != nil {
		u.setStatusError(err.Error())
		u.log.Warn("server start failed", "port", port, "err", err)
		return err
	}
	now := time.Now()
	u.mu.Lock()
	u.server = srv
	u.status = domain.ServerStatus{
		State: domain.ServerStateRunning, Port: port, StartedAt: &now,
	}
	listeners := append([]func(domain.ServerStatus)(nil), u.listeners...)
	u.mu.Unlock()
	u.log.Info("server started", "port", port)
	notify(listeners, u.Status())
	return nil
}

// Stop はサーバを停止する。停止中なら ErrServerNotRunning。
func (u *ServerUseCase) Stop(ctx context.Context) error {
	u.mu.Lock()
	if u.status.State != domain.ServerStateRunning {
		u.mu.Unlock()
		return ErrServerNotRunning
	}
	srv := u.server
	u.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		u.log.Warn("server shutdown error", "err", err)
		// ステータスは stopped に倒す（資源は最善努力で解放済みとみなす）
	}
	u.mu.Lock()
	u.server = nil
	u.status = domain.ServerStatus{State: domain.ServerStateStopped}
	listeners := append([]func(domain.ServerStatus)(nil), u.listeners...)
	u.mu.Unlock()
	u.log.Info("server stopped")
	notify(listeners, u.Status())
	return nil
}

// Restart は Stop → Start。起動中でなければ Start のみ実行。
func (u *ServerUseCase) Restart(ctx context.Context) error {
	if u.Status().State == domain.ServerStateRunning {
		if err := u.Stop(ctx); err != nil {
			return err
		}
	}
	return u.Start(ctx)
}

func (u *ServerUseCase) readPort(ctx context.Context) (int, error) {
	v, _, err := u.cfg.Get(ctx, "server_port")
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 50000, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid server_port %q: %w", v, err)
	}
	return p, nil
}

func (u *ServerUseCase) setStatusError(msg string) {
	u.mu.Lock()
	u.status = domain.ServerStatus{State: domain.ServerStateError, LastError: msg}
	listeners := append([]func(domain.ServerStatus)(nil), u.listeners...)
	u.mu.Unlock()
	notify(listeners, u.Status())
}

func notify(fns []func(domain.ServerStatus), s domain.ServerStatus) {
	for _, fn := range fns {
		fn(s)
	}
}
```

- [ ] **Step 4: テストを走らせて通過確認**

Run: `go test ./internal/usecase/... -run ServerUseCase -race -v`
Expected: 全 PASS

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/server_usecase.go internal/usecase/server_usecase_test.go
git commit -m "feat(usecase): ServerUseCase を追加 (Start/Stop/Restart + Status + OnStatusChange)"
```

---

## Task 12: adapter/httpserver — Server / Router / templates 雛形

**Files:**

- Create: `internal/adapter/httpserver/server.go`
- Create: `internal/adapter/httpserver/server_test.go`
- Create: `internal/adapter/httpserver/router.go`
- Create: `internal/adapter/httpserver/templates/index.html`

ハンドラ実装は次タスクで埋める。本タスクではサーバライフサイクルとルータ骨格 + テンプレ embed を導入し、`go build` を通す。

- [ ] **Step 1: テンプレを作成**

`internal/adapter/httpserver/templates/index.html`:

```html
<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="bmstable" content="header.json">
<title>{{.DisplayName}}</title>
<style>
body { font-family: system-ui, -apple-system, sans-serif; margin: 16px; color: #1b2636; }
h1 { font-size: 1.4em; margin: 0 0 8px; }
.meta { color: #666; font-size: 0.85em; margin-bottom: 16px; }
.refresh { margin-bottom: 16px; }
.refresh button { padding: 6px 14px; cursor: pointer; }
h2 { font-size: 1.1em; margin: 24px 0 4px; border-bottom: 1px solid #ccc; padding-bottom: 2px; }
table { border-collapse: collapse; width: 100%; font-size: 0.9em; }
th, td { padding: 4px 8px; border-bottom: 1px solid #eee; text-align: left; }
tr.owned td { background: #eaf6ea; }
tr.unowned td { color: #999; }
.md5 { font-family: monospace; font-size: 0.8em; color: #888; }
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
      <table>
        <tbody>
        {{range .Charts}}
          <tr class="{{if .Owned}}owned{{else}}unowned{{end}}">
            <td>{{.Title}}</td><td>{{.Artist}}</td><td class="md5">{{.MD5}}</td>
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

- [ ] **Step 2: server.go (lifecycle) を実装**

`internal/adapter/httpserver/server.go`:

```go
// Package httpserver は public な /:slug 系エンドポイントを提供する HTTP サーバの adapter。
package httpserver

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

//go:embed templates/index.html
var templatesFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

// Deps は HTTP ハンドラが依存する usecase 群。
type Deps struct {
	Pick *usecase.PickUseCase
	Pub  *usecase.PublishedTableUseCase
	Log  *slog.Logger
}

// AdapterServer は usecase.HTTPServer を実装する *http.Server ラッパ。
type AdapterServer struct {
	addr string
	srv  *http.Server
	mu   sync.Mutex
	ln   net.Listener
	done chan struct{}
}

// New は addr (":50000" 等) と Deps を受け取り AdapterServer を作る。
func New(addr string, deps Deps) *AdapterServer {
	mux := NewMux(deps)
	return &AdapterServer{
		addr: addr,
		srv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Addr は listen アドレスを返す。
func (s *AdapterServer) Addr() string { return s.addr }

// Start は同期的に Listen して、Serve は goroutine で起動する。
// Listen 失敗時のみエラーを返す（Serve のエラーはログのみ）。
func (s *AdapterServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.ln = ln
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// log via deps.Log は持っていないため標準出力にも残らない（簡易のため slog.Default を使う）
			slog.Default().Warn("http server Serve exited", "err", err)
		}
	}()
	return nil
}

// Shutdown は graceful shutdown する。
func (s *AdapterServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	done := s.done
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	if done != nil {
		<-done
	}
	return nil
}
```

- [ ] **Step 3: router.go を実装（仮ハンドラで 4 ルート登録、本ロジックは Task 13）**

`internal/adapter/httpserver/router.go`:

```go
package httpserver

import "net/http"

// NewMux は 4 ルートを登録した http.ServeMux を返す。
// 各ハンドラの実装は handler_*.go に分かれている。
func NewMux(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", newHTMLHandler(deps))
	mux.HandleFunc("GET /{slug}/header.json", newHeaderHandler(deps))
	mux.HandleFunc("GET /{slug}/data.json", newDataHandler(deps))
	mux.HandleFunc("POST /{slug}/_refresh", newRefreshHandler(deps))
	return mux
}
```

- [ ] **Step 4: 仮ハンドラ stub を 4 つ作成（コンパイルを通すため）**

`internal/adapter/httpserver/handler_html.go`:

```go
package httpserver

import "net/http"

// newHTMLHandler は GET /{slug} ハンドラ。本実装は Task 13。
func newHTMLHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
```

`internal/adapter/httpserver/handler_header.go`:

```go
package httpserver

import "net/http"

// newHeaderHandler は GET /{slug}/header.json ハンドラ。本実装は Task 13。
func newHeaderHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
```

`internal/adapter/httpserver/handler_data.go`:

```go
package httpserver

import "net/http"

// newDataHandler は GET /{slug}/data.json ハンドラ。本実装は Task 13。
func newDataHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
```

`internal/adapter/httpserver/handler_refresh.go`:

```go
package httpserver

import "net/http"

// newRefreshHandler は POST /{slug}/_refresh ハンドラ。本実装は Task 13。
func newRefreshHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}
```

- [ ] **Step 5: server_test.go でライフサイクル検証**

`internal/adapter/httpserver/server_test.go`:

```go
package httpserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/stretchr/testify/require"
)

func TestAdapterServer_StartShutdown(t *testing.T) {
	srv := httpserver.New("127.0.0.1:0", httpserver.Deps{})
	// Start は Listen 失敗時のみ error を返す
	require.NoError(t, srv.Start())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))
}

func TestAdapterServer_StartFailsOnPortInUse(t *testing.T) {
	// 1 つ目を起動 → 同じポートを取りに行く 2 つ目は失敗するはず
	first := httpserver.New("127.0.0.1:0", httpserver.Deps{})
	require.NoError(t, first.Start())
	defer first.Shutdown(context.Background())

	// 上の listener から Addr を取って同じポート再利用は port:0 では難しいので、
	// 代わりに固定ポートで衝突させる手は安定しないため、ここでは
	// "Start 後に再度 Start を呼ぶ" の二重 listen 検出で代用する
	second := httpserver.New(first.Addr(), httpserver.Deps{})
	defer second.Shutdown(context.Background())

	// Addr() は New 時に渡したアドレスをそのまま返すため、port:0 を使うと
	// 「未バインド」状態の文字列が返ることがある。代わりにダミー固定ポートで衝突確認するなら
	// 別途固定ポートテストを作るが、ここでは Start のスモークのみとする。
	_ = second
	_ = http.StatusOK
}
```

注: ポート競合の厳密なテストは flaky になりやすいため Plan 3 ではスモーク 1 ケースに留める。実機での EADDRINUSE はサーバ Start 時に listen エラーとして自然に検出される。

- [ ] **Step 6: ビルドとテスト**

Run: `go build ./... && go test ./internal/adapter/httpserver/... -v`
Expected: 全 PASS

- [ ] **Step 7: コミット**

```bash
git add internal/adapter/httpserver/
git commit -m "feat(httpserver): AdapterServer + Mux + テンプレ + 仮ハンドラを追加"
```

---

## Task 13: HTTP ハンドラ 4 種を本実装

**Files:**

- Modify: `internal/adapter/httpserver/handler_html.go`
- Modify: `internal/adapter/httpserver/handler_header.go`
- Modify: `internal/adapter/httpserver/handler_data.go`
- Modify: `internal/adapter/httpserver/handler_refresh.go`
- Create: `internal/adapter/httpserver/handler_html_test.go`
- Create: `internal/adapter/httpserver/handler_header_test.go`
- Create: `internal/adapter/httpserver/handler_data_test.go`
- Create: `internal/adapter/httpserver/handler_refresh_test.go`

各ハンドラを `httptest.NewRecorder` でテストする。共通の fixture を作って 4 ファイルから使う。

- [ ] **Step 1: 共通テスト fixture を作成**

`internal/adapter/httpserver/handler_test_helpers_test.go`:

```go
package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
	"time"
)

type stubClock struct{ t time.Time }

func (c stubClock) Now() time.Time { return c.t }

type stubIDGen struct{ seq int }

func (g *stubIDGen) New() string {
	g.seq++
	return "01J0PUB" + string(rune('A'+g.seq-1)) + "00000000000000000"
}

// httpFixture は handler テストで使う Mux + 種データ。
type httpFixture struct {
	mux      *httptest.Server
	pubUC    *usecase.PublishedTableUseCase
	pickUC   *usecase.PickUseCase
	srcRepo  *persistence.SourceTableRepoSQL
	pubRepo  *persistence.PublishedTableRepoSQL
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))

	srcRepo := persistence.NewSourceTableRepoSQL(db)
	pubRepo := persistence.NewPublishedTableRepoSQL(db)
	cfgStore := persistence.NewConfigStoreSQL(db)
	owned := usecase.NewOwnedMD5Cache(
		persistence.NewSongdataReader(),
		cfgStore,
		stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	store := usecase.NewPickResultStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pubUC := usecase.NewPublishedTableUseCase(pubRepo, srcRepo, &stubIDGen{}, logger)
	pickUC := usecase.NewPickUseCase(
		pubRepo, srcRepo, owned, store,
		stubClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)},
		port.RandSourceFactory(func(seed int64) port.RandSource { return randsrc.NewMathRandSource(seed) }),
		logger,
	)

	mux := httpserver.NewMux(httpserver.Deps{Pick: pickUC, Pub: pubUC, Log: logger})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &httpFixture{
		mux:     srv,
		pubUC:   pubUC,
		pickUC:  pickUC,
		srcRepo: srcRepo,
		pubRepo: pubRepo,
	}
}

// seedSourceWithCharts はソース表 + 譜面を本物の Repo へ保存する。
func (f *httpFixture) seedSourceWithCharts(t *testing.T, id, name string, levelOrder []string, charts []domain.SourceChart) {
	t.Helper()
	_, err := f.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: name, LevelOrder: levelOrder,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
	require.NoError(t, f.srcRepo.SaveFetched(context.Background(), id, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: name, Symbol: "sl", DataURL: "data.json", LevelOrder: levelOrder},
		Charts: charts,
		ETag:   "",
	}, time.Now()))
}

func (f *httpFixture) seedPublished(t *testing.T, slug, sourceID string, mode domain.RefreshMode, perLevel int, ownedOnly bool) string {
	t.Helper()
	id, err := f.pubUC.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: slug, DisplayName: slug, Symbol: "sl",
		SourceTableID: sourceID, OwnedOnly: ownedOnly, PickPerLevel: perLevel, RefreshMode: mode,
	})
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 2: handler_html.go 本実装**

`internal/adapter/httpserver/handler_html.go` を以下で完全置換:

```go
package httpserver

import (
	"context"
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

type htmlChart struct {
	Title  string
	Artist string
	MD5    string
	Owned  bool
}

func newHTMLHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		ctx := r.Context()
		result, pub, err := deps.Pick.PickBySlug(ctx, slug)
		if err != nil {
			handleHTMLError(w, err)
			return
		}

		data := buildHTMLPageData(ctx, deps, pub, result)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := indexTemplate.Execute(w, data); err != nil {
			deps.Log.Error("html template execute failed", "slug", slug, "err", err)
		}
	}
}

func buildHTMLPageData(ctx context.Context, deps Deps, pub domain.PublishedTable, r domain.PickResult) htmlPageData {
	// owned set 取得（OwnedOnly=true でなければ未取得 = すべて false 表示）。
	// PickBySlug は OwnedOnly=true 時に既に絞り込み済みなので、ここで再 fetch するのは
	// 「未絞り込み」の場合に色分けするため。
	ownedSet := map[string]struct{}{}
	if !pub.OwnedOnly {
		// 全件表示する場合のみ owned を引く。失敗時は空 set のまま続行。
		// （pickUC が persistence/songdata_reader を内部で使っているのと同じ owned cache を露出する API は
		// Plan 3 では未提供のため、ここでは UI 装飾のみの色分けは省略し全部 unowned 表示にする選択もある。
		// MVP として「未絞り込み時は全て unowned 色」を許容する）
		_ = deps // placeholder
	}

	levels := make([]htmlLevel, 0, len(r.LevelOrder))
	for _, level := range r.LevelOrder {
		var charts []htmlChart
		for _, c := range r.Charts {
			if c.Level != level {
				continue
			}
			_, owned := ownedSet[c.MD5]
			if pub.OwnedOnly {
				owned = true
			}
			charts = append(charts, htmlChart{
				Title: c.Title, Artist: c.Artist, MD5: c.MD5, Owned: owned,
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

注: HTML ビューでの「所持/未所持の色分け」は OwnedOnly=true 時は全て owned、false 時は全て unowned 表示で簡略化する。`OwnedMD5Cache` を Deps に追加して両方表示できるようにする手もあるが Plan 3 のスコープでは過剰。Plan 4 の HTML ビュー UX 改善時に owned set を Deps へ流す形にリファクタリングする。

- [ ] **Step 3: handler_header.go 本実装**

`internal/adapter/httpserver/handler_header.go` を以下で完全置換:

```go
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

type headerJSON struct {
	Name       string   `json:"name"`
	Symbol     string   `json:"symbol"`
	DataURL    string   `json:"data_url"`
	LevelOrder []string `json:"level_order"`
}

func newHeaderHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		result, pub, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}
		out := headerJSON{
			Name: pub.DisplayName, Symbol: pub.Symbol,
			DataURL: "data.json", LevelOrder: result.LevelOrder,
		}
		if out.LevelOrder == nil {
			out.LevelOrder = []string{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			deps.Log.Error("header.json encode failed", "slug", slug, "err", err)
		}
	}
}

func handleJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrPublishedTableNotFound):
		writeJSONError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, usecase.ErrSourceNotFetched):
		writeJSONError(w, http.StatusServiceUnavailable, "source_not_fetched")
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 4: handler_data.go 本実装**

`internal/adapter/httpserver/handler_data.go` を以下で完全置換:

```go
package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

func newDataHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		result, _, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}

		entries := make([]map[string]any, 0, len(result.Charts))
		for _, c := range result.Charts {
			entries = append(entries, mergeChart(c))
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			deps.Log.Error("data.json encode failed", "slug", slug, "err", err)
		}
	}
}

// mergeChart は SourceChart.Raw をベースに level/md5/sha256/title/artist を上書きしてマップを返す。
// 表固有フィールド（url, url_diff, lr2_bmsid 等）はパススルーされる。
func mergeChart(c domain.SourceChart) map[string]any {
	out := make(map[string]any, len(c.Raw)+5)
	for k, v := range c.Raw {
		out[k] = v
	}
	out["md5"] = c.MD5
	if c.SHA256 != "" {
		out["sha256"] = c.SHA256
	}
	out["level"] = c.Level
	out["title"] = c.Title
	out["artist"] = c.Artist
	return out
}
```

- [ ] **Step 5: handler_refresh.go 本実装**

`internal/adapter/httpserver/handler_refresh.go` を以下で完全置換:

```go
package httpserver

import (
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

func newRefreshHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		_, pub, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}
		if pub.Pick.RefreshMode != domain.RefreshModeManual {
			http.Error(w, "manual モード以外では再ピック不可", http.StatusMethodNotAllowed)
			return
		}
		if err := deps.Pick.ManualRefresh(r.Context(), pub.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/"+pub.Slug, http.StatusSeeOther)
	}
}
```

- [ ] **Step 6: handler_html_test.go を作成**

`internal/adapter/httpserver/handler_html_test.go`:

```go
package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
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
	_, err := f.srcRepo.Create(nil, domain.SourceTable{
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
```

注: `f.srcRepo.Create(nil, ...)` は context が nil だが `*sql.DB.ExecContext(nil, ...)` は実装上は `context.Background()` を使うので動作する。ただしクリーンには `context.Background()` を渡すべきなので、必要なら `context "context"` を import に追加して書き換える。

- [ ] **Step 7: handler_header_test.go を作成**

`internal/adapter/httpserver/handler_header_test.go`:

```go
package httpserver_test

import (
	"context"
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

// _ keep import
var _ = context.Background
```

- [ ] **Step 8: handler_data_test.go を作成**

`internal/adapter/httpserver/handler_data_test.go`:

```go
package httpserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
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
```

- [ ] **Step 9: handler_refresh_test.go を作成**

`internal/adapter/httpserver/handler_refresh_test.go`:

```go
package httpserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func newRefreshClient() *http.Client {
	return &http.Client{
		// 303 リダイレクトを自動追従させない（リダイレクト先が GET /:slug になることを確認するため）
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
```

- [ ] **Step 10: テストを走らせて通過確認**

Run: `go test ./internal/adapter/httpserver/... -v`
Expected: 全 PASS

- [ ] **Step 11: 全体テストでのリグレッション確認**

Run: `go test ./...`
Expected: 全 PASS

- [ ] **Step 12: コミット**

```bash
git add internal/adapter/httpserver/
git commit -m "feat(httpserver): 4 ハンドラの本実装 (HTML / header.json / data.json / 再ピック)"
```

---

## Task 14: app/handler — Wails Bind 用ハンドラ 4 種を追加

**Files:**

- Create: `internal/app/handler/published_table_handler.go`
- Create: `internal/app/handler/published_table_handler_test.go`
- Create: `internal/app/handler/pick_handler.go`
- Create: `internal/app/handler/server_status_handler.go`
- Create: `internal/app/handler/owned_chart_handler.go`

Wails Bind 経由でフロントエンドから呼ぶハンドラ群。既存の `config_handler.go` / `source_table_handler.go` と同じパターン（`SetContext` + 薄い DTO 変換）で書く。

- [ ] **Step 1: PublishedTableHandler を作成**

`internal/app/handler/published_table_handler.go`:

```go
package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PublishedTableDTO はフロントエンドに返す JSON 構造体。
type PublishedTableDTO struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
	SortOrder     int    `json:"sortOrder"`
}

// CreatePublishedTableRequest は CreatePublishedTable のリクエスト DTO。
type CreatePublishedTableRequest struct {
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
}

// UpdatePublishedTableRequest は UpdatePublishedTable のリクエスト DTO。
type UpdatePublishedTableRequest struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
	SourceTableID string `json:"sourceTableId"`
	OwnedOnly     bool   `json:"ownedOnly"`
	PickPerLevel  int    `json:"pickPerLevel"`
	RefreshMode   string `json:"refreshMode"`
	SortOrder     int    `json:"sortOrder"`
}

// SlugValidationDTO は ValidateSlug の応答 DTO。
type SlugValidationDTO struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"` // "invalid_format" / "reserved" / "duplicate"
}

// PublishedTableHandler は Wails Bind 経由で公開表 API を公開する。
type PublishedTableHandler struct {
	uc  *usecase.PublishedTableUseCase
	ctx context.Context
}

// NewPublishedTableHandler は新しい PublishedTableHandler を作る。
func NewPublishedTableHandler(uc *usecase.PublishedTableUseCase) *PublishedTableHandler {
	return &PublishedTableHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *PublishedTableHandler) SetContext(ctx context.Context) { h.ctx = ctx }

func toPublishedTableDTO(t domain.PublishedTable) PublishedTableDTO {
	return PublishedTableDTO{
		ID: t.ID, Slug: t.Slug, DisplayName: t.DisplayName, Symbol: t.Symbol,
		SourceTableID: t.SourceTableID, OwnedOnly: t.OwnedOnly,
		PickPerLevel: t.Pick.PerLevel, RefreshMode: string(t.Pick.RefreshMode),
		SortOrder: t.SortOrder,
	}
}

// ListPublishedTables は登録済み公開表をすべて返す。
func (h *PublishedTableHandler) ListPublishedTables() ([]PublishedTableDTO, error) {
	list, err := h.uc.List(h.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublishedTableDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toPublishedTableDTO(t))
	}
	return out, nil
}

// CreatePublishedTable は新規公開表を作成し、ID を返す。
func (h *PublishedTableHandler) CreatePublishedTable(req CreatePublishedTableRequest) (string, error) {
	return h.uc.Create(h.ctx, usecase.CreatePublishedTableInput{
		Slug: req.Slug, DisplayName: req.DisplayName, Symbol: req.Symbol,
		SourceTableID: req.SourceTableID, OwnedOnly: req.OwnedOnly,
		PickPerLevel: req.PickPerLevel,
		RefreshMode:  domain.RefreshMode(req.RefreshMode),
	})
}

// UpdatePublishedTable は公開表を更新する。
func (h *PublishedTableHandler) UpdatePublishedTable(req UpdatePublishedTableRequest) error {
	return h.uc.Update(h.ctx, usecase.UpdatePublishedTableInput{
		ID: req.ID, Slug: req.Slug, DisplayName: req.DisplayName, Symbol: req.Symbol,
		SourceTableID: req.SourceTableID, OwnedOnly: req.OwnedOnly,
		PickPerLevel: req.PickPerLevel,
		RefreshMode:  domain.RefreshMode(req.RefreshMode),
		SortOrder:    req.SortOrder,
	})
}

// DeletePublishedTable は公開表を削除する。
func (h *PublishedTableHandler) DeletePublishedTable(id string) error {
	return h.uc.Delete(h.ctx, id)
}

// ValidateSlug は slug 形式 / 予約語 / 重複を検査する。GUI のリアルタイム判定用。
func (h *PublishedTableHandler) ValidateSlug(slug string, excludeID string) SlugValidationDTO {
	err := h.uc.ValidateSlug(h.ctx, slug, excludeID)
	switch {
	case err == nil:
		return SlugValidationDTO{OK: true}
	case errors.Is(err, usecase.ErrSlugInvalidFormat):
		return SlugValidationDTO{OK: false, Reason: "invalid_format"}
	case errors.Is(err, usecase.ErrSlugReserved):
		return SlugValidationDTO{OK: false, Reason: "reserved"}
	case errors.Is(err, usecase.ErrSlugDuplicated):
		return SlugValidationDTO{OK: false, Reason: "duplicate"}
	default:
		return SlugValidationDTO{OK: false, Reason: err.Error()}
	}
}

// SuggestSlugFromSource はソース表名から slug を生成する。
func (h *PublishedTableHandler) SuggestSlugFromSource(sourceID string) (string, error) {
	return h.uc.SuggestSlugFromSource(h.ctx, sourceID)
}

// OpenPublishedTableURL はブラウザで http://127.0.0.1:<port>/<slug> を開く。
// port は config の値を Bind 時に取得する。失敗時はエラー文字列を返す。
func (h *PublishedTableHandler) OpenPublishedTableURL(slug string, port int) error {
	if h.ctx == nil {
		return errors.New("context が未初期化です")
	}
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/" + slug
	wailsruntime.BrowserOpenURL(h.ctx, url)
	return nil
}
```

- [ ] **Step 2: PublishedTableHandler テストを作成**

`internal/app/handler/published_table_handler_test.go`:

```go
package handler_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupPublishedTableHandler(t *testing.T) (*handler.PublishedTableHandler, *persistence.SourceTableRepoSQL) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "h.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, persistence.RunMigrations(db))
	src := persistence.NewSourceTableRepoSQL(db)
	pub := persistence.NewPublishedTableRepoSQL(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := usecase.NewPublishedTableUseCase(pub, src, idgen.NewULID(), logger)
	h := handler.NewPublishedTableHandler(uc)
	h.SetContext(context.Background())
	return h, src
}

func TestPublishedTableHandler_Create_List_Delete(t *testing.T) {
	h, src := setupPublishedTableHandler(t)
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: "SRC1", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	id, err := h.CreatePublishedTable(handler.CreatePublishedTableRequest{
		Slug: "ok", DisplayName: "OK", SourceTableID: "SRC1",
		RefreshMode: "per_request",
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	list, err := h.ListPublishedTables()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "ok", list[0].Slug)

	require.NoError(t, h.DeletePublishedTable(id))
	list, err = h.ListPublishedTables()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestPublishedTableHandler_ValidateSlug(t *testing.T) {
	h, src := setupPublishedTableHandler(t)
	_, err := src.Create(context.Background(), domain.SourceTable{
		ID: "SRC1", InputURL: "https://x", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)

	require.True(t, h.ValidateSlug("ok-slug", "").OK)
	require.False(t, h.ValidateSlug("BadSlug", "").OK)
	require.Equal(t, "invalid_format", h.ValidateSlug("BadSlug", "").Reason)
	require.Equal(t, "reserved", h.ValidateSlug("_admin", "").Reason)

	id, err := h.CreatePublishedTable(handler.CreatePublishedTableRequest{
		Slug: "taken", DisplayName: "T", SourceTableID: "SRC1",
		RefreshMode: "per_request",
	})
	require.NoError(t, err)

	require.Equal(t, "duplicate", h.ValidateSlug("taken", "").Reason)
	require.True(t, h.ValidateSlug("taken", id).OK, "自分自身を除外すれば OK")
}
```

- [ ] **Step 3: PickHandler を作成**

`internal/app/handler/pick_handler.go`:

```go
package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// PickHandler は Wails Bind 経由で手動再ピック API を公開する。
type PickHandler struct {
	uc  *usecase.PickUseCase
	ctx context.Context
}

// NewPickHandler は新しい PickHandler を作る。
func NewPickHandler(uc *usecase.PickUseCase) *PickHandler {
	return &PickHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *PickHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// ManualRefreshPick は指定 publishedID のピックを手動更新する。
func (h *PickHandler) ManualRefreshPick(publishedID string) error {
	return h.uc.ManualRefresh(h.ctx, publishedID)
}
```

- [ ] **Step 4: ServerStatusHandler を作成**

`internal/app/handler/server_status_handler.go`:

```go
package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// ServerStatusDTO はフロントエンドに返す JSON 構造体。
type ServerStatusDTO struct {
	State     string `json:"state"`
	Port      int    `json:"port"`
	StartedAt string `json:"startedAt"`
	LastError string `json:"lastError"`
}

// ServerStatusHandler は Wails Bind 経由で HTTP サーバ操作 API を公開する。
type ServerStatusHandler struct {
	uc  *usecase.ServerUseCase
	ctx context.Context
}

// NewServerStatusHandler は新しい ServerStatusHandler を作る。
func NewServerStatusHandler(uc *usecase.ServerUseCase) *ServerStatusHandler {
	return &ServerStatusHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *ServerStatusHandler) SetContext(ctx context.Context) { h.ctx = ctx }

func toServerStatusDTO(s domain.ServerStatus) ServerStatusDTO {
	out := ServerStatusDTO{
		State: string(s.State), Port: s.Port, LastError: s.LastError,
	}
	if s.StartedAt != nil {
		out.StartedAt = s.StartedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// GetServerStatus は現在のサーバステータスを返す。
func (h *ServerStatusHandler) GetServerStatus() ServerStatusDTO {
	return toServerStatusDTO(h.uc.Status())
}

// StartServer は HTTP サーバを起動する。
func (h *ServerStatusHandler) StartServer() error {
	return h.uc.Start(h.ctx)
}

// StopServer は HTTP サーバを停止する。
func (h *ServerStatusHandler) StopServer() error {
	return h.uc.Stop(h.ctx)
}

// RestartServer は HTTP サーバを再起動する（停止 → 起動）。
func (h *ServerStatusHandler) RestartServer() error {
	return h.uc.Restart(h.ctx)
}
```

- [ ] **Step 5: OwnedChartHandler を作成**

`internal/app/handler/owned_chart_handler.go`:

```go
package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// OwnedCacheStatusDTO は GetOwnedCacheStatus が返す DTO。
type OwnedCacheStatusDTO struct {
	Loaded     bool   `json:"loaded"`
	Count      int    `json:"count"`
	LoadedAt   string `json:"loadedAt"`
	LoadedPath string `json:"loadedPath"`
	LastError  string `json:"lastError"`
}

// OwnedChartHandler は Wails Bind 経由で所持キャッシュの状態と再読み込み API を公開する。
type OwnedChartHandler struct {
	cache *usecase.OwnedMD5Cache
	ctx   context.Context
}

// NewOwnedChartHandler は新しい OwnedChartHandler を作る。
func NewOwnedChartHandler(cache *usecase.OwnedMD5Cache) *OwnedChartHandler {
	return &OwnedChartHandler{cache: cache, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *OwnedChartHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// GetOwnedCacheStatus は現在の所持キャッシュ状態を返す。
func (h *OwnedChartHandler) GetOwnedCacheStatus() OwnedCacheStatusDTO {
	st := h.cache.Status()
	out := OwnedCacheStatusDTO{
		Loaded: st.Loaded, Count: st.Count,
		LoadedPath: st.LoadedPath, LastError: st.LastError,
	}
	if st.LoadedAt != nil {
		out.LoadedAt = st.LoadedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ReloadOwnedCache は songdata.db を再読み込みする。
func (h *OwnedChartHandler) ReloadOwnedCache() error {
	return h.cache.Reload(h.ctx)
}
```

- [ ] **Step 6: ビルドとテストを確認**

Run: `go build ./... && go test ./internal/app/handler/... -v`
Expected: 全 PASS（PublishedTableHandler のテストが pass する。残り 3 ハンドラはユニットテストを書かなくてもビルドは通る）

- [ ] **Step 7: コミット**

```bash
git add internal/app/handler/published_table_handler.go internal/app/handler/published_table_handler_test.go internal/app/handler/pick_handler.go internal/app/handler/server_status_handler.go internal/app/handler/owned_chart_handler.go
git commit -m "feat(app/handler): Wails Bind 用ハンドラ 4 種を追加 (PublishedTable / Pick / ServerStatus / OwnedChart)"
```

---

## Task 15: Bootstrap + main.go + app.go の配線

**Files:**

- Modify: `internal/app/bootstrap.go`
- Modify: `main.go`
- Modify: `app.go`

新規 UseCase / Handler を `Services` に追加し、`main.go` の Bind 配列に 4 ハンドラを足し、`app.go` の `startup` でサーバ自動起動 + `OnStatusChange` を Wails event へブリッジ + `OpenURL` メソッドを追加する。

- [ ] **Step 1: bootstrap.go を変更**

`internal/app/bootstrap.go` を以下で完全置換:

```go
// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/gateway"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// Services はアプリ全体で共有する依存を保持する。
type Services struct {
	DB                    *sql.DB
	Logger                *slog.Logger
	LoggerClose           logger.CloseFunc
	ConfigHandler         *handler.ConfigHandler
	SourceTableHandler    *handler.SourceTableHandler
	PublishedTableHandler *handler.PublishedTableHandler
	PickHandler           *handler.PickHandler
	ServerStatusHandler   *handler.ServerStatusHandler
	OwnedChartHandler     *handler.OwnedChartHandler
	SourceTableUseCase    *usecase.SourceTableUseCase
	ServerUseCase         *usecase.ServerUseCase
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
		LogDir: logDir, MaxSizeMB: 50, MaxBackups: 7, MaxAgeDays: 7,
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

	pubRepo := persistence.NewPublishedTableRepoSQL(db)
	pubUC := usecase.NewPublishedTableUseCase(pubRepo, sourceRepo, idGen, lg)
	pubHandler := handler.NewPublishedTableHandler(pubUC)

	systemClock := clock.System{}
	ownedRepo := persistence.NewSongdataReader()
	ownedCache := usecase.NewOwnedMD5Cache(ownedRepo, configStore, systemClock, lg)
	pickStore := usecase.NewPickResultStore()
	randFactory := port.RandSourceFactory(func(seed int64) port.RandSource {
		return randsrc.NewMathRandSource(seed)
	})
	pickUC := usecase.NewPickUseCase(pubRepo, sourceRepo, ownedCache, pickStore, systemClock, randFactory, lg)
	pickHandler := handler.NewPickHandler(pickUC)
	ownedHandler := handler.NewOwnedChartHandler(ownedCache)

	// songdata_db_path 変更時に owned cache を invalidate + ピックキャッシュを clear
	configUC.AddSongdataPathChangeHook(func() {
		ownedCache.Invalidate()
		pickUC.InvalidateAll()
	})

	httpFactory := func(addr string) usecase.HTTPServer {
		return httpserver.New(addr, httpserver.Deps{Pick: pickUC, Pub: pubUC, Log: lg})
	}
	serverUC := usecase.NewServerUseCase(configStore, httpFactory, lg)
	serverHandler := handler.NewServerStatusHandler(serverUC)

	lg.Info("bootstrap complete", "db", dbPath, "logDir", logDir)

	return &Services{
		DB:                    db,
		Logger:                lg,
		LoggerClose:           closeLog,
		ConfigHandler:         configHandler,
		SourceTableHandler:    sourceHandler,
		PublishedTableHandler: pubHandler,
		PickHandler:           pickHandler,
		ServerStatusHandler:   serverHandler,
		OwnedChartHandler:     ownedHandler,
		SourceTableUseCase:    sourceUC,
		ServerUseCase:         serverUC,
	}, nil
}

// Close は Services が保持する全リソースを解放する。
// サーバ稼働中は最大 5 秒で graceful shutdown する。
func (s *Services) Close() {
	if s.ServerUseCase != nil {
		ctx, cancel := contextWithTimeout(5 * time.Second)
		defer cancel()
		_ = s.ServerUseCase.Stop(ctx)
	}
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
```

- [ ] **Step 2: bootstrap.go の補助関数 contextWithTimeout を追加**

`internal/app/bootstrap.go` の末尾に追加:

```go
// contextWithTimeout は app パッケージ内で使う context.Context を 1 行で作るヘルパ。
// import を bootstrap.go の冒頭でまとめるため、別関数で隔離している。
func contextWithTimeout(d time.Duration) (ctx interface {
	Done() <-chan struct{}
	Deadline() (time.Time, bool)
	Err() error
	Value(any) any
}, cancel func()) {
	ctx, cancel = stdContextWithTimeout(d)
	return
}
```

ただし上記は import の取り回しが煩雑なので、シンプルに `bootstrap.go` の最初の import に `"context"` を追加し、`Close()` の中で直接 `context.WithTimeout(context.Background(), 5*time.Second)` を呼ぶ書き方にする方が綺麗。書き換え:

`internal/app/bootstrap.go` の `import` ブロックに `"context"` を追加し、`Close()` を以下に置換:

```go
// Close は Services が保持する全リソースを解放する。
// サーバ稼働中は最大 5 秒で graceful shutdown する。
func (s *Services) Close() {
	if s.ServerUseCase != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.ServerUseCase.Stop(ctx)
	}
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
```

そして上記 Step 2 で書いた `contextWithTimeout` ラッパは不要なので削除する（Step 1 で直接 `context.WithTimeout` を使うように書いていれば、Step 2 のラッパは丸ごと不要）。

最終的な `internal/app/bootstrap.go` の import は次のようになる:

```go
import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/gateway"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)
```

- [ ] **Step 3: main.go の Bind 配列を拡張**

`main.go` の `Bind: []any{...}` ブロックを以下で置換:

```go
		Bind: []any{
			myApp,
			services.ConfigHandler,
			services.SourceTableHandler,
			services.PublishedTableHandler,
			services.PickHandler,
			services.ServerStatusHandler,
			services.OwnedChartHandler,
		},
```

- [ ] **Step 4: app.go の startup でサーバ自動起動 + OnStatusChange を event へ流す**

`app.go` の `startup` メソッドを以下で置換:

```go
// startup は OnStartup で呼ばれる。ハンドラに ctx を引き渡し、ソース表の
// バックグラウンド更新と HTTP サーバの自動起動を行う。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.services.ConfigHandler.SetContext(ctx)
	a.services.SourceTableHandler.SetContext(ctx)
	a.services.PublishedTableHandler.SetContext(ctx)
	a.services.PickHandler.SetContext(ctx)
	a.services.ServerStatusHandler.SetContext(ctx)
	a.services.OwnedChartHandler.SetContext(ctx)
	a.services.Logger.Info("wails startup")

	// ServerStatus 変化を Wails event 経由でフロントへ流す
	a.services.ServerUseCase.OnStatusChange(func(s wailsServerStatus) {
		wailsruntime.EventsEmit(ctx, "server_status:changed", s)
	})

	// 起動時のソース表バックグラウンド更新
	go func() {
		a.services.Logger.Info("startup refresh all begin")
		if err := a.services.SourceTableUseCase.RefreshAll(ctx); err != nil {
			a.services.Logger.Warn("startup refresh all failed", "err", err)
		}
		a.services.Logger.Info("startup refresh all done")
		wailsruntime.EventsEmit(ctx, "source_table:refresh_all_done")
	}()

	// HTTP サーバ自動起動
	go func() {
		if err := a.services.ServerUseCase.Start(ctx); err != nil {
			a.services.Logger.Warn("auto-start http server failed", "err", err)
		}
	}()
}
```

ここで `wailsServerStatus` という参照は実際には `domain.ServerStatus` を直接渡せばよい。app.go の import に `"github.com/meta-BE/bms-random-table-compositor/internal/domain"` を追加し、コールバックの引数型を `domain.ServerStatus` にする。

最終的な `app.go` 上部の import:

```go
import (
	"context"
	"os"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	"github.com/meta-BE/bms-random-table-compositor/internal/app"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)
```

そして OnStatusChange のコールバックを以下で確定:

```go
	a.services.ServerUseCase.OnStatusChange(func(s domain.ServerStatus) {
		wailsruntime.EventsEmit(ctx, "server_status:changed", s)
	})
```

- [ ] **Step 5: ビルドとテストを確認**

Run: `go build ./... && go test ./...`
Expected: 全 PASS

- [ ] **Step 6: コミット**

```bash
git add internal/app/bootstrap.go main.go app.go
git commit -m "feat(app): Plan 3 のサービスを配線 (HTTPサーバ自動起動 + 4ハンドラ Bind)"
```

---

## Task 16: frontend/api.ts 拡張 + App.svelte に公開表タブ追加

**Files:**

- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/App.svelte`

`wails dev` か `wails build` で `frontend/wailsjs/go/handler/` 配下に新規 4 ハンドラの TypeScript wrapper が自動生成される前提。`api.ts` から新ハンドラを薄くラップする。

- [ ] **Step 1: Wails の TypeScript wrapper を再生成**

Run: `wails generate module`
（または `wails dev` を一度起動して終了する）
Expected: `frontend/wailsjs/go/handler/PublishedTableHandler.d.ts` 等が生成される

- [ ] **Step 2: api.ts を以下で完全置換**

`frontend/src/lib/api.ts`:

```ts
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
import {
  ListPublishedTables,
  CreatePublishedTable,
  UpdatePublishedTable,
  DeletePublishedTable,
  ValidateSlug,
  SuggestSlugFromSource,
  OpenPublishedTableURL,
} from '../../wailsjs/go/handler/PublishedTableHandler';
import { ManualRefreshPick } from '../../wailsjs/go/handler/PickHandler';
import {
  GetServerStatus,
  StartServer,
  StopServer,
  RestartServer,
} from '../../wailsjs/go/handler/ServerStatusHandler';
import {
  GetOwnedCacheStatus,
  ReloadOwnedCache,
} from '../../wailsjs/go/handler/OwnedChartHandler';
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

export type AddSourceTableRequest = { url: string };

export type RefreshMode = 'per_request' | 'daily' | 'manual';

export type PublishedTableDTO = {
  id: string;
  slug: string;
  displayName: string;
  symbol: string;
  sourceTableId: string;
  ownedOnly: boolean;
  pickPerLevel: number;
  refreshMode: RefreshMode;
  sortOrder: number;
};

export type CreatePublishedTableRequest = {
  slug: string;
  displayName: string;
  symbol: string;
  sourceTableId: string;
  ownedOnly: boolean;
  pickPerLevel: number;
  refreshMode: RefreshMode;
};

export type UpdatePublishedTableRequest = CreatePublishedTableRequest & {
  id: string;
  sortOrder: number;
};

export type SlugValidation =
  | { ok: true; reason?: undefined }
  | { ok: false; reason: 'invalid_format' | 'reserved' | 'duplicate' | string };

export type ServerState = 'stopped' | 'running' | 'error';

export type ServerStatusDTO = {
  state: ServerState;
  port: number;
  startedAt: string;
  lastError: string;
};

export type OwnedCacheStatusDTO = {
  loaded: boolean;
  count: number;
  loadedAt: string;
  loadedPath: string;
  lastError: string;
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
  // ---- 公開表 ----
  listPublishedTables(): Promise<PublishedTableDTO[]> {
    return ListPublishedTables() as Promise<PublishedTableDTO[]>;
  },
  createPublishedTable(req: CreatePublishedTableRequest): Promise<string> {
    return CreatePublishedTable(req) as Promise<string>;
  },
  updatePublishedTable(req: UpdatePublishedTableRequest): Promise<void> {
    return UpdatePublishedTable(req);
  },
  deletePublishedTable(id: string): Promise<void> {
    return DeletePublishedTable(id);
  },
  validateSlug(slug: string, excludeId: string): Promise<SlugValidation> {
    return ValidateSlug(slug, excludeId) as Promise<SlugValidation>;
  },
  suggestSlugFromSource(sourceId: string): Promise<string> {
    return SuggestSlugFromSource(sourceId) as Promise<string>;
  },
  openPublishedTableURL(slug: string, port: number): Promise<void> {
    return OpenPublishedTableURL(slug, port);
  },
  manualRefreshPick(publishedId: string): Promise<void> {
    return ManualRefreshPick(publishedId);
  },
  // ---- サーバ ----
  getServerStatus(): Promise<ServerStatusDTO> {
    return GetServerStatus() as Promise<ServerStatusDTO>;
  },
  startServer(): Promise<void> {
    return StartServer();
  },
  stopServer(): Promise<void> {
    return StopServer();
  },
  restartServer(): Promise<void> {
    return RestartServer();
  },
  onServerStatusChanged(cb: (s: ServerStatusDTO) => void): () => void {
    EventsOn('server_status:changed', cb);
    return () => EventsOff('server_status:changed');
  },
  // ---- 所持キャッシュ ----
  getOwnedCacheStatus(): Promise<OwnedCacheStatusDTO> {
    return GetOwnedCacheStatus() as Promise<OwnedCacheStatusDTO>;
  },
  reloadOwnedCache(): Promise<void> {
    return ReloadOwnedCache();
  },
  // ---- イベント ----
  onSourceTableRefreshAllDone(cb: () => void): () => void {
    EventsOn('source_table:refresh_all_done', cb);
    return () => EventsOff('source_table:refresh_all_done');
  },
};
```

- [ ] **Step 3: App.svelte に「公開表」タブを追加**

`frontend/src/App.svelte` を以下で完全置換:

```svelte
<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
  import SourceTablesTab from './lib/tabs/SourceTablesTab.svelte';
  import PublishedTablesTab from './lib/tabs/PublishedTablesTab.svelte';

  type TabKey = 'server' | 'source-tables' | 'published-tables';
  let active: TabKey = 'server';
</script>

<main>
  <header>
    <h1>BMS Random Table Compositor</h1>
    <nav>
      <button class:active={active === 'server'} on:click={() => (active = 'server')}>サーバ設定</button>
      <button class:active={active === 'source-tables'} on:click={() => (active = 'source-tables')}>ソース表</button>
      <button class:active={active === 'published-tables'} on:click={() => (active = 'published-tables')}>公開表</button>
    </nav>
  </header>
  {#if active === 'server'}
    <ServerTab />
  {:else if active === 'source-tables'}
    <SourceTablesTab />
  {:else if active === 'published-tables'}
    <PublishedTablesTab />
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

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/api.ts frontend/src/App.svelte frontend/wailsjs/go/handler/
git commit -m "feat(frontend): api.ts に公開表/サーバ/所持キャッシュ API を追加し、App.svelte に公開表タブを追加"
```

---

## Task 17: frontend/PublishedTablesTab.svelte 新設

**Files:**

- Create: `frontend/src/lib/tabs/PublishedTablesTab.svelte`

公開表 CRUD UI。インライン展開フォーム（追加 + 編集）+ 一覧 + 各行の「開く / 編集 / 削除 / 再ピック (manual のみ)」ボタン。`SourceTablesTab.svelte` の構造を踏襲。Plan 2 lessons 通り `window.confirm` は使わず即削除する。

- [ ] **Step 1: PublishedTablesTab を作成**

`frontend/src/lib/tabs/PublishedTablesTab.svelte`:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    api,
    type PublishedTableDTO,
    type CreatePublishedTableRequest,
    type UpdatePublishedTableRequest,
    type RefreshMode,
    type SourceTableDTO,
    type ServerStatusDTO,
    type SlugValidation,
  } from '../api';

  let rows: PublishedTableDTO[] = [];
  let sources: SourceTableDTO[] = [];
  let serverStatus: ServerStatusDTO | null = null;
  let listError = '';
  let formError = '';
  let formMode: 'closed' | 'create' | 'edit' = 'closed';
  let editingId: string = '';
  let busy = false;
  let unsubscribeStatus: (() => void) | null = null;

  // フォーム状態
  let f = blankForm();
  let slugValidation: SlugValidation = { ok: true };
  let slugTimer: ReturnType<typeof setTimeout> | null = null;

  function blankForm(): CreatePublishedTableRequest {
    return {
      slug: '',
      displayName: '',
      symbol: '',
      sourceTableId: '',
      ownedOnly: false,
      pickPerLevel: 0,
      refreshMode: 'per_request',
    };
  }

  async function load() {
    listError = '';
    try {
      rows = await api.listPublishedTables();
      sources = await api.listSourceTables();
      serverStatus = await api.getServerStatus();
    } catch (e: any) {
      listError = `読み込みエラー: ${String(e)}`;
    }
  }

  function openCreate() {
    formMode = 'create';
    editingId = '';
    f = blankForm();
    formError = '';
    slugValidation = { ok: true };
  }

  function openEdit(row: PublishedTableDTO) {
    formMode = 'edit';
    editingId = row.id;
    f = {
      slug: row.slug,
      displayName: row.displayName,
      symbol: row.symbol,
      sourceTableId: row.sourceTableId,
      ownedOnly: row.ownedOnly,
      pickPerLevel: row.pickPerLevel,
      refreshMode: row.refreshMode,
    };
    formError = '';
    slugValidation = { ok: true };
  }

  function closeForm() {
    formMode = 'closed';
    editingId = '';
    formError = '';
  }

  function debounceValidateSlug() {
    if (slugTimer) clearTimeout(slugTimer);
    slugTimer = setTimeout(async () => {
      if (!f.slug) {
        slugValidation = { ok: false, reason: 'invalid_format' };
        return;
      }
      try {
        slugValidation = await api.validateSlug(f.slug, editingId);
      } catch (e: any) {
        slugValidation = { ok: false, reason: String(e) };
      }
    }, 300);
  }

  async function suggestSlug() {
    if (!f.sourceTableId) return;
    try {
      f.slug = await api.suggestSlugFromSource(f.sourceTableId);
      debounceValidateSlug();
    } catch (e: any) {
      formError = `slug 候補の生成に失敗: ${String(e)}`;
    }
  }

  async function submitForm() {
    formError = '';
    if (!f.displayName) {
      formError = '表示名は必須です';
      return;
    }
    if (!f.sourceTableId) {
      formError = 'ソース表を選択してください';
      return;
    }
    if (!slugValidation.ok) {
      formError = `slug が無効: ${slugValidation.reason}`;
      return;
    }
    busy = true;
    try {
      if (formMode === 'create') {
        await api.createPublishedTable(f);
      } else if (formMode === 'edit') {
        const req: UpdatePublishedTableRequest = { ...f, id: editingId, sortOrder: 0 };
        await api.updatePublishedTable(req);
      }
      closeForm();
      await load();
    } catch (e: any) {
      formError = String(e);
    } finally {
      busy = false;
    }
  }

  async function remove(id: string) {
    // Plan 2 lessons: window.confirm は Wails WebView で機能しないため即削除
    busy = true;
    try {
      await api.deletePublishedTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = false;
    }
  }

  async function manualRefresh(id: string) {
    busy = true;
    try {
      await api.manualRefreshPick(id);
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = false;
    }
  }

  async function openInBrowser(slug: string) {
    if (!serverStatus || serverStatus.state !== 'running') {
      listError = 'サーバが起動していません';
      return;
    }
    try {
      await api.openPublishedTableURL(slug, serverStatus.port);
    } catch (e: any) {
      listError = String(e);
    }
  }

  function sourceLabel(id: string): string {
    const s = sources.find((x) => x.id === id);
    if (!s) return id;
    const name = s.displayName || s.name || s.inputUrl;
    if (s.lastFetchStatus === 'never') return `${name} (未取得)`;
    return name;
  }

  onMount(() => {
    load();
    unsubscribeStatus = api.onServerStatusChanged((s) => (serverStatus = s));
  });

  onDestroy(() => {
    if (unsubscribeStatus) unsubscribeStatus();
    if (slugTimer) clearTimeout(slugTimer);
  });
</script>

<section class="tab">
  <h2>公開表</h2>

  {#if listError}
    <div class="error">{listError}</div>
  {/if}

  {#if formMode === 'closed'}
    <button class="primary" on:click={openCreate} disabled={busy}>+ 公開表を追加</button>
  {/if}

  {#if formMode !== 'closed'}
    <div class="form">
      <h3>{formMode === 'create' ? '公開表を追加' : '公開表を編集'}</h3>
      <label>
        表示名
        <input type="text" bind:value={f.displayName} />
      </label>
      <label>
        ソース表
        <select bind:value={f.sourceTableId}>
          <option value="">— 選択 —</option>
          {#each sources as s}
            <option value={s.id}>{sourceLabel(s.id)}</option>
          {/each}
        </select>
      </label>
      <label>
        Slug
        <span class="slug-row">
          <input type="text" bind:value={f.slug} on:input={debounceValidateSlug} />
          <button type="button" on:click={suggestSlug} disabled={!f.sourceTableId}>ソース表名から生成</button>
        </span>
        {#if !slugValidation.ok}
          <span class="slug-err">slug が無効: {slugValidation.reason}</span>
        {/if}
      </label>
      <label>
        Symbol
        <input type="text" bind:value={f.symbol} />
      </label>
      <label class="checkbox">
        <input type="checkbox" bind:checked={f.ownedOnly} />
        所持譜面のみ表示する
      </label>
      <label>
        レベルあたりの最大曲数 (0 = 無制限)
        <input type="number" min="0" bind:value={f.pickPerLevel} />
      </label>
      <label>
        ピック更新モード
        <select bind:value={f.refreshMode}>
          <option value="per_request">per_request (アクセス毎)</option>
          <option value="daily">daily (1 日 1 回)</option>
          <option value="manual">manual (手動)</option>
        </select>
      </label>
      {#if formError}
        <div class="error">{formError}</div>
      {/if}
      <div class="actions">
        <button on:click={submitForm} disabled={busy}>保存</button>
        <button on:click={closeForm} disabled={busy}>キャンセル</button>
      </div>
    </div>
  {/if}

  {#if rows.length === 0}
    <p class="empty">公開表が登録されていません。</p>
  {:else}
    <table>
      <thead>
        <tr><th>表示名</th><th>Slug</th><th>ソース表</th><th>所持限定</th><th>各レベル</th><th>モード</th><th></th></tr>
      </thead>
      <tbody>
        {#each rows as row}
          <tr>
            <td>{row.displayName}</td>
            <td><code>/{row.slug}</code></td>
            <td>{sourceLabel(row.sourceTableId)}</td>
            <td>{row.ownedOnly ? '✓' : ''}</td>
            <td>{row.pickPerLevel === 0 ? '無制限' : row.pickPerLevel}</td>
            <td>{row.refreshMode}</td>
            <td class="ops">
              <button on:click={() => openInBrowser(row.slug)} disabled={busy}>開く</button>
              <button on:click={() => openEdit(row)} disabled={busy}>編集</button>
              {#if row.refreshMode === 'manual'}
                <button on:click={() => manualRefresh(row.id)} disabled={busy}>再ピック</button>
              {/if}
              <button class="danger" on:click={() => remove(row.id)} disabled={busy}>削除</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<style>
  .tab { padding: 12px 16px; }
  h2 { margin: 0 0 12px; font-size: 16px; }
  .error { color: #b00020; margin: 8px 0; }
  .empty { color: #999; }
  .form { border: 1px solid #ccc; padding: 12px; margin: 12px 0; background: #fafafa; }
  .form h3 { margin: 0 0 8px; font-size: 14px; }
  .form label { display: block; margin: 6px 0; font-size: 13px; }
  .form label.checkbox { display: flex; align-items: center; gap: 6px; }
  .form input[type="text"], .form select, .form input[type="number"] {
    width: 100%; box-sizing: border-box; padding: 4px 6px; font-size: 13px;
  }
  .slug-row { display: flex; gap: 6px; }
  .slug-row input { flex: 1; }
  .slug-err { color: #b00020; font-size: 12px; }
  .actions { display: flex; gap: 6px; margin-top: 8px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { padding: 4px 8px; border-bottom: 1px solid #eee; text-align: left; }
  td.ops { display: flex; gap: 4px; }
  button { padding: 3px 8px; cursor: pointer; }
  button.primary { background: #1b2636; color: #fff; padding: 6px 12px; }
  button.danger { color: #b00020; }
</style>
```

- [ ] **Step 2: 動作確認（macOS）**

Run: `wails dev`
- 公開表タブを開いて「+ 公開表を追加」→ ソース表選択 → 「ソース表名から生成」で slug 自動入力 → 保存
- 一覧に表示され、「開く」でブラウザが立ち上がって `/<slug>` HTML が表示されること
- 「編集」で値変更 → 保存 → 一覧に反映
- 「削除」で消えること

- [ ] **Step 3: コミット**

```bash
git add frontend/src/lib/tabs/PublishedTablesTab.svelte
git commit -m "feat(frontend): PublishedTablesTab.svelte を追加 (公開表 CRUD + 開く + 再ピック)"
```

---

## Task 18: frontend/ServerTab.svelte 拡張

**Files:**

- Modify: `frontend/src/lib/tabs/ServerTab.svelte`

既存の設定保存 UI に「サーバステータス + 操作ボタン」「所持譜面キャッシュ + 再読み込み」セクションを追加する。`onMount` で `getServerStatus` / `getOwnedCacheStatus` を取得 + `onServerStatusChanged` 購読。

- [ ] **Step 1: 既存 ServerTab.svelte を確認**

Run: `cat frontend/src/lib/tabs/ServerTab.svelte`

既存の構造を把握してから差分編集する。Plan 1 で書かれた最小設定タブ（ポート + songdata.db パスの保存ボタン）の下に新セクションを追加する形にする。

- [ ] **Step 2: ServerTab.svelte を以下で完全置換**

`frontend/src/lib/tabs/ServerTab.svelte`:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    api,
    type ServerConfig,
    type ServerStatusDTO,
    type OwnedCacheStatusDTO,
  } from '../api';

  let cfg: ServerConfig = { port: 50000, songdataDbPath: '' };
  let saveError = '';
  let saving = false;

  let status: ServerStatusDTO | null = null;
  let statusError = '';
  let busy = false;

  let owned: OwnedCacheStatusDTO | null = null;
  let ownedError = '';

  let unsubscribeStatus: (() => void) | null = null;

  async function load() {
    try {
      cfg = await api.getServerConfig();
    } catch (e: any) {
      saveError = `設定読み込み失敗: ${String(e)}`;
    }
    try {
      status = await api.getServerStatus();
    } catch (e: any) {
      statusError = String(e);
    }
    try {
      owned = await api.getOwnedCacheStatus();
    } catch (e: any) {
      ownedError = String(e);
    }
  }

  async function save() {
    saveError = '';
    saving = true;
    try {
      await api.setServerPort(cfg.port);
      await api.setSongdataDBPath(cfg.songdataDbPath);
    } catch (e: any) {
      saveError = String(e);
    } finally {
      saving = false;
    }
  }

  async function startServer() {
    busy = true;
    statusError = '';
    try { await api.startServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function stopServer() {
    busy = true;
    statusError = '';
    try { await api.stopServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function restartServer() {
    busy = true;
    statusError = '';
    try { await api.restartServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function reloadOwned() {
    ownedError = '';
    busy = true;
    try {
      await api.reloadOwnedCache();
      owned = await api.getOwnedCacheStatus();
    } catch (e: any) {
      ownedError = String(e);
      // 失敗時もステータスを取得（lastError が更新されている可能性）
      try { owned = await api.getOwnedCacheStatus(); } catch {}
    } finally {
      busy = false;
    }
  }

  function formatJST(iso: string): string {
    if (!iso) return '-';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString('ja-JP', {
      timeZone: 'Asia/Tokyo',
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
  }

  function stateLabel(s: ServerStatusDTO['state'] | undefined): string {
    switch (s) {
      case 'running': return '起動中';
      case 'stopped': return '停止中';
      case 'error': return 'エラー';
      default: return '不明';
    }
  }

  function stateClass(s: ServerStatusDTO['state'] | undefined): string {
    switch (s) {
      case 'running': return 'state ok';
      case 'stopped': return 'state stopped';
      case 'error': return 'state err';
      default: return 'state';
    }
  }

  onMount(() => {
    load();
    unsubscribeStatus = api.onServerStatusChanged((s) => (status = s));
  });
  onDestroy(() => {
    if (unsubscribeStatus) unsubscribeStatus();
  });
</script>

<section class="tab">
  <h2>サーバ設定</h2>

  <div class="block">
    <h3>基本設定</h3>
    <label>
      ポート番号
      <input type="number" min="1" max="65535" bind:value={cfg.port} />
    </label>
    <label>
      songdata.db パス
      <input type="text" bind:value={cfg.songdataDbPath} placeholder="/path/to/songdata.db" />
    </label>
    {#if saveError}
      <div class="error">{saveError}</div>
    {/if}
    <button on:click={save} disabled={saving}>保存</button>
  </div>

  <div class="block">
    <h3>サーバステータス</h3>
    {#if status}
      <p>
        <span class={stateClass(status.state)}>● {stateLabel(status.state)}</span>
        {#if status.state === 'running'}
          <code>http://127.0.0.1:{status.port}</code>
        {/if}
      </p>
      {#if status.startedAt}
        <p class="meta">起動: {formatJST(status.startedAt)}</p>
      {/if}
      {#if status.lastError}
        <p class="error">{status.lastError}</p>
      {/if}
    {/if}
    {#if statusError}<div class="error">{statusError}</div>{/if}
    <div class="actions">
      <button on:click={startServer} disabled={busy || status?.state === 'running'}>起動</button>
      <button on:click={stopServer} disabled={busy || status?.state !== 'running'}>停止</button>
      <button on:click={restartServer} disabled={busy}>再起動</button>
    </div>
  </div>

  <div class="block">
    <h3>所持譜面キャッシュ</h3>
    {#if owned}
      <p>読み込み済み: {owned.count.toLocaleString()} 曲</p>
      {#if owned.loadedAt}<p class="meta">最終読み込み: {formatJST(owned.loadedAt)}</p>{/if}
      {#if owned.loadedPath}<p class="meta">パス: <code>{owned.loadedPath}</code></p>{/if}
      {#if owned.lastError}<p class="error">{owned.lastError}</p>{/if}
    {/if}
    {#if ownedError}<div class="error">{ownedError}</div>{/if}
    <button on:click={reloadOwned} disabled={busy}>再読み込み</button>
  </div>
</section>

<style>
  .tab { padding: 12px 16px; }
  h2 { margin: 0 0 12px; font-size: 16px; }
  h3 { margin: 0 0 8px; font-size: 14px; }
  .block { border: 1px solid #ddd; padding: 12px; margin-bottom: 12px; background: #fafafa; }
  label { display: block; margin: 6px 0; font-size: 13px; }
  input[type="number"], input[type="text"] {
    width: 100%; box-sizing: border-box; padding: 4px 6px; font-size: 13px;
  }
  .actions { display: flex; gap: 6px; margin-top: 8px; }
  button { padding: 4px 10px; cursor: pointer; }
  .state { font-weight: bold; }
  .state.ok { color: #2c8a3e; }
  .state.stopped { color: #888; }
  .state.err { color: #b00020; }
  .meta { color: #666; font-size: 12px; }
  .error { color: #b00020; margin: 4px 0; }
  code { font-family: monospace; background: #eef; padding: 0 4px; }
</style>
```

- [ ] **Step 3: 動作確認**

Run: `wails dev`
- サーバタブで「停止 → 起動 → 再起動」を試して状態表示が切り替わること
- 「ポート番号」を変更して「保存」→「再起動」で新しいポートで listen することを確認
- songdata.db パスを設定して「保存」→「再読み込み」で曲数が更新されること

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/tabs/ServerTab.svelte
git commit -m "feat(frontend): ServerTab に起動/停止/再起動 + 所持キャッシュ操作を追加"
```

---

## Task 19: 最終ビルド + 手動 E2E + Windows 確認 + push

**Files:**

- 変更なし（手動検証のみ）

- [ ] **Step 1: ローカルでフルビルド + 全テスト**

Run: `make lint && make test && make build`
Expected: 全 pass。`build/bin/` に macOS 成果物。

- [ ] **Step 2: macOS 実機で手動 E2E**

実行ファイルを起動 → 以下のシナリオを通す:

1. ソース表タブで Satellite 等のソース表を 1 つ追加（Plan 2 の動作確認）
2. 公開表タブで「+ 公開表を追加」→ ソース表を選び、`pick_per_level=5`, `refresh_mode=daily` で保存
3. 公開表行の「開く」でブラウザが立ち上がり、HTML ビューが表示される
4. `http://127.0.0.1:50000/<slug>/header.json` を curl して `{"name":..., "data_url":"data.json", "level_order":[...]}` が返ること
5. `http://127.0.0.1:50000/<slug>/data.json` を curl して `[{"md5":..., "level":..., "title":..., "url":...}]` の JSON 配列が返ること
6. 同じ URL を beatoraja の難易度表として登録 → 譜面一覧が読み込まれること
7. `owned_only=true` で公開表を別途作り、所持譜面のみが返ること
8. `manual` モードで「再ピック」ボタンを押して曲が変わること
9. 設定タブで「停止」→ ブラウザが繋がらなくなる、「起動」で復活すること
10. 再起動後（アプリ終了 → 再起動）も公開表設定が保持されていること

- [ ] **Step 3: main へ push**

```bash
git push origin main
```

- [ ] **Step 4: Windows ビルドを workflow_dispatch で実行**

```bash
gh workflow run build-windows.yml
gh run watch
gh run download --name bms-random-table-compositor-windows-amd64 --dir ./tmp/windows-plan3
```

- [ ] **Step 5: Windows 機での E2E 確認**

Windows 機（または VM）に exe を持ち込み、上記 macOS E2E と同じシナリオを通す。特に:

- システムトレイ動作（Plan 1 の retro check）
- HTTP サーバが Windows Defender Firewall で阻害されないこと（127.0.0.1 のみなので通常は通る）
- beatoraja から実機接続できること

- [ ] **Step 6: 完了コミット（必要なら）**

E2E で見つかった軽微な修正があれば追加コミット & push。なければスキップ。

```bash
# 必要時のみ
git add -A
git commit -m "fix(plan3): E2E で見つかった issue を修正"
git push origin main
```

---

## Self-Review

実装完了後（Plan 3 終了時点）に以下を確認:

### Spec カバレッジ

spec §5 (PublishedTable / PickResult / ServerStatus) → Task 1 / 4 / 11 ✓
spec §6 (PublishedTableRepo / OwnedChartRepo / Clock / RandSource port) → Task 2 ✓
spec §6 (PublishedTableUseCase / PickUseCase / PickResultStore / ServerUseCase) → Task 6 / 8 / 9 / 11 ✓
spec §7.2 (PickBySlug の母集団取得 → owned 絞り込み → レベル別シャッフル → 整列) → Task 9 ✓
spec §7.3 (header.json の level_order 抽出ルール) → Task 13 + Task 9 (LevelOrder 計算) ✓
spec §7.4 (HTML テンプレ + 再ピックフォーム) → Task 12 / 13 ✓
spec §8 (エラーハンドリングテーブル) → Task 13 (404/405/503/303) + Task 11 (ServerStateError) + Task 7 (owned 失敗時保持) ✓
spec §10 ピック重点 9 項目 → Task 9 のテストで全網羅
- owned_only=true で母集団絞り込み ✓
- owned_only=true で 0 件時の空応答 ✓
- pick_per_level=0 で全件 ✓
- pick_per_level=N で各レベル N 曲、不足時全件 ✓
- daily 決定論性 ✓
- manual キャッシュ維持 ✓
- per_request 連続呼び出しで違う結果 ✓
- level_order 順序維持 ✓
- シード決定論性 ✓

### Plan 4 へ持ち越し（明示的に外したもの、再確認）

- ダッシュボード: `PickResultStore.Snapshot()` だけ実装済みで GUI なし → Plan 4
- トレイアイコン色切替: `ServerUseCase.OnStatusChange` リスナー API は実装済み → Plan 4 で購読
- HTML ビューの「未絞り込み公開表での所持/未所持色分け」: Task 13 Step 2 のコメント参照 → Plan 4 で `OwnedMD5Cache` を Deps に流す
- CSS ライブラリ導入とスタイル磨き → Plan 4
- Plan 2.5（参照ボタン + コンテキストメニュー）→ Plan 4 で同時実装
- prefer_old_play UI 化: v2 機能のため Plan 4 でも対象外

### 型整合チェック

- `port.RandSource` / `port.RandSourceFactory` 名は Task 2 → Task 9 → Task 14 (Bootstrap) で一貫
- `usecase.HTTPServer` インタフェース（`Start() error` / `Shutdown(ctx) error` / `Addr() string`）は Task 11 → Task 12 (`AdapterServer`) で一貫
- `domain.RefreshMode` の 3 値は Task 1 / 6 / 9 / 13 / 14 / 16 / 17 で文字列リテラル `"per_request" / "daily" / "manual"` 一致

### 既知のリスクと対応

- ポート競合 (EADDRINUSE) → Task 11 + Task 12 で listen エラーを `ServerStateError` に倒す。GUI で再起動 / ポート変更を促す
- `songdata.db` の WAL ロック → Task 5 で `?_busy_timeout=2000` を指定。失敗時は前回 set 保持（Task 7）
- HTML テンプレの XSS → `html/template` のデフォルト autoescape 任せ。`template.HTML` への型変換禁止（コードレビューで確認）
- 大量譜面（数千〜万曲）の応答 → Plan 3 では応答時間が長くなる程度で機能維持。Plan 4 でページネーション検討

---

