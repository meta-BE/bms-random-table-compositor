# 最終プレイ日時優先ピック設計

- 作成日: 2026-05-11
- 関連: `docs/TODO.md` v2 機能「最終プレイ日時優先」
- 前提: v0.2.0 まで実装済み (複数ソース表合成 / Pick の RefreshMode / SongdataAttacher)

## 1. 概要とスコープ

公開表のピックに「最終プレイ日時で古い曲を優先する」重み付けを導入し、復習・捨て曲掘りの体験を改善する。

**3 モードを公開表ごとに選択可能とする**。既存の `PickConfig.RefreshMode` と並列の独立軸。

| Mode | 識別子 | 挙動 |
|---|---|---|
| OFF | `weight_off` | 一様ランダム (現状の `UniformWeighter` 相当) |
| 確率 (X 倍) | `weight_probability` | 重み付き非復元サンプリング。重み `w = 1 + (X-1)·a`、`a = (now-date)/maxAge` |
| 完全日時順ソート | `weight_sort` | 古い順に確定的に取る |

**非スコープ**:
- `scorelog.db` / `scoredatalog.db` の参照 (`score.db` で十分と判明済み)
- グローバルデフォルト値 (将来必要なら後付け)
- プレイ回数や難易度別の重み付け (別タスク)

## 2. 「最終プレイ日時」の取得元

### 2.1 DB 調査結果

| DB / テーブル | 役割 | 採用 |
|---|---|---|
| `score.db` / `score` (PK: `sha256, mode`) | 譜面ごとの最新統計。`date` は最終プレイ更新時刻 | ✅ |
| `scoredatalog.db` / `scoredatalog` | `score` の subset (2115/5057 行)。`date` 完全一致 | ❌ subset のため不要 |
| `scorelog.db` / `scorelog` | `clear/score/combo/minbp` のいずれか変化時のみ追記。「ベスト更新ログ」 | ❌ 最終プレイ日時としては過小 (例: 2022 vs 2026) |

### 2.2 取得クエリ (mode をまたいだ MAX)

```sql
(SELECT MAX(sc.date) FROM sc.score sc
  WHERE sc.sha256 = c.sha256 AND sc.date > 0) AS last_played_at
```

- `score` PK が `(sha256, mode)` のため、同一譜面を 7K/14K キーマップで遊んだケースでも最終時刻を統合
- `c.sha256 = ""` の難易度表行は LEFT 相当で NULL → 未プレイ扱い (`a = 1`)
- `date = 0` (古い beatoraja DB 等で日時欠落) は除外 → NULL → 未プレイ扱い (`a = 1`)

### 2.3 並行アクセス時の安全策

beatoraja 動作中の score.db アクセスは `SQLITE_BUSY` の可能性あり。

- attach 時 / 読み取り時に BUSY を受けたら 1 回再試行
- 失敗継続なら `last_played_at = NULL` (= 全曲未プレイ扱い → 確率モードは一様、ソートモードは `(mappingIdx, position)` 順) で続行
- これは既存の `SongdataAttacher` の「attach 失敗時は OwnedOnly=0 件で続行」と同じ精神

## 3. データソース構成

### 3.1 `ScoreDBAttacher` (新設)

`internal/adapter/persistence/score_attacher.go`

- メイン `*sql.DB` に `ATTACH DATABASE '<score_db_path>' AS sc` (RO モード)
- 既存メモリ方針 (`concurrent_db_access_policy`) に従い RO 専用 (RW は不要)
- ライフサイクル:
  - 起動時: `config.score_db_path` を読んで attach 試行 (失敗ログだけ残してアプリは起動継続)
  - `SetScoreDBPath` 経由のパス変更: 再 attach → `PickUseCase.InvalidateAll()` でピックキャッシュ破棄

### 3.2 `LoadCharts` SQL 拡張

`internal/adapter/persistence/source_table_repo.go` の `loadChartsAttached` を以下に拡張:

```sql
SELECT
  c.position, t.symbol, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
  EXISTS(SELECT 1 FROM sd.song s WHERE s.md5 = c.md5)               AS is_owned,
  (SELECT MAX(sc.date) FROM sc.score sc
    WHERE sc.sha256 = c.sha256 AND sc.date > 0)                     AS last_played_at
FROM source_table_chart c
JOIN source_table t ON t.id = c.source_id
WHERE c.source_id = ?
  AND (? = 0 OR EXISTS (SELECT 1 FROM sd.song s WHERE s.md5 = c.md5))
ORDER BY c.position ASC
```

- score.db が未 attach の場合は `sc.score` を参照する subquery を含めない別 SQL に切り替え (`scAttacher.IsAttached()` で分岐)
- `scanEnrichedRows` で `sql.NullInt64` → `time.Unix(v, 0).UTC()` のポインタ変換、NULL は `nil` のまま

## 4. ドメインモデル / ポート拡張

### 4.1 `domain` パッケージ

```go
// internal/domain/published_table.go

type WeightMode string
const (
    WeightModeOff         WeightMode = "off"
    WeightModeProbability WeightMode = "probability"
    WeightModeSort        WeightMode = "sort"
)

type PickConfig struct {
    RefreshMode  RefreshMode
    WeightMode   WeightMode // 既定 WeightModeOff
    WeightParamX int        // 既定 10 (probability モードでのみ意味を持つ)
}
```

`EnrichedChart.LastPlayedAt *time.Time` は既存の枠をそのまま使用。

### 4.2 `port` パッケージ

`port.Weighter` の責務を再定義 (破壊的変更):

```go
// 集合内で正規化された経過時間 a ∈ [0,1] (0=最新, 1=最古) を重みに変換する純関数。
// 0 以下を返した譜面は対象外。
type Weighter interface {
    Weight(a float64) float64
}

type WeighterFactory interface {
    For(cfg domain.PickConfig) Weighter
}
```

`PickUseCase` は `port.WeighterFactory` だけ依存し、公開表ごとに `For(pub.Pick)` で Weighter を選択する。

### 4.3 `weighter` adapter

`internal/adapter/weighter/`:

```go
// UniformWeighter: WeightMode=off / WeightMode=sort (経路で未使用) で使用
type UniformWeighter struct{}
func (UniformWeighter) Weight(_ float64) float64 { return 1 }

// LastPlayedWeighter: WeightMode=probability で使用
type LastPlayedWeighter struct{ X float64 }
func (w LastPlayedWeighter) Weight(a float64) float64 { return 1.0 + (w.X-1.0)*a }

// Factory: domain.PickConfig から適切な Weighter を選択
type Factory struct{}
func (Factory) For(cfg domain.PickConfig) port.Weighter {
    switch cfg.WeightMode {
    case domain.WeightModeProbability:
        x := float64(cfg.WeightParamX)
        if x < 1 { x = 1 }
        return LastPlayedWeighter{X: x}
    default:
        return UniformWeighter{}
    }
}
```

## 5. ピック計算ロジック (`pickLevel` 改修)

### 5.1 `a` の正規化 (公開レベル = unionPool スコープ)

```go
maxAgeSec := int64(0)
for _, c := range unionPool {
    if c.LastPlayedAt != nil {
        age := max(now.Unix()-c.LastPlayedAt.Unix(), 0)
        if age > maxAgeSec { maxAgeSec = age }
    }
}

aOf := func(c domain.EnrichedChart) float64 {
    if c.LastPlayedAt == nil { return 1.0 }    // 未プレイ = 最古扱い
    if maxAgeSec <= 0        { return 0.0 }    // プレイ済み全員同時刻 = 最新扱い
    age := max(now.Unix()-c.LastPlayedAt.Unix(), 0)
    return float64(age) / float64(maxAgeSec)
}
```

- phase1 / phase2 で `aOf` を共有 → 同じ曲は同重み
- `unionPool` 全員未プレイ (`maxAgeSec == 0` かつ全員 nil): 全員 `a = 1` → 重み均一 → 一様ランダムに退化
- 全員プレイ済みかつ同時刻 (`maxAgeSec == 0` かつ全員非 nil): 全員 `a = 0` → 重み均一

### 5.2 `WeightMode` 分岐

| Mode | phase1 (各マッピングから m 曲) | phase2 (union 残りから TotalPick まで) |
|---|---|---|
| `off` | `UniformWeighter` で重み付きサンプル | 同左 |
| `probability` | `LastPlayedWeighter{X}` でサンプル | 同左 |
| `sort` | マッピング pool を `(aOf 降順, mappingIdx 昇順, Position 昇順)` でソートし上から m 件 | unionPool 残りを同タイブレークで必要件数 |

既存の `weightedSampleWithoutReplacement` には `aOf func(EnrichedChart) float64` を引数追加し、内部で `w.Weight(aOf(c))` を呼ぶ。

### 5.3 `sort` モードのタイブレーク

1. `aOf` 降順 (大きい = 古い)
2. `mappingIdx` 昇順
3. `Position` 昇順

これで `date == 0` / `nil` の曲が大量にあるケース (`unionPool` 全員未プレイ等) でも確定的順序。

### 5.4 シード / キャッシュ

- `sort` モードでも既存の `RefreshMode` キャッシュ判定 (daily / manual) は機能継続
- `sort` は決定論なので seed 自体は未使用だが、コード簡単化のため既存の `baseSeed XOR fnv32(level.ID)` 計算は残す
- `manual` + `sort` の組み合わせ: 再ピックしても同結果になる (古い順は変わらない) → 仕様として許容

### 5.5 `PickUseCase` シグネチャ変更

```go
func NewPickUseCase(
    pubRepo port.PublishedTableRepo,
    srcRepo port.SourceTableRepo,
    store *PickResultStore,
    clock port.Clock,
    randNew port.RandSourceFactory,
    log *slog.Logger,
    weighterFactory port.WeighterFactory, // 旧 weighter port.Weighter
) *PickUseCase
```

Bootstrap (`internal/app/bootstrap.go`) で `weighter.Factory{}` を注入。

## 6. UI

### 6.1 公開表編集画面

既存の `RefreshMode` セレクタの近傍に追加:

```
ピック更新タイミング: [per_request | daily | manual]   ← 既存

最終プレイ日時優先:   [OFF | 確率 (X倍まで偏らせる) | 完全日時順ソート]
  └ X = [    ] (2〜10000 の自然数, 既定 10)   ← probability 選択時のみ表示
```

- バリデーション: フロントで `X ∈ [2, 10000]` の整数チェック
- 既定値: `WeightMode = off`, `WeightParamX = 10` (probability に切替時の出発点)
- X=1 を許さない理由: X=1 は OFF と完全に等価で UI が冗長になるため
- 上限 10000: シミュレーション結果 (X=100 で最古 ≈ 最新 100 倍) より、10000 で実質「最古優先の極端寄り」になる
- バックエンド側 (`ConfigHandler` / `PublishedTableHandler`) でも防御的に範囲チェックし、範囲外はエラー返却

### 6.2 設定画面 (score.db パス)

既存の songdata.db パス選択の**直下**に追加:

```
songdata.db パス: [/path/to/songdata.db]  [選択...]  [状態: ...]
score.db    パス: [/path/to/score.db]     [選択...]  [状態: ...]
```

- `ConfigHandler` に `GetScoreDBPath` / `SetScoreDBPath` / `PickScoreDB` (ファイルダイアログ) を追加 (songdata と同じ 3 点)
- 状態表示: 「接続中」「未接続 (パス未設定)」「未接続 (`<理由>`)」

## 7. 永続化

### 7.1 `config` テーブル

KV 構造なので INSERT/UPSERT で済む。スキーマ変更不要。

- 新キー: `score_db_path` (デフォルト空文字列)

### 7.2 `published_table` テーブル

`internal/adapter/persistence/migrations.go` に追加:

```sql
ALTER TABLE published_table ADD COLUMN weight_mode TEXT NOT NULL DEFAULT 'off';
ALTER TABLE published_table ADD COLUMN weight_param_x INTEGER NOT NULL DEFAULT 10;
```

既存 CLAUDE.md ルールに従い:
- `pragma_table_info` で対象カラム存在チェックして冪等化
- `migrations_test.go` に旧スキーマからの ALTER 適用テストを追加

## 8. テスト方針

1. **`weighter.LastPlayedWeighter` 単体**
   - `Weight(0) == 1`, `Weight(1) == X`, `Weight(0.5) == (1+X)/2` (X=2/10/100 で table-driven)

2. **`weighter.Factory` 単体**
   - `WeightMode=off` で `UniformWeighter` を返す
   - `WeightMode=probability, X=10` で `LastPlayedWeighter{X:10}`
   - `WeightMode=sort` で `UniformWeighter` (sort 経路から呼ばれない安全側)
   - `WeightParamX < 1` は内部で 1 に clamp

3. **`pickLevel` 統合テスト** (`pick_usecase_test.go`)
   - `WeightMode=off` で従来と同挙動 (既存テスト維持)
   - `WeightMode=probability` + X=10000 + seeded rng で「最古曲の選出確率が直近曲より圧倒的に高い」(シード固定の決定論アサート)
   - `WeightMode=sort` で `(date 降順, mappingIdx, position)` の確定順
   - 全曲未プレイ集合 → 全員 `a=1` → off と同等の一様ランダム
   - 一部未プレイ集合 → 未プレイ曲が最古プレイ曲と同重み

4. **`scanEnrichedRows` 単体**
   - `last_played_at` カラム から `*time.Time` への変換 (date=0 → nil, date>0 → `time.Unix(date, 0).UTC()`)

5. **`ScoreDBAttacher` 単体**
   - テスト用 score.db ファイルを使った attach 成否
   - `SetScoreDBPath` 経由の再 attach

6. **マイグレーション** (`migrations_test.go`)
   - 旧 schema (`weight_mode` / `weight_param_x` 列なし) からの ALTER 冪等性
   - 既存行に対する DEFAULT 値適用 (`off`, `10`)

7. **シミュレーション再現テスト** (testdata 依存)
   - 前提: **`OwnedOnly = true` の公開表で実行する** (シミュレーションスクリプトは songdata.db に md5 一致するもののみ抽出した条件と揃える)
   - sl0 (level=0) の所持・プレイ済み 170 曲集合で X=10 の確率分布が想定 (最古 ≈ 1.13%, 中央値 ≈ 0.66%, 最新 ≈ 0.11%) と一致することを assert
   - `testdata/songdata.db` / `testdata/score.db` がない環境では skip (既存 `songdata_reader_test.go` と同方針)

## 9. 影響範囲チェックリスト

- [ ] `internal/domain/published_table.go`: `WeightMode`, `PickConfig` 拡張
- [ ] `internal/port/weighter.go`: `Weighter` シグネチャ変更, `WeighterFactory` 新設
- [ ] `internal/adapter/weighter/`: `UniformWeighter` 改修, `LastPlayedWeighter` / `Factory` 新設
- [ ] `internal/adapter/persistence/score_attacher.go`: 新設
- [ ] `internal/adapter/persistence/source_table_repo.go`: `LoadCharts` SQL 拡張
- [ ] `internal/adapter/persistence/migrations.go`: `weight_mode` / `weight_param_x` 追加
- [ ] `internal/usecase/pick_usecase.go`: コンストラクタ変更, `pickLevel` 改修, `WeightMode` 分岐
- [ ] `internal/app/bootstrap.go`: `ScoreDBAttacher` 構築 / DI, `WeighterFactory` 注入
- [ ] `internal/app/handler/config_handler.go`: `GetScoreDBPath` / `SetScoreDBPath` / `PickScoreDB`
- [ ] `internal/app/handler/published_table_handler.go`: `WeightMode` / `WeightParamX` の送受信 (Update DTO)
- [ ] `frontend/`: 公開表編集画面 / 設定画面の UI 追加, フロントバリデーション
- [ ] `wails generate module` でフロント型再生成
- [ ] `docs/manual.md`: 新機能を追記

## 10. 既知のトレードオフ / 留意点

- **「最古からの確定 N 曲」と「X 巨大値の確率モード」は近似不能**: 古い曲群が同確率近くで散らばるため、確定順を望む場合は `weight_sort` を選ぶ必要がある。
- **未所持曲は `a=1` で大量に積まれる**: `OwnedOnly=false` の公開表では、所持曲のうち最古曲と未所持曲が同重み (`X`) になり、未所持曲がピックされやすくなる。これは「触ってない曲を引っ張り出す」の自然な拡張だが、ユーザーが混乱する可能性あり。マニュアルで明記。
- **beatoraja 並行稼働時の整合性**: score.db を RO で attach しているため、beatoraja 側の書き込みが SQLite のロック粒度に応じて待たされる可能性。BUSY 時のフォールバック (`last_played_at = NULL`) で安全側に倒す。
