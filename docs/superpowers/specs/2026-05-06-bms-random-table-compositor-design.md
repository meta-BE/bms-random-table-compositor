# BMS Random Table Compositor 設計ドキュメント

- 作成日: 2026-05-06
- ステータス: ドラフト（実装前）

## 1. 概要と目的

既存のBMS難易度表をローカルで再ホストし、編集を加えて beatoraja に提供するWindows向けデスクトップアプリケーション。タスクバーに常駐し、ローカルHTTPサーバとして編集済み難易度表を配信する。

主な編集機能:

- レベルごとのランダムピック（M曲のみ抽出）
- 所持譜面のみ表示（beatoraja の `songdata.db` を参照）
- 複数難易度表の合成（**MVPスコープ外、v2機能**）

ターゲット環境はWindowsだが、開発機の都合上 macOS でも動作する必要がある（タスクバー常駐は両OS、コア機能は両OSで利用可能）。

## 2. スコープ

### MVPに含む

- Wails常駐アプリ + システムトレイ + ローカルHTTPサーバ
- ソース表の登録（HTML or header.json直 の両入力対応）と起動時バックグラウンド更新 + 手動更新
- 「1公開表 = 1ソース表」の管理（合成なし）
- 所持譜面絞り込み（songdata.db参照）
- ランダムピック（`per_request` / `daily` / `manual` の3モード）
- HTTP応答3エンドポイント（HTMLビュー / `header.json` / `data.json`）
- 設定UI（ポート / `songdata.db`パス / ソース表 / 公開表）
- ダッシュボード（最近のリクエスト / ソース表更新履歴 / 現在のピック結果）
- ロギング + 日次ローテ
- Goユニットテスト

### MVPに含まない（v2以降）

- 複数ソース表の合成・レベルマッピング
- ピックアルゴリズムB（シャッフル＋ローテーション）/ C（重み付きランダム）
- 最終プレイ日時優先（データソース未定）
- コースデータ（完全に無視）
- インストーラ / 自動アップデート
- クロスビルドCI

## 3. 技術スタック

| 層 | 技術 |
|---|---|
| 言語 | Go 1.24+ |
| デスクトップフレームワーク | [Wails v2](https://wails.io/) v2.11+ |
| フロントエンド | Svelte + TypeScript + Vite |
| ローカルDB | `modernc.org/sqlite`（Pure Go SQLite） |
| HTTPサーバ | Go標準 `net/http` |
| HTMLパース | `golang.org/x/net/html` |
| システムトレイ | `getlantern/systray` または同等（事前検証必要） |
| ロギング | 標準 `log/slog` + `gopkg.in/natefinch/lumberjack.v2`（日次ローテ） |
| テスト | 標準 `testing` + `github.com/stretchr/testify` |

bms-elsa（Go + Wails v2 + Svelte + modernc/sqlite）の知見・パターンを最大限踏襲する。

### システムトレイの事前検証

Wails v2 の WebView2 メインスレッド要件と、systray ライブラリの OS スレッド要件の競合パターンに留意。実装プランの最初のタスクとして、最小サンプルで以下を検証する:

- ウィンドウ閉じ→トレイのみ稼働→トレイメニューから設定再表示・終了
- HTTPサーバ起動状態に応じたトレイアイコン色切替

検証失敗時のフォールバック:

- GUIを残すなら **GUIとHTTPサーバ/トレイのプロセス分離**（IPC追加で複雑化）
- ブラウザベースに切り替えるなら **設定画面もHTTP経由のWebUI**（Wails不採用）

## 4. アーキテクチャ概要

### コンポーネント構成

```
┌─ bms-random-table-compositor (single Wails binary) ──────────────┐
│                                                                   │
│  ┌─ Tray (systray) ──────┐                                        │
│  │ • Status icon         │ ← HTTPサーバ稼働状態を反映（緑/赤）    │
│  │ • Menu: 設定 / 終了   │                                        │
│  └────────┬──────────────┘                                        │
│           ▼                                                        │
│  ┌─ Wails Window (Svelte UI) ─┐                                   │
│  │ • 設定画面                  │ Bind経由 ┌─ Go core ────────┐    │
│  │ • ソース表管理              │ ───────→ │ Handlers          │    │
│  │ • 公開表管理                │ ◀─────── │                  │    │
│  │ • ダッシュボード/ログ       │ events   └──────────────────┘    │
│  └─────────────────────────────┘                                   │
│                                                                    │
│  ┌─ HTTP Server (net/http, goroutine) ──────────────────────────┐ │
│  │  /:slug             → HTML view                              │ │
│  │  /:slug/header.json → bmstable header                        │ │
│  │  /:slug/data.json   → bmstable data                          │ │
│  │  POST /:slug/_refresh → 手動再ピック (manual mode のみ)      │ │
│  └────────┬─────────────────────────────────────────────────────┘ │
│           ▼                                                        │
│  ┌─ Core services (Go) ─────────────────────────────────────────┐ │
│  │  • SourceTableUseCase   (ソース表CRUD + バックグラウンド取得)│ │
│  │  • PublishedTableUseCase(公開表CRUD)                          │ │
│  │  • PickUseCase          (シード/モード別ピック + 所持絞り込み)│ │
│  │  • ServerUseCase        (HTTPサーバの起動/停止/状態)         │ │
│  │  • PickResultStore      (in-memory pick cache)               │ │
│  │  • Logger               (日次ローテログ + GUI連携)           │ │
│  └──────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────┘
                                  ↕
                        ┌─ Filesystem (ポータブル) ─┐
                        │ ./compositor.db           │
                        │ ./logs/YYYY-MM-DD.log     │
                        │ ./.lock                   │
                        └───────────────────────────┘
                                  ↕
                        beatoraja の songdata.db (read-only)
                        外部の難易度表サイト (HTTP取得)
```

### プロセス境界・ライフサイクル

- **単一プロセス**: Wailsアプリのメインプロセス内で、トレイ・GUI・HTTPサーバ・コアサービスをすべて稼働
- **シングルインスタンスロック**: 起動時に `./.lock` ファイルへ排他ロック（`syscall.Flock` / Windows は `LockFileEx`）。既存インスタンスがあれば既存窓を前面化（簡易IPC: 名前付きパイプ or 監視ファイル）して新インスタンスは即終了
- **ウィンドウクローズ時**: Wails の `OnBeforeClose` で `runtime.WindowHide()` を呼び、トレイに格納（要件「タスクバー常駐」を満たす）。HTTPサーバとコアサービスは動き続ける
- **トレイ「終了」メニュー**: HTTPサーバを `Shutdown(ctx)` でグレースフル停止 → DBクローズ → ロック解放 → プロセス終了

### 依存方向（クリーンアーキテクチャ風）

bms-elsa の `internal/{adapter,app,domain,port,usecase}` 構造に倣う:

```
domain    : SourceTable, PublishedTable, SourceChart, PickResult 等の純粋データ型
usecase   : ピック、所持突合、ソース表更新等のビジネスロジック
port      : OwnedChartRepo, SourceTableFetcher, ConfigStore のインタフェース
adapter   : modernc/sqlite, net/http, ファイルシステム, トレイ実装
app       : Wails Bind層、HTTPハンドラ配線、サービスの起動・配線
```

外側（adapter, app）が内側（domain, usecase, port）に依存する方向のみ。

## 5. データモデル

### ファイルレイアウト（実行ファイル隣）

```
./bms-random-table-compositor.exe (or .app)
./compositor.db                       # アプリ本体DB (read-write, modernc/sqlite)
./logs/YYYY-MM-DD.log                 # 日次ローテーションログ
./.lock                               # シングルインスタンスロックファイル
```

外部参照: ユーザーが設定画面で指定した beatoraja の `songdata.db`（read-only でオープン）。

### `compositor.db` スキーマ

bms-elsa方式（`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE` の冪等順次実行）に倣う。

```sql
-- 1. 設定（key-valueストア）
CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- config の使用例:
--   ('schema_version', '1')
--   ('server_port', '50000')
--   ('songdata_db_path', '/path/to/songdata.db')

-- 2. ソース表
CREATE TABLE IF NOT EXISTS source_table (
    id                TEXT PRIMARY KEY,            -- ULID
    input_url         TEXT NOT NULL,               -- ユーザー入力URL（HTML or header.json直）
    input_kind        TEXT NOT NULL CHECK(input_kind IN ('html', 'header_json')),
    display_name      TEXT NOT NULL DEFAULT '',    -- ユーザー編集可。空ならNameを使う
    name              TEXT NOT NULL DEFAULT '',    -- header.json の name
    symbol            TEXT NOT NULL DEFAULT '',
    level_order_json  TEXT NOT NULL DEFAULT '[]',  -- header.json の level_order をJSON文字列で保存
    data_url          TEXT NOT NULL DEFAULT '',    -- 解決済みdata_url（絶対URL）
    etag              TEXT NOT NULL DEFAULT '',
    last_fetched_at   TEXT,                        -- ISO8601
    last_fetch_status TEXT NOT NULL DEFAULT 'never' CHECK(last_fetch_status IN ('never','ok','error')),
    last_fetch_error  TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 3. ソース表の譜面エントリ（取得した data.json を行展開）
CREATE TABLE IF NOT EXISTS source_table_chart (
    source_id  TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,                   -- data.json の元の並び順
    md5        TEXT NOT NULL,
    sha256     TEXT NOT NULL DEFAULT '',
    level      TEXT NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    artist     TEXT NOT NULL DEFAULT '',
    raw_json   TEXT NOT NULL DEFAULT '{}',         -- 元エントリの完全JSON（パススルー用）
    PRIMARY KEY (source_id, position)
);
CREATE INDEX IF NOT EXISTS idx_stc_md5 ON source_table_chart(md5);
CREATE INDEX IF NOT EXISTS idx_stc_source_level ON source_table_chart(source_id, level);

-- 4. 公開表
CREATE TABLE IF NOT EXISTS published_table (
    id                 TEXT PRIMARY KEY,           -- ULID
    slug               TEXT NOT NULL UNIQUE,
    display_name       TEXT NOT NULL,
    symbol             TEXT NOT NULL DEFAULT '',
    source_table_id    TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
    owned_only         INTEGER NOT NULL DEFAULT 0, -- 0/1
    pick_per_level     INTEGER NOT NULL DEFAULT 0, -- 0=無制限
    pick_refresh_mode  TEXT NOT NULL DEFAULT 'manual'
                       CHECK(pick_refresh_mode IN ('per_request', 'daily', 'manual')),
    prefer_old_play    INTEGER NOT NULL DEFAULT 0, -- v2用フラグ
    sort_order         INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### domain層の主要型

```go
type SourceTable struct {
    ID              string
    InputURL        string
    InputKind       InputKind     // HTML | HeaderJSON
    DisplayName     string
    Name            string        // header.json の name
    Symbol          string
    LevelOrder      []string
    DataURL         string        // 解決済み絶対URL
    ETag            string
    LastFetchedAt   *time.Time
    LastFetchStatus FetchStatus   // Never | OK | Error
    LastFetchError  string
}

type SourceChart struct {
    SourceID string
    Position int
    MD5      string
    SHA256   string
    Level    string
    Title    string
    Artist   string
    Raw      map[string]any // raw_jsonをデコードしたもの。応答時にそのまま使う
}

type PublishedTable struct {
    ID            string
    Slug          string
    DisplayName   string
    Symbol        string
    SourceTableID string
    OwnedOnly     bool
    Pick          PickConfig
}

type PickConfig struct {
    PerLevel      int           // 0=無制限
    RefreshMode   RefreshMode   // PerRequest | Daily | Manual
    PreferOldPlay bool          // v2用フラグ
}

// 揮発、in-memory
type PickResult struct {
    PublishedTableID string
    GeneratedAt      time.Time
    Charts           []SourceChart
}
```

### 重要な設計判断

1. **ソース表のキャッシュは `compositor.db` 内に正規化保存**: 譜面を行展開しつつ、表固有フィールド（`url`, `url_diff`, `lr2_bmsid` 等）は `raw_json` カラムでパススルー保持
2. **songdata.db は別ファイル（read-only）として参照**: `compositor.db` に ATTACH せず、Go側で「対象md5集合」をクエリ取得→メモリ上でセット突合
3. **ピック結果はメモリ常駐 + 揮発**: `PickResultStore` がメモリで保持。`refresh_mode: "daily"` は「ローカル日付」をシードに含めて決定的に再生成（再起動でも同じ結果）
4. **slug衝突チェック**: `published_table.slug` に `UNIQUE` 制約。予約語（`_admin`等）はバリデーションで弾く
5. **マイグレーション**: `RunMigrations(db *sql.DB) error` を `internal/adapter/persistence/migrations.go` に置き、bms-elsa方式で冪等順次実行

### GUI 新規公開表作成時のデフォルト値

| フィールド | デフォルト |
|---|---|
| `display_name` | ソース表の `display_name` または `name` |
| `slug` | ソース表名から自動生成（kebab-case化、衝突時は末尾連番） |
| `symbol` | ソース表の `symbol` |
| `owned_only` | `false` |
| `pick_per_level` | `0`（無制限） |
| `pick_refresh_mode` | `per_request` |
| `prefer_old_play` | `false`（v2用） |

## 6. 主要コンポーネントとインタフェース

### Port（インタフェース、`internal/port/`）

```go
// SourceTableFetcher: 外部URLから難易度表を取得する
type SourceTableFetcher interface {
    FetchByHTML(ctx context.Context, htmlURL string, etag string) (FetchedTable, error)
    FetchByHeader(ctx context.Context, headerURL string, etag string) (FetchedTable, error)
}

type FetchedTable struct {
    Header      domain.BMSTableHeader
    Charts      []domain.SourceChart
    ETag        string
    NotModified bool                  // 304時 true
}

// SourceTableRepo: ソース表のメタとキャッシュを永続化
type SourceTableRepo interface {
    List(ctx context.Context) ([]domain.SourceTable, error)
    Get(ctx context.Context, id string) (domain.SourceTable, error)
    Create(ctx context.Context, in domain.SourceTable) (string, error)
    Update(ctx context.Context, t domain.SourceTable) error
    Delete(ctx context.Context, id string) error
    SaveFetched(ctx context.Context, sourceID string, ft FetchedTable, fetchedAt time.Time) error
    MarkFetchError(ctx context.Context, sourceID string, err error, fetchedAt time.Time) error
    LoadCharts(ctx context.Context, sourceID string) ([]domain.SourceChart, error)
}

// PublishedTableRepo
type PublishedTableRepo interface {
    List(ctx context.Context) ([]domain.PublishedTable, error)
    GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error)
    Create(ctx context.Context, t domain.PublishedTable) (string, error)
    Update(ctx context.Context, t domain.PublishedTable) error
    Delete(ctx context.Context, id string) error
}

// ConfigStore: server_port, songdata_db_path 等のkey-value設定
type ConfigStore interface {
    Get(ctx context.Context, key string) (string, bool, error)
    Set(ctx context.Context, key string, value string) error
}

// OwnedChartRepo: beatorajaのsongdata.dbから所持譜面（md5集合）を取得
type OwnedChartRepo interface {
    LoadOwnedMD5Set(ctx context.Context) (map[string]struct{}, error)
}
```

### UseCase（`internal/usecase/`）

- `SourceTableUseCase`: ソース表の登録・更新・削除と、バックグラウンド取得
- `PublishedTableUseCase`: 公開表のCRUD + slug衝突/予約語チェック
- `PickUseCase`: 公開表に対するピック実行（所持絞り込み + ランダム/全件）
- `ServerUseCase`: HTTPサーバの起動・停止・ステータス管理
- `PickResultStore`: メモリ上のピック結果キャッシュ

### Adapter（`internal/adapter/`）

```
adapter/
├── persistence/
│   ├── migrations.go              // RunMigrations(*sql.DB)
│   ├── source_table_repo.go
│   ├── published_table_repo.go
│   ├── config_store.go
│   └── songdata_reader.go         // OwnedChartRepo 実装（bms-elsa流用パターン）
├── gateway/
│   └── bmstable_fetcher.go        // SourceTableFetcher 実装
│                                  //   - HTMLパース: golang.org/x/net/html
│                                  //   - HTTP: 標準net/http、相対data_urlの絶対化、ETag対応
│                                  //   - GASリダイレクト: http.Client標準で302追従
├── httpserver/
│   ├── server.go
│   ├── router.go
│   ├── handler_html.go            // HTMLビュー（html/template）
│   ├── handler_header.go
│   ├── handler_data.go
│   └── templates/index.html
├── tray/
│   └── tray.go
└── logger/
    └── logger.go
```

### App層（`internal/app/`）

```go
// app.go: Wails NewApp() に集約、Bind対象のハンドラを公開
type App struct {
    ConfigService          *handler.ConfigHandler
    SourceTableHandler     *handler.SourceTableHandler
    PublishedTableHandler  *handler.PublishedTableHandler
    PickHandler            *handler.PickHandler   // 「再ピック」ボタン用
    ServerStatusHandler    *handler.ServerStatusHandler
    LogHandler             *handler.LogHandler

    server *httpserver.Server
    tray   *tray.Tray
}
```

## 7. データフロー

### 7.1 ソース表の取り込み（HTML起点）

```
ユーザー入力: https://stellabms.xyz/st/table.html
       │
       ▼
[1] SourceTableUseCase.Add(input_url, input_kind=html)
       │   - source_table 行を作成（last_fetch_status='never'）
       │   - id返却（GUIで即座に行表示）
       ▼
[2] バックグラウンド goroutine: refreshOne(id)
       │
       ▼
[3] BMSTableFetcher.FetchByHTML(htmlURL, etag="")
       │   ① GET html → HTMLパース
       │      - golang.org/x/net/html で <meta name="bmstable" content="...">
       │      - content属性が相対なら htmlURL を基準に絶対化
       │   ② GET header.json → JSON Decode
       │      - data_url を絶対化（headerURL基準）
       │   ③ GET data.json (with If-None-Match: etag)
       │      - 302リダイレクトはhttp.Client標準で追従（GAS対応）
       │      - 譜面エントリを []SourceChart に変換、Raw に元JSON保持
       │   ④ FetchedTable返却（Header, Charts, ETag）
       ▼
[4] SourceTableRepo.SaveFetched(id, fetched, now)
       │   - Tx内で:
       │     - source_table を UPDATE
       │     - source_table_chart を全削除→再挿入（位置順）
       ▼
[5] EventEmit("source_table_updated", id) → GUI更新通知
```

`header.json` 直URL入力時はステップ ③① をスキップし `[3]②` から開始。

起動時の `RefreshAll`: 全 source_table を並列度4で `refreshOne` 実行。GUIをブロックしない。

### 7.2 HTTP応答 `/:slug/data.json`

```
GET /:slug/data.json
       │
       ▼
[1] PublishedTableRepo.GetBySlug(slug)
       │   - 404: 存在しない → JSON {"error":"not_found"}
       ▼
[2] PickUseCase.Pick(slug)
       │
       ├─[2a] PickResultStore.Get(publishedID) でキャッシュ確認
       │       refresh_mode=manual: キャッシュあれば即返却、なければ生成
       │       refresh_mode=daily: GeneratedAtの日付が今日なら即返却、違えば再生成
       │       refresh_mode=per_request: 常に再生成
       │
       └─[2b] 生成パス:
              ① SourceTableRepo.LoadCharts(sourceID) → []SourceChart
              ② owned_only=true:
                 - OwnedChartRepo.LoadOwnedMD5Set() で md5 set 取得
                 - チャートを set に含むもののみに絞る
              ③ レベル別にグループ化（map[string][]SourceChart）
              ④ pick_per_level == 0: そのまま全件
                 pick_per_level > 0: 各レベルから ランダム N 曲選出
                   - シード生成:
                     * per_request: time.Now().UnixNano() + publishedID hash
                     * daily: ymd(local) + publishedID hash
                     * manual: 手動更新時刻 + publishedID hash
                   - rand.Shuffle で並べ替え後、先頭N曲を採用
                   - 不足時(N > レベル内譜面数)はそのレベルの全件を採用
              ⑤ 整列:
                 - レベル間の順序: source_table.level_order に従う
                 - レベル内の順序: source_table_chart.position の昇順で安定整列
              ⑥ PickResultStore.Set(publishedID, result)
       ▼
[3] JSON応答ボディ生成:
       - 各SourceChart の Raw マップをコピー
       - Raw に level/title/artist/md5/sha256 の最新値をマージ（パススルーしつつ正規化）
       - JSON配列としてエンコード、Content-Type: application/json
```

### 7.3 HTTP応答 `/:slug/header.json`

```json
{
  "name":        "<publishedTable.DisplayName>",
  "symbol":      "<publishedTable.Symbol>",
  "data_url":    "data.json",
  "level_order": ["<ピック後に1曲以上残ったレベルのみ抽出>"]
}
```

`level_order` の計算ルール:

- ソース表の `level_order` をベースに、ピック後（所持絞り込み + ランダム選出後）に1曲以上残ったレベルのみを順序維持で抽出
- ソース表に `level_order` が無い場合は、ピック後の実在レベルを文字列の自然順でソートして使う
- `course` フィールドは含めない（コース機能は完全無視）

### 7.4 HTTP応答 `/:slug` HTMLビュー

`html/template` を使った静的レンダー。

```html
<meta name="bmstable" content="header.json">
<h1>{{.DisplayName}}</h1>
<form method="POST" action="/{{.Slug}}/_refresh"><!-- manual mode のみ表示 -->
  <button>再ピック</button>
</form>
{{range .Levels}}
  <h2>{{$.Symbol}}{{.Level}}</h2>
  <table>
    {{range .Charts}}
    <tr class="{{if .Owned}}owned{{else}}unowned{{end}}">
      <td>{{.Title}}</td><td>{{.Artist}}</td><td>{{.MD5}}</td>
    </tr>
    {{end}}
  </table>
{{end}}
```

「再ピック」エンドポイント `POST /:slug/_refresh` は `manual` モードでのみ受け付け、`PickUseCase.ManualRefresh` を呼ぶ。他モードでは405。

## 8. エラーハンドリング

| 事象 | 挙動 |
|---|---|
| ポート確保失敗（Listen時 EADDRINUSE） | `ServerStatus = error`、Trayアイコン赤、設定画面に注意表示。HTTPサーバは未起動。ユーザーがポート変更→保存→再起動試行 |
| ソース表取得失敗（HTTP/パースエラー） | `last_fetch_status='error'`, error保存。**前回成功時のキャッシュは保持**し、公開表は応答可能。GUIに警告バッジ |
| 公開表のソース表が `last_fetch_status='never'` | HTTP応答は503 + JSONエラー。GUI上でも警告 |
| songdata.dbオープン失敗 / 未設定 | `owned_only=true`の公開表は譜面0件として応答。GUIに警告。`owned_only=false`の表は無影響 |
| slugに予約語（`_admin`, `_health`等）/ 既存使用済み / 不正文字 | バリデーションで拒否、GUIにエラー表示 |
| シングルインスタンス: 二重起動 | 既存窓に「Show」シグナルを送って前面化（ロックファイル隣の名前付きパイプ or watchファイル）して新インスタンスは即終了 |
| ウィンドウクローズ | OnBeforeClose で `runtime.WindowHide` → トレイのみ稼働。HTTPサーバは継続 |
| トレイ「終了」 | HTTPサーバ Shutdown → DBクローズ → ロック解放 → exit |

## 9. ロギング

- 全 `usecase` / `httpserver` メソッドが冒頭で構造化ログを出力（`log/slog`）
- レベル: DEBUG（リクエスト詳細）/ INFO（取り込み結果、サーバ起動）/ WARN（取得失敗、ファイル無し）/ ERROR（致命的）
- 出力先:
  - 標準エラー（開発時）
  - `./logs/YYYY-MM-DD.log`（lumberjackで日次ローテ、サイズ上限50MB、保持7日）
- GUIダッシュボード: 「最近のリクエスト100件」「ソース表更新履歴」「現在のピック結果サマリ」をメモリリングバッファ + イベント通知で表示

## 10. テスト方針

### レイヤー

| レイヤー | 対象 | 方針 |
|---|---|---|
| ユニット | `domain` の純粋関数、`usecase` のロジック | 標準 `testing` + `testify/assert`。外部依存はPortモック化 |
| アダプタ | `adapter/persistence`、`adapter/gateway`、`adapter/httpserver` | SQLiteは `:memory:` DB、HTTPは `httptest.Server` |
| E2E（手動） | Wails起動 → トレイ常駐 → ブラウザ確認 → beatoraja実機接続 | 自動化なし。手順は `docs/test-plan.md` に記述 |

### 重点的にカバーするテスト

1. **`PickUseCase.Pick`** — ロジックの中核
   - `owned_only=true` で母集団が所持譜面のみに絞られること
   - `owned_only=true` + 所持0件で空応答
   - `pick_per_level=0` で全件返ること
   - `pick_per_level=3` で各レベル3曲（不足時はそのレベルの全件）
   - `refresh_mode=daily` の決定論性（同一日付で同じ結果、日付が変わると違う）
   - `refresh_mode=manual` のキャッシュ維持
   - `refresh_mode=per_request` の連続呼び出しで（高確率で）異なる結果
   - `level_order` に従って結果が並ぶこと
   - シード決定論性（同じシードで同じ結果）

2. **`bmstable_fetcher`**
   - HTMLから `<meta name="bmstable">` 抽出（相対 / 絶対両対応）
   - `data_url` 絶対化
   - data.json エントリ正規化（既知フィールド抽出 + Raw保持）
   - 302リダイレクト追従（`httptest.Server` でGAS模擬）
   - ETag対応（304時 NotModified=true）
   - パースエラー時の適切なエラー型

3. **`source_table_repo` / `published_table_repo`**
   - CRUD一通り
   - `SaveFetched` のトランザクション性（更新と再挿入の原子性）
   - `slug UNIQUE` 制約違反時のエラー
   - マイグレーション冪等性（複数回 `RunMigrations` で成功）

4. **`songdata_reader`**（OwnedChartRepo）
   - 既存testdata `songdata.db` で md5集合の正しい取得
   - DBファイル不存在時のエラーハンドリング
   - 大量行（1万行以上）でのパフォーマンス（100ms以内目標）

5. **`httpserver`** ハンドラ
   - `httptest.NewRecorder` で各エンドポイント検証
   - 存在しないslug → 404
   - `POST /:slug/_refresh` のmanualモード以外で405

### モック戦略

- `Clock` インタフェース: `time.Now()` 注入用（`refresh_mode=daily` テストで日付固定）
- `RandSource` インタフェース: `math/rand` のソース注入（決定論的テスト用）
- `OwnedChartRepo` モック: テーブル駆動でmd5集合を差し込み
- HTTP外部呼び出しは `httptest.Server` で代替

### CIスコープ（MVP）

- **GitHub Actions**: PR時に `go test ./...` + `go vet ./...` + `gofmt -l` チェック
- **クロスビルド**: 当面手動（`make build` + Wails標準コマンド）
- **golangci-lint**: 設定は最小（標準ルール）

## 11. プロジェクト構造

```
bms-random-table-compositor/
├── main.go
├── app.go                          // NewApp(), Init(), startup, shutdown, OnBeforeClose
├── go.mod / go.sum
├── wails.json
├── Makefile
├── README.md
├── CLAUDE.md
├── .gitignore
├── frontend/                       // Svelte + TypeScript + Vite
│   ├── package.json
│   ├── svelte.config.js
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── App.svelte
│       ├── lib/
│       │   ├── tabs/
│       │   │   ├── ServerTab.svelte
│       │   │   ├── SourceTablesTab.svelte
│       │   │   ├── PublishedTablesTab.svelte
│       │   │   └── DashboardTab.svelte
│       │   └── api.ts
│       └── main.ts
├── internal/
│   ├── domain/
│   ├── port/
│   ├── usecase/
│   ├── adapter/
│   │   ├── persistence/
│   │   ├── gateway/
│   │   ├── httpserver/
│   │   ├── tray/
│   │   └── logger/
│   └── app/
│       └── handler/
├── testdata/
│   ├── songdata.db                 // 既存
│   ├── satellite_header.json       // 既存
│   ├── satellite_data.json         // 既存
│   └── ...                         // テスト用に追加
├── docs/
│   ├── superpowers/specs/
│   │   └── 2026-05-06-bms-random-table-compositor-design.md
│   ├── manual.md
│   └── test-plan.md
└── build/                          // Wails生成成果物（gitignore）
```

## 12. 主要依存

```
require (
    github.com/wailsapp/wails/v2     v2.11.0
    modernc.org/sqlite               v1.46.1
    golang.org/x/net                 v0.35.0
    github.com/oklog/ulid/v2         latest
    github.com/getlantern/systray    latest      // 事前検証必要
    gopkg.in/natefinch/lumberjack.v2 latest
    github.com/stretchr/testify      latest
)
```

## 13. 開発環境セットアップ

### 前提

- Go 1.24+
- Node.js 20+
- Wails CLI v2.11+
  ```
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
  ```

### 初回セットアップ

```bash
cd frontend && npm install
wails doctor          # ビルド前提を確認
```

### 開発・テスト・ビルド

```bash
make dev              # wails dev で開発サーバ起動（フロントエンドホットリロード）
make test             # go test ./...
make lint             # gofmt -l + go vet ./...
make build            # 現在のOS向け本番ビルド
make build-windows    # Windows向けクロスビルド
make clean            # build/, frontend/dist/ 等を削除
```

### CLAUDE.md（プロジェクト固有指示）

新規作成し、以下を記載:

- ビルド: `go build ./...` を使う（`go build .` はバイナリ出力するため不可）
- マイグレーション: スキーマ変更は `internal/adapter/persistence/migrations.go` に冪等な ALTER 文として追加。既存DBが壊れないよう `CREATE IF NOT EXISTS` / `pragma_table_info` チェックで保護。必ずユニットテストを追加
- フロントエンド: 設定画面のUI規約は `docs/style-guide.md` に従う
- マニュアル: ユーザー向けは `docs/manual.md`。機能追加時に更新

## 14. 将来拡張（v2以降）

| 項目 | 概要 |
|---|---|
| **複数ソース表の合成（マッピングUI）** | ユーザー定義のレベル多対一マッピング。`published_table_level_mapping` テーブル + GUIエディタ。`published_table.source_table_id` を削除してマッピング経由参照に変更 |
| **ピックアルゴリズムB**（シャッフル＋ローテーション） | 全曲シャッフル順列を作りNずつ消費する公平モード。カーソル位置の永続化が必要 |
| **ピックアルゴリズムC**（重み付きランダム） | 直近出現曲の重みを下げる。最終プレイ日時優先と統合可能 |
| **最終プレイ日時優先** | beatorajaの scorelog/score 参照（データソース未定）。古い譜面ほど重みを上げる |
| **ETag/304対応の本格運用** | 取得頻度を抑えるための条件付きGET。設定UI上は「自動」 |
| **ソース表のスケジュール自動更新** | 起動時のみではなく、N時間ごとの定期更新 |
| **クロスビルドCI** | GitHub Actionsで Windows / macOS バイナリ自動生成 |

### v2参考: 合成表のレベルマッピングテーブル

```sql
CREATE TABLE published_table_level_mapping (
    published_table_id TEXT NOT NULL REFERENCES published_table(id) ON DELETE CASCADE,
    target_level       TEXT NOT NULL,     -- 公開表のレベル "mix1"
    source_table_id    TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
    source_level       TEXT NOT NULL,     -- ソース表のレベル "sl0"
    PRIMARY KEY (published_table_id, target_level, source_table_id, source_level)
);
```

## 15. 主要な設計判断のサマリ

| # | 判断 | 採用 | 理由 |
|---|---|---|---|
| 1 | ターゲットOS | Windowsメイン + macOS動作 | 開発機がmacOS、配布対象はWindows |
| 2 | フレームワーク | Go + Wails v2 + Svelte | bms-elsa踏襲で既存知見を活用 |
| 3 | DB | SQLite単一ファイル `compositor.db` | bms-elsa流儀、SQLでの突合容易 |
| 4 | songdata.db扱い | 別ファイルRead-only、Go側でmd5集合突合 | ATTACH不要でシンプル |
| 5 | ソース表/公開表モデル | 2層分離 | 1ソース表を複数公開表で再利用可能 |
| 6 | 合成機能 | MVPスコープ外（v2） | UI実装コストが大、まずはコア機能優先 |
| 7 | ランダムピック更新 | per_request / daily / manual の3モード | シンプル、要件範囲で十分 |
| 8 | 所持絞り込み順序 | 所持で絞ってからピック | ユーザー直感に合致 |
| 9 | コースデータ | 完全無視 | 合成時の複雑さ回避 |
| 10 | ポート衝突時 | エラー表示のみ、自動変更しない | beatoraja側のURL固定が前提 |
| 11 | 設定保存場所 | 実行ファイル隣 | ポータブル運用 |
| 12 | ピック結果 | メモリ揮発、daily は日付シードで決定論 | 永続化不要、再起動でも一貫 |
| 13 | HTMLビュー | 標準的な表ビュー（所持色分け、再ピックボタン） | デバッグ・運用上の確認に有用 |
| 14 | テスト | ユニット中心、E2Eは手動 | MVPで実装スコープを絞る |
| 15 | ロギング | slog + lumberjack日次ローテ + GUIダッシュボード | 常駐アプリの可観測性 |
