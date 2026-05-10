# 複数ソース表合成（公開表レベルマッピング）設計

- 作成日: 2026-05-10
- ステータス: ドラフト（実装前）
- 依存スペック: `2026-05-06-bms-random-table-compositor-design.md`（全体設計）
- 関連 TODO: `docs/TODO.md` の v2 機能「複数ソース表合成」「最終プレイ日時優先」

## 1. 概要と目的

公開表 1 レベルに対して **複数の `(ソース表, ソースレベル)` を紐付け** て合成できるようにする。例: 「自作の難易度 5 表」を「stella ★5」「satellite sl5」「自作補完表 lv5」3 ソースのミックスで構成する、等。

合わせて、ピック設定を「ソース表レベルごとの最低保証 m 曲」+「公開レベル全体の目標合計 n 曲」に再設計し、マッピング数に依らず合計曲数が揃う UX にする。将来追加予定の **最終プレイ日時優先ピック** はピックアルゴリズムを修正せずに差し込めるよう、`Weighter` インターフェースを今回導入しておく。

## 2. スコープ

### In

- `PublishedTable` のデータモデル再設計（公開レベルリスト + 各レベルへのソースマッピングリスト）
- DB スキーマ刷新（`published_table.source_table_id` を撤去 / 新テーブル 2 つを追加）
- 公開表編集 UI 刷新（公開レベル一覧テーブル + マッピング編集 + バルク適用パネル + 作成導線 2 種）
- ピックアルゴリズム刷新（フェーズ 1: マッピング毎に m 曲 / フェーズ 2: 全体プールから合計 n 曲になるよう追加ピック）
- HTTP 出力（`header.json` / `data.json`）の公開レベル名対応
- `Weighter` インターフェース導入（実装は `UniformWeighter` のみ）

### Out

- 既存データのマイグレーション（**クリーンブレイク**: 既存公開表は破棄して再作成）
- 最終プレイ日時優先ピックの本体実装（`Weighter` の差し替え点だけ用意）
- ピックアルゴリズム B / C（シャッフル＋ローテ / 重み付き）
- HTTP キャッシュ（ETag 304）、コースデータ
- ソース表のスキーマ変更
- 公開表のシンボル/HTML レンダリング自体の改修（直近の `2026-05-09-public-table-html-redesign-design.md` の構造を維持）

### 非変更（明示的に既存仕様維持）

- `OwnedOnly` は公開表単位（全レベル共通）
- `RefreshMode` (`per_request` / `daily` / `manual`) は公開表単位（全レベルが同じシードで同期）
- `Symbol`（公開表シンボル）は公開表単位（既存）。各譜面の per-chart symbol は引き続きソース表側 symbol を `data.json` に流す
- 公開表内で同一 `(ソース表, ソースレベル)` を **複数の公開レベル** に紐付け可能（譜面が複数レベル列に登場し得る、公開表全体としての dedup はしない）

## 3. ドメインモデル

```go
// internal/domain/published_table.go (改定)
type PublishedTable struct {
    ID            string
    Slug          string
    DisplayName   string
    Symbol        string
    OwnedOnly     bool
    Pick          PickConfig // RefreshMode のみ保持。PerLevel/PreferOldPlay は廃止
    SortOrder     int
    Levels        []PublishedTableLevel // 並び順 SortOrder 昇順、name は公開レベル名
}

type PickConfig struct {
    RefreshMode RefreshMode // per_request / daily / manual
}

// internal/domain/published_table_level.go (新規)
type PublishedTableLevel struct {
    ID                string
    PublishedTableID  string
    Name              string // 公開レベル表示名（例: "5", "Lv.5", "中級", "★5-6 mix"）
    SortOrder         int
    PerMappingPick    int  // 各マッピングからの最低保証ピック数 m (>= 0)
    TotalPick         int  // 公開レベル全体の目標合計ピック数 n (>= 0)
    Mappings          []PublishedTableLevelMapping // SortOrder 昇順
}

type PublishedTableLevelMapping struct {
    ID                     string
    PublishedTableLevelID  string
    SourceTableID          string
    SourceLevel            string // ソース表内のレベル文字列（例: "5", "★5"）
    SortOrder              int
}
```

`PublishedTable.SourceTableID` は撤去。`Pick.PerLevel` / `Pick.PreferOldPlay` も撤去（後者は今回 `Weighter` 経由での将来拡張に置き換え）。

## 4. DB スキーマ

`schema_version` を `2` に上げる。**クリーンブレイク**: マイグレーションでは既存 `published_table` レコードを破棄する（ALTER せず DROP & CREATE）。

```sql
-- 旧 published_table を破棄（クリーンブレイク方針）
DROP TABLE IF EXISTS published_table;

CREATE TABLE published_table (
    id                TEXT PRIMARY KEY,
    slug              TEXT NOT NULL UNIQUE,
    display_name      TEXT NOT NULL,
    symbol            TEXT NOT NULL DEFAULT '',
    owned_only        INTEGER NOT NULL DEFAULT 0,
    pick_refresh_mode TEXT NOT NULL DEFAULT 'manual'
                      CHECK(pick_refresh_mode IN ('per_request','daily','manual')),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE published_table_level (
    id                  TEXT PRIMARY KEY,
    published_table_id  TEXT NOT NULL REFERENCES published_table(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    sort_order          INTEGER NOT NULL DEFAULT 0,
    per_mapping_pick    INTEGER NOT NULL DEFAULT 0,
    total_pick          INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_ptl_table ON published_table_level(published_table_id, sort_order);

CREATE TABLE published_table_level_mapping (
    id                         TEXT PRIMARY KEY,
    published_table_level_id   TEXT NOT NULL REFERENCES published_table_level(id) ON DELETE CASCADE,
    source_table_id            TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
    source_level               TEXT NOT NULL,
    sort_order                 INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_ptlm_level ON published_table_level_mapping(published_table_level_id, sort_order);
CREATE INDEX idx_ptlm_source ON published_table_level_mapping(source_table_id);
```

`schema_version=2` への上書きを `migrations.go` 末尾に追加。冪等性のため schema_version の現在値を確認してから DROP/CREATE を行う（`schema_version=1` から上がるときのみ実行）。

## 5. ピックアルゴリズム

### 5.1 Weighter インターフェース

```go
// internal/port/weighter.go (新規)
type Weighter interface {
    // Weight は譜面に対する重みを返す。0 以下を返した場合はピック対象外（ただし MVP の UniformWeighter は常に 1 を返す）。
    Weight(ctx context.Context, ch domain.EnrichedChart, now time.Time) float64
}
```

`UniformWeighter` は常に 1 を返す。将来 `LastPlayedWeighter`（`scorelog.db` を参照して経過時間で重みを返す）を追加できる。`Bootstrap` 時に注入。

### 5.2 ピックフロー

各公開レベルごとに以下を独立実行する。シードは公開表 ID + 公開レベル ID + RefreshMode 由来キーから決定（既存 `makeSeed` を流用しつつ公開レベル ID を混ぜる）。

```
入力: level (PublishedTableLevel), Weighter, rng, OwnedOnly
出力: picked []EnrichedChart, levelOrderEntry string

1. プール構築
   - mappings の各 (source_table_id, source_level) ごとに
     LoadCharts(source_table_id, ChartQuery{OwnedOnly}) し、source_level に一致する譜面を収集
   - mapping ごとの局所プール pool[i] を作る
   - 公開レベル全体の合体プール unionPool を「mapping を SortOrder 昇順で走査し、
     既に unionPool にあるもの（dedup キー一致）を除いて追加」で作る
   - dedup キー: md5 が非空なら md5、空なら sha256（既存リポジトリの慣例）

2. フェーズ 1: 各 mapping から m 曲ずつ
   m := level.PerMappingPick
   for i, mapping := range mappings (SortOrder 昇順):
       avail := pool[i] から「すでに picked に含まれる譜面（md5 主・sha256 フォールバックで一致）」を除外
       Weighter で min(m, len(avail)) 曲を重み付き加重サンプリング（重み 0 以下は対象外）
       picked に追加（mapping_index = i も合わせて記録）
   # mapping は SortOrder 昇順固定で走査することで rng 消費順を決定論化

3. フェーズ 2: 全体から合計 n 曲になるよう追加ピック
   need := level.TotalPick - len(picked)
   if need > 0:
       remainingUnion := unionPool から picked を除外
       Weighter で min(need, len(remainingUnion)) 曲を追加サンプリング
       picked に追加（mapping_index = -1 として記録）
   # need <= 0（n = 0 含む / sum(m_picked) >= n 含む）の場合はフェーズ 2 をスキップ

4. 出力: picked を (mapping_index 昇順, Position 昇順) の安定整列で並べて levelOrderEntry = level.Name とともに返す
   # フェーズ 2 で追加されたものは mapping_index = -1 で先頭にしたくないので、
   # 実装上は (フェーズ番号, mapping_index, Position) の安定整列にする
   # → フェーズ 1 のマッピング順 → フェーズ 2 追加分の順、各群内は Position 昇順
```

**dedup の主キー**: 上記 5.2-1 と同じ（md5 が非空なら md5、空なら sha256）。フェーズ 1 / フェーズ 2 共通。

**シードと決定論性**: フェーズ 1 のマッピング走査順、フェーズ 2 のサンプリングはすべて `rand.Rand` 1 個から消費する。`refresh_mode=daily` で同一日内に再起動しても同じ結果になるよう、`level.ID` をシードのキーに加える。

**重み付き加重サンプリング**: O(n) の Walker's alias 法ではなく、ピック数が小さい想定なので「累積重みからの逐次選択 + 削除」のシンプル実装で良い（`UniformWeighter` のときは現状の Fisher-Yates と等価になるよう、重み一様の経路を最適化）。

### 5.3 公開表全体の RefreshMode

公開表単位で 1 回の `regenerate` 内で全公開レベルを順に処理し、それぞれの結果を `domain.PickResult` に集約する。`SeedKey` は公開表全体（既存通り日付 / nano）。レベル別シードは `seed XOR fnv(level.ID)` で導出。

## 6. UseCase / Repository 変更

### 6.1 PublishedTableRepo（拡張）

```go
type PublishedTableRepo interface {
    // 既存
    List(ctx context.Context) ([]domain.PublishedTable, error) // Levels/Mappings は含めない（一覧用、軽量）
    Get(ctx context.Context, id string) (domain.PublishedTable, error) // Levels/Mappings 込み
    GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error) // 同上
    Create(ctx context.Context, t domain.PublishedTable) (string, error) // Levels/Mappings 込みで一括 INSERT (1 トランザクション)
    Update(ctx context.Context, t domain.PublishedTable) error // Levels/Mappings 込み。子テーブル全削除 → 再 INSERT で実装
    Delete(ctx context.Context, id string) error // CASCADE で子テーブルも消える
    SlugExists(ctx context.Context, slug string, excludeID string) (bool, error)
}
```

`Update` は子テーブル全削除 → 再 INSERT で実装（公開表編集はバッチ的、レコード数も小さい）。

### 6.2 PublishedTableUseCase（拡張）

```go
// 既存メソッドは維持しつつ
// 新規:
func (u *PublishedTableUseCase) CreateFromSourceTable(ctx context.Context, sourceTableID, slug, displayName, symbol string) (string, error)
// → 指定ソース表の LevelOrder を読み、各レベル毎に公開レベル 1 つ + マッピング 1 つ（同 source/同 level、m=0/n=0）を生成してウィザード相当の公開表を作る
func (u *PublishedTableUseCase) ApplyBulkPickConfig(ctx context.Context, publishedTableID string, perMappingPick, totalPick int) error
// → 当該公開表の全 PublishedTableLevel に (m, n) を一括上書き
```

`Create` は通常通り `Levels` を持った値オブジェクトを受けて全件挿入。

### 6.3 PickUseCase（書き換え）

`pick_usecase.go` の `regenerate` を以下に置換:

```
1. pub := pubRepo.Get(ctx, id)（Levels/Mappings 込み）
2. 関与する全 source_table_id を集めて一括で srcRepo.LoadCharts を実行（重複読み回避のためキャッシュ）
3. 公開レベルごとに 5.2 のフローを Weighter 注入で実行
4. 結果を domain.PickResult に集約（LevelOrder = pub.Levels.Name の順）
```

`Weighter` は `Bootstrap` で注入される（MVP では `UniformWeighter`）。

### 6.4 SourceTableRepo.LoadCharts の引数

現状は `port.ChartQuery{OwnedOnly}` のみだが、レベルでフィルタする SQL を追加するか、メモリで絞るかを選ぶ必要がある。**メモリで絞る**（現状動作と同じく source 単位で全件取り、ピック側で `level == sourceLevel` のものを抽出）。これは既存の SQL クエリを変更しない方針。性能上問題が出たら後日 `ChartQuery.Levels []string` を追加する。

## 7. HTTP 出力

### 7.1 `/{slug}/header.json`

```json
{
  "name": "<DisplayName>",
  "symbol": "<Symbol>",
  "data_url": "/{slug}/data.json",
  "level_order": ["<Level1.Name>", "<Level2.Name>", ...]
}
```

`level_order` は公開レベルの `Name` 順（`SortOrder` 昇順）。

### 7.2 `/{slug}/data.json`

各譜面の `level` フィールドを **公開レベル名** で上書きして出力する（ソース表側の元レベルではなく）。`Raw` パススルー時に `level` キーを公開レベル名で上書き。`symbol`（per-chart）は **ソース表側の symbol** をそのまま流す（既存仕様維持）。

```json
[
  { "md5": "...", "sha256": "...", "title": "...", "artist": "...",
    "level": "<公開レベル名>", "symbol": "<ソース表 symbol>", ... },
  ...
]
```

### 7.3 公開表 HTML（`/{slug}` index.html）

直近の `2026-05-09-public-table-html-redesign-design.md` の構造を維持。レベル見出しは公開レベル名、行頭セルの `{Symbol}{Level}` は公開レベル名 + per-chart symbol（ソース由来）の合成。

## 8. UI（Svelte / 公開表編集）

### 8.1 作成導線

公開表新規作成ボタン → ダイアログ:

```
公開表の作成方法を選んでください:
  ( ) ソース表からウィザード生成（推奨）
      ソース表選択 ▼ [stella]
      → 選んだソース表の各レベルが公開レベルとして自動生成されます
  ( ) ブランクから作成
      → 公開レベルとマッピングを手で組み立てます
[キャンセル] [次へ]
```

ウィザード生成は `CreateFromSourceTable` 経由。ブランクは `slug/displayName/symbol` のみ入力 → `Levels=[]` の公開表を作る。

### 8.2 公開表詳細ページ

公開表ごとの編集ページに以下を配置:

```
[公開表メタ情報パネル] DisplayName / Slug / Symbol / OwnedOnly / RefreshMode

[全レベル一括適用パネル]
  m: [  ] / n: [  ]  [全レベルに適用]

[公開レベル一覧テーブル]
  | 並び | 公開レベル名 | マッピング (chip 列挙)              | m  | n  | [操作] |
  | ▲▼   | "5"          | [stella★5 x] [satellite sl5 x] [+]  | 2  | 5  | [削除]  |
  | ▲▼   | "5-6"        | [stella★5 x] [stella★6 x] [+]       | 1  | 4  | [削除]  |
  ...
  [+ 公開レベル追加]
```

- 公開レベルの並び替えは `▲▼` ボタンで `SortOrder` を入れ替え（drag-and-drop は v3）
- マッピング追加は chip 横の `[+]` → 「ソース表ドロップダウン → ソースレベルドロップダウン」モーダル
- マッピング削除は chip の `x`
- バルク適用ボタンで全レベルの `(m, n)` を上書き（一回限り、確認ダイアログ）
- 同一 `(source, source_level)` を同一公開レベル内で重複追加するのはバリデーションでブロック（ただし異なる公開レベル間では許容）

### 8.3 既存タブとの関係

- `PublishedTablesTab.svelte`（一覧 + 各行リンク）はメタ情報のみ表示で十分
- 編集は別画面 or モーダル展開
- 「ピック設定 PerLevel」を表示していた箇所は、新モデルでは「公開レベル数」だけ表示する形に縮約

## 9. バリデーション

| 対象                              | ルール                                              | 違反時 |
|-----------------------------------|-----------------------------------------------------|--------|
| `Slug`                            | 既存と同じ（UNIQUE / 半角英数記号）                 | エラー |
| `Level.Name`                      | 同一公開表内で重複不可                              | エラー |
| `Level.PerMappingPick (m)`        | `>= 0`                                              | エラー |
| `Level.TotalPick (n)`             | `>= 0`                                              | エラー |
| `Mapping (source_table, source_level)` | 同一公開レベル内で重複不可                     | エラー |
| `m * len(Mappings) > n` のレベル  | 警告（保存はブロックしない）                        | 警告  |
| `Mapping` 0 件のレベル            | 警告（ピック結果が空になる）                        | 警告  |

## 10. テスト方針

### Go ユニットテスト

- `published_table_repo_test.go`: Levels/Mappings を含む CRUD、`Update` での子テーブル全置換、CASCADE 削除
- `migrations_test.go`: `schema_version=1 → 2` 遷移で旧 `published_table` 行が消え、新スキーマで起動できる
- `published_table_usecase_test.go`:
  - `CreateFromSourceTable`: ソース表 LevelOrder から正しく公開レベル列を生成
  - `ApplyBulkPickConfig`: 全レベルの `(m, n)` が一括上書き
  - バリデーションエラー（重複レベル名 / 重複マッピング）
- `pick_usecase_test.go`:
  - フェーズ 1 のみ（n=0）
  - フェーズ 1 + フェーズ 2（sum(m) < n、追加ピックで合計 n に揃う）
  - sum(m) >= n（フェーズ 2 スキップ、合計 sum(m_picked)）
  - dedup: 同一 MD5 が複数マッピングに含まれる → 1 回しかピックされない
  - プール不足: 各マッピングプール `< m` → 取れた分のみで継続、フェーズ 2 が補填可能なら補填
  - 決定論性: 同一シード（同一 RefreshMode / 同一日）で複数回呼んでも結果が一致
  - `Weighter` 差し替え: モック Weighter を注入して、重み 0 の譜面が選ばれないこと
- `weighter_test.go` (UniformWeighter): 全譜面に同一重み

### HTTP テスト

- `header.json`: `level_order` が公開レベル名順
- `data.json`: 各譜面の `level` が公開レベル名で上書きされる、`symbol` はソース由来

### フロントエンド

- 型チェック (`npm run check`) のみ。Svelte コンポーネントのユニットテストは現状方針に従い書かない。

## 11. マイグレーション方針（クリーンブレイク）

- `schema_version=1 → 2` 遷移時、旧 `published_table` のレコードは破棄される（DROP TABLE）
- 起動時のリリースノート / 設定ダイアログで「v0.x.0 で公開表データ構造が刷新されたため、既存の公開表は再作成が必要です」と告知
- ソース表データは無変更

## 12. 拡張ポイント: 最終プレイ日時優先

将来 `LastPlayedWeighter` を追加する際の見取り図:

1. `scorelog.db` から各譜面の最終プレイ時刻を取得する `port.LastPlayedSource` インターフェースを追加
2. `LastPlayedWeighter` を実装: `weight = base * f(now - lastPlayed)`（経過時間に対する単調増加関数、未プレイは最大重み）
3. `Bootstrap` で `Weighter` の注入を `UniformWeighter` から `LastPlayedWeighter` に切り替え（公開表設定で選択可能化は別フェーズ）

`PickUseCase` 本体・スキーマ・UI の変更は不要（このスペック内の `Weighter` 注入経路だけで完結する）。

## 13. 実装順の見立て

各ステップは TDD（先に赤テストを追加 → 実装 → 緑）。

1. ドメイン型 + 新スキーマ（migrations + 移行テスト）+ Repo 拡張（CRUD テスト）
2. PublishedTableUseCase 拡張（CreateFromSourceTable / ApplyBulkPickConfig + バリデーション）
3. `Weighter` インターフェース + UniformWeighter（単純な重み一様テスト）
4. PickUseCase 書き換え（5.2 のフロー、決定論性 / dedup / 不足時 / Weighter 差し替えのテスト網羅）
5. HTTP 出力の公開レベル名対応（header.json / data.json 出力テスト）
6. フロントエンド: 公開表編集画面 + 作成ダイアログ（型チェック通過 + 手動動作確認）
7. `wails generate module` でバインド再生成
8. マニュアル (`docs/manual.md`) 更新（新機能の操作手順 + 既存公開表データが破棄される旨）

## 14. 未解決事項 / 将来検討

- マッピングの並び替え（drag-and-drop UI）: 現状は `▲▼` ボタンで十分
- 公開レベル名の変更時、HTML キャッシュ等の整合性（現状 ETag 304 を本格運用していないので不要）
- ソース表削除時の挙動: `ON DELETE CASCADE` で関連マッピングも消える → 公開レベルが空になる可能性、UI で警告したい（v3 で検討）
- マッピングごとの `m` 個別指定（現状は公開レベル単位で共通の m）: 必要になったら `published_table_level_mapping` に `pick_count` カラムを足すだけで拡張可能
