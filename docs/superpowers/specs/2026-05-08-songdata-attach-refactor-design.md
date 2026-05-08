# songdata.db ATTACH リファクタ設計

- 作成日: 2026-05-08
- ステータス: ドラフト（実装前）
- 対象範囲: 所持楽曲データの取り込みフローの再設計

## 1. 背景と目的

### 現状の問題

ピック生成時の所持判定は `usecase.OwnedMD5Cache` が `port.OwnedChartRepo.LoadOwnedMD5Set` 経由で beatoraja の `songdata.db` から `md5` 全件を `map[string]struct{}` として常駐キャッシュし、`PickUseCase.regenerate` 内で Go 側 intersect する構成になっている。

これにより以下の問題がある:

- **メモリ常駐**: 数十万譜面オーダーの songdata で 1 エントリあたり ~83 B（string ヘッダ 16 B + md5 hex 32 B + map overhead ~35 B）→ 30 万譜面で約 25 MB が常駐
- **コピーコスト**: `OwnedMD5Cache.Get()` が呼ばれるたびに `copySet` でフルコピー → `per_request` モードでリクエスト毎に同等量を alloc/GC
- **将来機能との整合**: 「最終プレイ時刻で抽選ロジックを変更する」等、`score` テーブル参照を伴う機能を追加する際に、md5 集合の発想だけでは足りず、別のキャッシュ機構やインメモリ JOIN が必要になる

### 目的

`songdata.db` を ATTACH DATABASE してメイン `*sql.DB` 経由で SQL 一発で JOIN できる構成に切り替える。これにより:

- 所持判定のメモリ常駐を **完全に削除**
- 「所持しているか」「最終プレイ時刻」等のローカル状態を、譜面ロード時の同一クエリで一括取得できる構造に統一
- 将来のフィルタ追加が `EnrichedChart` のフィールド追加と SQL 拡張だけで済む

bms-elsa の `AttachSongdata` パターン（`SetMaxOpenConns(1)` + 起動時 ATTACH）を踏襲する。

## 2. スコープ

### このリファクタに含む

- `port.OwnedChartRepo` と `usecase.OwnedMD5Cache` の削除
- `port.SourceTableRepo.LoadCharts` のシグネチャ変更（フィルタ + enriched 化）
- `domain.EnrichedChart` の追加
- `persistence.SongdataAttacher`（新規 adapter）による ATTACH/DETACH/Status 管理
- `usecase.PickUseCase.regenerate` のロジック変更（`OwnedMD5Cache` 依存除去）
- `usecase.ConfigUseCase` の `songdata_db_path` 変更ハンドラ書き換え
- ダッシュボード `OwnedCacheStatus` を `SongdataAttachStatus` に置換
- `app.Bootstrap` での `db.SetMaxOpenConns(1)` 設定 + 起動時 ATTACH
- フロントエンド: songdata.db パス設定カードへの ATTACH ステータス表示

### このリファクタに含まない

- `LastPlayedAt` の **実取得**（beatoraja `score` テーブルのカラム名・JOIN キー確定が必要）。今回は SQL で `NULL AS last_played_at` 固定とし、`EnrichedChart.LastPlayedAt = nil` で返す。フィールドと配線だけ通しておく
- 「最近プレイしてない曲を優先」等の v2 抽選ロジック
- 既存スキーマのマイグレーション変更（`source_table_chart` テーブルは触らない）
- `score` テーブル以外の beatoraja スキーマ参照（`information` 等）

## 3. 設計

### 3.1 ドメイン型

新規 `EnrichedChart`（`internal/domain/source_chart.go` に追加）:

```go
// EnrichedChart は SourceChart にローカル DB 由来の状態を載せた読み取り専用ビュー。
// 永続化はせず、リクエスト毎に SQL 経由で組み立てる。
type EnrichedChart struct {
    SourceChart                 // 既存フィールドを埋め込み
    IsOwned      bool           // sd.song に存在するか
    LastPlayedAt *time.Time     // sd.score 由来。実取得は v2、現状は常に nil
}
```

`SourceChart` 自体は変更しない。`SaveFetched` 等の書き込み経路で使い続ける。

### 3.2 ポート

`port/source_table_repo.go`:

```go
// ChartQuery は LoadCharts に渡す SQL レベルのフィルタ。
// IsOwned/LastPlayedAt 等の派生プロパティは戻り値の EnrichedChart に常に入る。
// このフィルタは「DB 段階で足切りしたい場合」だけ指定する（パフォーマンス目的）。
type ChartQuery struct {
    OwnedOnly bool   // EXISTS sd.song で足切り
    // 将来: LastPlayedBefore *time.Time など
}

type SourceTableRepo interface {
    // 既存 CRUD はそのまま
    Get(ctx, id) (SourceTable, error)
    Create(ctx, t) (string, error)
    Update(ctx, t) error
    Delete(ctx, id) error
    List(ctx) ([]SourceTable, error)
    SaveFetched(ctx, sourceID, FetchedTable, time.Time) error
    MarkFetchError(ctx, sourceID, error, time.Time) error

    // シグネチャ変更: 戻り値が []SourceChart → []EnrichedChart、第3引数 ChartQuery 追加
    LoadCharts(ctx context.Context, sourceID string, q ChartQuery) ([]EnrichedChart, error)
}
```

`port.OwnedChartRepo` (`port/owned_chart_repo.go`) は削除。

### 3.3 SongdataAttacher（新規 adapter）

`internal/adapter/persistence/songdata_attacher.go`:

```go
// SongdataAttacher はメイン *sql.DB に対する ATTACH/DETACH ライフサイクルを管理する。
// SetMaxOpenConns(1) 前提。GUI 表示用に最終アタッチ状態とエラーをスナップショット保持する。
type SongdataAttacher struct {
    db    *sql.DB
    clock port.Clock
    log   *slog.Logger

    mu       sync.RWMutex
    attached bool
    path     string
    songCount int           // SELECT COUNT(*) FROM sd.song の最終値
    attachedAt *time.Time
    lastErr  string
}

// Attach は songdata.db を 'sd' として RO ATTACH する。
// path が空なら何もしない。失敗時はステータスにエラーを記録し error を返す。
func (a *SongdataAttacher) Attach(ctx context.Context, path string) error

// Detach は 'sd' を DETACH する。未アタッチなら no-op。
func (a *SongdataAttacher) Detach(ctx context.Context) error

// ReAttach は Detach → Attach を 1 連の操作で行う（設定変更時のフック用）。
func (a *SongdataAttacher) ReAttach(ctx context.Context, path string) error

// IsAttached は現在 'sd' がアタッチされているかを返す。
// adapter 内のクエリ分岐で参照する。
func (a *SongdataAttacher) IsAttached() bool

// Status は GUI 表示用のスナップショットを返す。
func (a *SongdataAttacher) Status() domain.SongdataAttachStatus
```

ATTACH SQL は SQLite URI 形式で RO:
```sql
ATTACH DATABASE 'file:<urlEscapedPath>?mode=ro' AS sd
```

ATTACH 成功直後に `SELECT COUNT(*) FROM sd.song` を実行して `songCount` を更新する（GUI 表示と "正しい songdata.db を選んだか" の検証用途）。失敗してもエラーログのみ。

### 3.4 SourceTableRepoSQL の改修

`adapter/persistence/source_table_repo.go` の `LoadCharts` を書き換え:

```go
type SourceTableRepoSQL struct {
    db       *sql.DB
    attacher *SongdataAttacher  // IsAttached() 参照のため DI
}

func (r *SourceTableRepoSQL) LoadCharts(
    ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
    if r.attacher.IsAttached() {
        return r.loadChartsAttached(ctx, sourceID, q)
    }
    return r.loadChartsBare(ctx, sourceID, q)
}
```

#### Attached 経路の SQL

```sql
SELECT
  c.position, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
  EXISTS(SELECT 1 FROM sd.song s WHERE s.md5 = c.md5)  AS is_owned,
  NULL                                                  AS last_played_at
FROM source_table_chart c
WHERE c.source_id = ?
  AND (? = 0 OR EXISTS (SELECT 1 FROM sd.song s WHERE s.md5 = c.md5))
ORDER BY c.position ASC
```

`?` プレースホルダ: `[sourceID, ownedOnlyAsInt]`。`OwnedOnly=true` のときだけ EXISTS フィルタが効く。

`last_played_at` は今回 `NULL` リテラル固定。v2 で `(SELECT sc.<date_column> FROM sd.score sc WHERE sc.<key> = c.<key> LIMIT 1)` 等に差し替える。

#### Bare 経路（未アタッチ時）の SQL

```sql
SELECT c.position, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json
FROM source_table_chart c
WHERE c.source_id = ?
ORDER BY c.position ASC
```

Go 側で `IsOwned=false`, `LastPlayedAt=nil` をスタンプ。`OwnedOnly=true` で未アタッチの場合は **空配列を返す**（spec 既存ルール「DB 未設定時は owned_only の表は 0 件」と整合）。

### 3.5 PickUseCase の改修

`usecase.PickUseCase.regenerate` から `OwnedMD5Cache` 依存を除去:

- before:
  - `srcRepo.LoadCharts(ctx, sourceID)` → `[]SourceChart`
  - `pub.OwnedOnly` のとき `ownedCache.Get(ctx)` で md5 集合取得 → Go 側 intersect
- after:
  - `srcRepo.LoadCharts(ctx, sourceID, ChartQuery{OwnedOnly: pub.OwnedOnly})` → `[]EnrichedChart`
  - intersect ループ削除

シャッフル / レベル順制御 / シード生成は変更なし。型シグネチャだけ `[]SourceChart` → `[]EnrichedChart` に追従する。最終 `domain.PickResult.Charts` は **`[]EnrichedChart` に変更**する。理由: HTML ハンドラ (`handler_html.go`) が所持状態に応じた色分けを行うため、ピック結果と一緒に `IsOwned` フラグを保持する必要がある。HTTP サーバはローカル限定 (利用ユーザー一人) のため所持状態を `data.json` 経由で外部に出すことに問題はない。

`data.json` のシリアライズフォーマットは互換維持: `mergeChart` は `EnrichedChart` を受け取るが、内部で埋め込み `SourceChart` 部分のみ map に展開する（`is_owned`/`last_played_at` は出力しない）。

### 3.6 ConfigUseCase / Bootstrap

#### `app.Bootstrap` 変更点

- `sql.Open` 直後に `db.SetMaxOpenConns(1)` を追加
- マイグレーション後、`SongdataAttacher.Attach(ctx, configuredPath)` を呼ぶ
  - 失敗は警告ログ、起動継続
- `SongdataAttacher` を `SourceTableRepoSQL` に DI

#### `ConfigUseCase` 変更点

- `songdata_db_path` 変更時のフックを「`OwnedMD5Cache.Invalidate` + `PickUseCase.InvalidateAll`」から「`SongdataAttacher.ReAttach(newPath)` + `PickUseCase.InvalidateAll`」へ
- `OwnedMD5Cache` への参照を削除

### 3.7 ステータス UI

`domain.SongdataAttachStatus`:

```go
type SongdataAttachStatus struct {
    Attached    bool
    Path        string
    SongCount   int        // SELECT COUNT(*) FROM sd.song
    AttachedAt  *time.Time
    LastError   string
}
```

`DashboardUseCase` (または `ConfigUseCase` 経由)で GUI に露出する。フロントエンド側 (`frontend/src/`) では songdata.db パス設定カードに以下を表示:

- バッジ: ✅ アタッチ済み / ⚪ 未設定 / 🔴 エラー
- パス（マスク不要、ローカルのみ）
- 楽曲数（アタッチ済の場合）
- 最終アタッチ時刻（JST 表示、既存の UTC→JST 変換ヘルパに従う）
- エラーメッセージ（失敗時のみ）

既存の `OwnedCacheStatus` 表示コンポーネントは廃止。

### 3.8 削除リスト

- `internal/port/owned_chart_repo.go`
- `internal/usecase/owned_md5_cache.go`
- `internal/usecase/owned_md5_cache_test.go`
- `internal/adapter/persistence/songdata_reader.go` 全体（`SongdataReader` には `LoadOwnedMD5Set` 以外のメソッドが無いためファイルごと削除）
- `internal/adapter/persistence/songdata_reader_test.go`
- `domain.OwnedCacheStatus`
- `app.Bootstrap` 内の `OwnedMD5Cache` 構築コード
- フロントエンドの所持譜面ロード状態関連コンポーネント

## 4. 移行計画

このリファクタはスキーマを触らないので、ユーザー DB の互換性問題は無い。コードレベルの破壊的変更のみ。

順序:

1. `domain.EnrichedChart` / `port.ChartQuery` の追加
2. `persistence.SongdataAttacher` 実装 + テスト
3. `SourceTableRepoSQL.LoadCharts` 改修 + テスト（attached / bare 両経路）
4. `PickUseCase` 改修 + 既存テスト追従
5. `Bootstrap` / `ConfigUseCase` 配線変更
6. `OwnedChartRepo` / `OwnedMD5Cache` の削除
7. ダッシュボード / フロントエンド表示の差し替え
8. `wails generate module` でフロント側型再生成

各ステップで `go test ./...` を通す（`testdata/songdata.db` 不要なテストのみ）。

## 5. テスト戦略

### 5.1 SongdataAttacher

- 単体テスト: `testdata/songdata.db` を使った Attach/Detach/ReAttach 経路の確認、存在しないパス指定でエラー、URL エンコードが必要なパス
- ステータス遷移: 未設定 → アタッチ済み → ReAttach 成功 → 失敗時のエラー保持
- `SongCount` が ATTACH 直後に取得されること

### 5.2 SourceTableRepoSQL.LoadCharts

- attached + 未指定フィルタ: 全件 EnrichedChart で返り、`IsOwned` がテストデータ通り、`LastPlayedAt=nil`
- attached + `OwnedOnly=true`: 所持のみ、未所持は弾かれる
- bare（attacher 未アタッチ）+ `OwnedOnly=false`: 全件、`IsOwned=false` 一律、`LastPlayedAt=nil`
- bare + `OwnedOnly=true`: 空配列
- `position ASC` 順序保証

### 5.3 PickUseCase

既存テストの一部 (`fakeSourceRepo`, `fakeOwnedRepo`) を改修:

- `fakeSourceRepo.LoadCharts(ctx, id, q)` が `[]EnrichedChart` を返すよう変更
- `fakeOwnedRepo` は廃止
- `TestPickUseCase_OwnedOnlyFiltersBeforePick` 等の所持絞り込みテストは、`fakeSourceRepo` 側で「`OwnedOnly=true` のときに enriched chart の subset を返す」挙動に書き換え

決定論性テスト (`TestPickUseCase_DeterministicAcrossRestarts`) は型変更だけで挙動は変わらない。

### 5.4 統合（手動）

- macOS dev で `make dev` → 既存挙動と同じピックが返ること（`testdata/songdata.db` がある状態）
- songdata.db パスを未設定にしたとき、`OwnedOnly=true` の表が空になること
- 設定画面でパスを変更したときに ATTACH ステータスが切り替わり、ピック結果も追従すること

## 6. 並行性の注意点

`SetMaxOpenConns(1)` により全 SQLite クエリが直列化される。HTTP `GET /:slug/data.json` 並列リクエストはピック結果キャッシュ (`PickResultStore`) で大半が即返るので問題は出ないと予想。

`per_request` モードかつ大規模ソース表で `regenerate` がボトルネックになる場合は、将来的に:

- 別 `*sql.DB` を読み取り専用で持って app DB と分離
- もしくは ATTACH をリクエスト単位で行う

等を検討するが、Phase 1 範囲外。

## 7. リスクと未決事項

| 項目 | リスク | 対応 |
|---|---|---|
| ATTACH RO + WAL モードの相互運用 | beatoraja が WAL で開いている songdata.db に他プロセスから RO ATTACH するときの挙動。読み取り側は問題ないが、`-shm`/`-wal` ファイルへのアクセス権限問題が稀に出る | 手動テストで確認。失敗時は `lastErr` に詳細出してユーザーに通知 |
| `SetMaxOpenConns(1)` の直列化 | per_request モード + 高頻度リクエストで体感低下 | Phase 1 では負荷は低い前提。問題出たら Phase 2 で別 DB 分離 |
| `score` テーブル参照の準備 | 今回は `NULL` 固定なので影響無し。v2 でカラム名・JOIN キー確定が必要 | v2 設計時に beatoraja のスキーマを実機確認 |
| パス変更時のピックキャッシュ | 設定変更で ATTACH 切替後、古いピック結果が残ると整合しない | `ConfigUseCase` で `PickUseCase.InvalidateAll()` を呼ぶ（既存挙動を維持） |

## 8. 受け入れ基準

- [ ] `go test ./...` が通る（`testdata/songdata.db` を要求するテストを除く）
- [ ] `make lint` が通る
- [ ] `OwnedMD5Cache` / `OwnedChartRepo` が grep で見つからない
- [ ] ダッシュボードに ATTACH ステータスが表示される（手動確認）
- [ ] `songdata_db_path` 変更で ATTACH が切り替わり、ピック結果が更新される（手動確認）
- [ ] 数十万譜面規模で `OwnedMD5Cache` 由来のメモリ常駐が消えていることを `runtime.ReadMemStats` 等で確認（任意）
