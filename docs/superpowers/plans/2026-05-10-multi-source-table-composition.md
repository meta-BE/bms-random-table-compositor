# 複数ソース表合成（公開表レベルマッピング）実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 公開表 1 レベルに対して複数の `(ソース表, ソースレベル)` を紐付ける合成機能を導入し、ピック設定を「マッピング 1 件あたり最低 m 曲」+「公開レベル合計目標 n 曲」の二段ロジックに置き換える。最終プレイ日時優先ピックに備えて `Weighter` 拡張点も同時に導入する。既存公開表データはクリーンブレイクで破棄する方針。

**Architecture:** ドメイン層に `PublishedTableLevel` / `PublishedTableLevelMapping` を新設し、`PublishedTable.SourceTableID` / `Pick.PerLevel` / `Pick.PreferOldPlay` を撤去。永続化は `published_table_level` / `published_table_level_mapping` の 2 テーブルを追加し、`schema_version` を `2` に上げて旧 `published_table` を DROP/CREATE で作り直す。`PickUseCase` はフェーズ 1（マッピング毎に m 曲）+ フェーズ 2（合計が n になるよう全体プールから補填）の 2 段ピックに書き換え、`port.Weighter` インターフェース経由で重み付けを差し替え可能にする（MVP は `UniformWeighter`）。HTTP 出力は `header.json.level_order` と `data.json` の各譜面 `level` を公開レベル名で出力。フロントは公開表詳細編集画面に「公開レベル一覧テーブル + マッピング chip 編集 + 全レベル一括適用パネル」を実装し、新規作成は「ソース表からウィザード生成 / ブランク作成」の 2 導線をダイアログで分岐させる。

**Tech Stack:** Go 1.24 / Wails v2.11+ / `modernc.org/sqlite` / 標準 `net/http` / Svelte + TypeScript / Tailwind v4 + daisyUI v5（既存）

**設計ドキュメント:** `docs/superpowers/specs/2026-05-10-multi-source-table-composition-design.md`

**完了条件:**

- `schema_version=2` でマイグレーション完了、旧 `published_table` データは破棄、新 2 テーブルが作成される
- 公開表 CRUD で `Levels []PublishedTableLevel`（各レベルが `PerMappingPick`(m) / `TotalPick`(n) と `Mappings []PublishedTableLevelMapping` を持つ）が永続化される
- `PublishedTableUseCase.CreateFromSourceTable(sourceTableID, slug, displayName, symbol)` でソース表の `LevelOrder` から公開レベル＋マッピングを自動生成できる
- `PublishedTableUseCase.ApplyBulkPickConfig(publishedTableID, m, n)` で全公開レベルの `(m, n)` を一括上書きできる
- `port.Weighter` インターフェースが定義され、`UniformWeighter` が `Bootstrap` で注入されている
- `PickUseCase.regenerate` が公開レベルごとにフェーズ 1 + フェーズ 2 を実行し、dedup（md5 主・sha256 フォールバック）と決定論的シードが効く
- `header.json.level_order` が公開レベル名順、`data.json` の各譜面 `level` が公開レベル名で上書き出力される（`symbol` はソース由来 per-chart）
- 公開表編集画面で公開レベルの追加・削除・並び替え・名前編集・マッピング追加/削除が動作し、バルク適用ボタンで `(m, n)` が全レベルに反映される
- 新規作成ダイアログで「ソース表からウィザード生成 / ブランク作成」が選べ、それぞれ `CreateFromSourceTable` / 通常 `Create` を呼ぶ
- `go build ./...` / `go test ./...` 全 pass、`make lint` 通過、`cd frontend && npm run check` 通過
- マニュアル（`docs/manual.md`）に新仕様の操作手順が反映され、「v0.x.0 で公開表データが刷新されたため再作成必要」の告知が入っている
- `wails generate module` でフロントの bind 再生成済み

**スコープ外:**

- 既存公開表データのマイグレーション（クリーンブレイク方針、起動時に旧データは破棄）
- 最終プレイ日時優先ピックの実装本体（`Weighter` 拡張点の整備のみ）
- マッピング個別の `m` 指定（公開レベル単位で共通の m を維持）
- マッピングの drag-and-drop 並び替え（`▲▼` ボタンのみ）

**ブランチ運用:** 既存方針に従い main 上で直接コミット。完了時は `git push origin main` で remote 反映。

---

## ファイル構造（追加・変更）

新規作成:

```
internal/
├── domain/
│   └── published_table_level.go              # PublishedTableLevel + PublishedTableLevelMapping
├── port/
│   └── weighter.go                           # Weighter インターフェース
└── adapter/
    └── weighter/
        ├── uniform.go                        # UniformWeighter
        └── uniform_test.go

frontend/src/lib/
├── components/
│   └── PublishedTableLevelEditor.svelte      # 公開レベル一覧テーブル + マッピング chip 編集
└── tabs/
    └── PublishedTableEditor.svelte           # 公開表詳細編集モーダル/画面（新規 or 既存タブを拡張）
```

修正:

```
internal/
├── domain/
│   └── published_table.go                    # SourceTableID/Pick.PerLevel/Pick.PreferOldPlay 撤去 / Levels 追加
├── adapter/
│   └── persistence/
│       ├── migrations.go                     # schema_version=2、旧 published_table を DROP/CREATE、新 2 テーブル追加
│       ├── migrations_test.go                # v1→v2 遷移テスト
│       ├── published_table_repo.go           # Levels/Mappings 込みの CRUD（子テーブル全削除→再 INSERT パターン）
│       └── published_table_repo_test.go      # 新スキーマ対応テスト
├── port/
│   └── published_table_repo.go               # コメントのみ更新（インターフェース署名は不変）
├── usecase/
│   ├── published_table_usecase.go            # Input 構造体差し替え、CreateFromSourceTable / ApplyBulkPickConfig 追加
│   ├── published_table_usecase_test.go       # 新仕様
│   ├── pick_usecase.go                       # フェーズ 1+2 ロジック書き換え、Weighter 注入
│   └── pick_usecase_test.go                  # 新ロジックテスト網羅
├── adapter/httpserver/
│   ├── handler_data.go                       # data.json の level 上書き
│   ├── handler_data_test.go
│   ├── handler_header.go                     # 既存通り（result.LevelOrder を流すだけ）
│   └── handler_header_test.go
└── app/
    ├── bootstrap.go                          # Weighter 注入
    └── handler/
        ├── published_table_handler.go        # DTO 差し替え、CreateFromSource / BulkApply バインド追加
        └── published_table_handler_test.go

frontend/src/
├── lib/
│   ├── api.ts                                # PublishedTableDTO の Levels[] 追加、新 API メソッド
│   └── tabs/
│       └── PublishedTablesTab.svelte         # 一覧 + 作成導線ダイアログ + 編集モーダル呼び出し
└── wailsjs/                                  # `wails generate module` で再生成 (.gitignore 対象)

docs/manual.md                                # 公開表編集手順を新仕様に書き換え + 移行告知
docs/TODO.md                                  # 「複数ソース表合成」項目を完了マーク
```

---

## Task 1: 新ドメイン型を定義する

**Files:**
- Create: `internal/domain/published_table_level.go`

- [ ] **Step 1: ファイル新規作成**

```go
package domain

// PublishedTableLevel は公開表が持つ 1 つの公開レベル。
// PerMappingPick (m) は「各マッピングからの最低保証ピック数」、
// TotalPick (n) は「公開レベル全体の目標合計ピック数」。
// 詳細は docs/superpowers/specs/2026-05-10-multi-source-table-composition-design.md §3 参照。
type PublishedTableLevel struct {
	ID               string
	PublishedTableID string
	Name             string // 公開レベル表示名（例: "5", "Lv.5", "中級"）
	SortOrder        int
	PerMappingPick   int // m: 各マッピングからの最低保証ピック数 (>= 0)
	TotalPick        int // n: 公開レベル全体の目標合計ピック数 (>= 0)
	Mappings         []PublishedTableLevelMapping // SortOrder 昇順
}

// PublishedTableLevelMapping は公開レベルが参照する 1 件のソース表レベル。
type PublishedTableLevelMapping struct {
	ID                    string
	PublishedTableLevelID string
	SourceTableID         string
	SourceLevel           string // ソース表内のレベル文字列（例: "5", "★5"）
	SortOrder             int
}
```

- [ ] **Step 2: コンパイル確認**

Run: `go build ./internal/domain/...`
Expected: 成功

- [ ] **Step 3: コミット**

```bash
git add internal/domain/published_table_level.go
git commit -m "feat(domain): PublishedTableLevel / PublishedTableLevelMapping 型を追加"
```

---

## Task 2: PublishedTable 構造体を新仕様に差し替える

このタスク完了時点で repo / usecase / handler / pick_usecase は compile broken になる。後続タスクで順次修正する。

**Files:**
- Modify: `internal/domain/published_table.go`

- [ ] **Step 1: 既存の published_table.go を新仕様に書き換える**

`internal/domain/published_table.go` を以下に置き換える:

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
// PerLevel / PreferOldPlay は撤去（複数ソース表合成スペックで Levels[].PerMappingPick/TotalPick と Weighter に置き換わった）。
type PickConfig struct {
	RefreshMode RefreshMode // per_request / daily / manual
}

// PublishedTable はユーザーが公開する表。Levels に複数ソース表のレベルを合成して持つ。
type PublishedTable struct {
	ID          string
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	Pick        PickConfig
	SortOrder   int
	Levels      []PublishedTableLevel // SortOrder 昇順
}
```

- [ ] **Step 2: ドメインだけ単体でビルドして他層の破綻を確認**

Run: `go build ./internal/domain/...`
Expected: 成功

Run: `go build ./...`
Expected: **失敗**（repo / usecase / handler / pick_usecase が SourceTableID 等を参照しているため）。失敗箇所を控えておき、後続タスクで修正対象とする。

- [ ] **Step 3: コミット（compile 全体は赤の状態でコミット）**

```bash
git add internal/domain/published_table.go
git commit -m "refactor(domain): PublishedTable から SourceTableID / Pick.PerLevel / PreferOldPlay を撤去し Levels[] を追加 (WIP)"
```

---

## Task 3: schema_version=2 マイグレーションを書く

**Files:**
- Modify: `internal/adapter/persistence/migrations.go`
- Modify: `internal/adapter/persistence/migrations_test.go`

- [ ] **Step 1: 失敗するテストを追加**

`internal/adapter/persistence/migrations_test.go` の末尾に追加:

```go
func TestRunMigrations_UpgradeV1ToV2_DropsOldPublishedTableAndCreatesNewTables(t *testing.T) {
	db := openMemoryDB(t)
	defer db.Close()

	// 1. まず v1 相当のスキーマを直接作成する（旧 published_table カラム構成）
	_, err := db.Exec(`CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO config(key, value) VALUES('schema_version', '1')`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE source_table (id TEXT PRIMARY KEY, input_url TEXT NOT NULL, input_kind TEXT NOT NULL CHECK(input_kind IN ('html','header_json')), display_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', symbol TEXT NOT NULL DEFAULT '', level_order_json TEXT NOT NULL DEFAULT '[]', data_url TEXT NOT NULL DEFAULT '', etag TEXT NOT NULL DEFAULT '', last_fetched_at TEXT, last_fetch_status TEXT NOT NULL DEFAULT 'never' CHECK(last_fetch_status IN ('never','ok','error')), last_fetch_error TEXT NOT NULL DEFAULT '', sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE published_table (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL, symbol TEXT NOT NULL DEFAULT '', source_table_id TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE, owned_only INTEGER NOT NULL DEFAULT 0, pick_per_level INTEGER NOT NULL DEFAULT 0, pick_refresh_mode TEXT NOT NULL DEFAULT 'manual' CHECK(pick_refresh_mode IN ('per_request','daily','manual')), prefer_old_play INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)

	// ダミーデータ: 旧公開表 1 件
	_, err = db.Exec(`INSERT INTO source_table(id, input_url, input_kind) VALUES('s1', 'http://x', 'html')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO published_table(id, slug, display_name, source_table_id) VALUES('p1', 'old', 'Old', 's1')`)
	require.NoError(t, err)

	// 2. RunMigrations 実行
	require.NoError(t, RunMigrations(db))

	// 3. schema_version が "2" になっている
	var ver string
	require.NoError(t, db.QueryRow(`SELECT value FROM config WHERE key='schema_version'`).Scan(&ver))
	require.Equal(t, "2", ver)

	// 4. 旧 published_table データが消えている（クリーンブレイク）
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM published_table`).Scan(&n))
	require.Equal(t, 0, n)

	// 5. published_table に source_table_id / pick_per_level / prefer_old_play カラムが「ない」
	cols := tableColumns(t, db, "published_table")
	require.NotContains(t, cols, "source_table_id")
	require.NotContains(t, cols, "pick_per_level")
	require.NotContains(t, cols, "prefer_old_play")
	require.Contains(t, cols, "owned_only")
	require.Contains(t, cols, "pick_refresh_mode")

	// 6. 新テーブル published_table_level / published_table_level_mapping が存在する
	require.Contains(t, tableColumns(t, db, "published_table_level"),
		"per_mapping_pick")
	require.Contains(t, tableColumns(t, db, "published_table_level"),
		"total_pick")
	require.Contains(t, tableColumns(t, db, "published_table_level_mapping"),
		"source_level")
}

// tableColumns は対象テーブルのカラム名を返す。
func tableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		out = append(out, n)
	}
	return out
}
```

`tableColumns` ヘルパが既存 `migrations_test.go` 内に既にあれば二重定義を避ける（同名関数があれば再利用）。

- [ ] **Step 2: テスト実行 → 失敗を確認**

Run: `go test ./internal/adapter/persistence/... -run TestRunMigrations_UpgradeV1ToV2 -v`
Expected: FAIL（schema_version が "2" にならない / 新テーブル無し）

- [ ] **Step 3: migrations.go を更新**

`internal/adapter/persistence/migrations.go` を以下に置き換える:

```go
package persistence

import (
	"database/sql"
	"fmt"
)

// schemaVersion は現在のスキーマバージョン。スキーマ変更時にインクリメント。
const schemaVersion = "2"

// RunMigrations は compositor.db のスキーマを冪等に作成する。
// schema_version=1 から 2 へ上げるときは旧 published_table を DROP/CREATE する
// （複数ソース表合成スペックのクリーンブレイク方針）。
func RunMigrations(db *sql.DB) error {
	// config テーブルだけは先に確保
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("migration exec: %w", err)
	}

	// 現在の schema_version を取得（初回起動時は空）
	var current string
	if err := db.QueryRow(
		`SELECT value FROM config WHERE key='schema_version'`,
	).Scan(&current); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read schema_version: %w", err)
	}

	// source_table 系は v1 と同じ（変更なし）
	v1Statements := []string{
		`CREATE TABLE IF NOT EXISTS source_table (
			id                TEXT PRIMARY KEY,
			input_url         TEXT NOT NULL,
			input_kind        TEXT NOT NULL CHECK(input_kind IN ('html', 'header_json')),
			display_name      TEXT NOT NULL DEFAULT '',
			name              TEXT NOT NULL DEFAULT '',
			symbol            TEXT NOT NULL DEFAULT '',
			level_order_json  TEXT NOT NULL DEFAULT '[]',
			data_url          TEXT NOT NULL DEFAULT '',
			etag              TEXT NOT NULL DEFAULT '',
			last_fetched_at   TEXT,
			last_fetch_status TEXT NOT NULL DEFAULT 'never' CHECK(last_fetch_status IN ('never','ok','error')),
			last_fetch_error  TEXT NOT NULL DEFAULT '',
			sort_order        INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS source_table_chart (
			source_id  TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			position   INTEGER NOT NULL,
			md5        TEXT NOT NULL,
			sha256     TEXT NOT NULL DEFAULT '',
			level      TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			artist     TEXT NOT NULL DEFAULT '',
			raw_json   TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (source_id, position)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_stc_md5 ON source_table_chart(md5)`,
		`CREATE INDEX IF NOT EXISTS idx_stc_source_level ON source_table_chart(source_id, level)`,
	}
	for _, s := range v1Statements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// v1 -> v2 遷移: 旧 published_table を破棄
	if current == "1" {
		if _, err := db.Exec(`DROP TABLE IF EXISTS published_table`); err != nil {
			return fmt.Errorf("drop published_table: %w", err)
		}
	}

	// v2 スキーマ（初回起動も v1->v2 もここを通る）
	v2Statements := []string{
		`CREATE TABLE IF NOT EXISTS published_table (
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
		)`,
		`CREATE TABLE IF NOT EXISTS published_table_level (
			id                  TEXT PRIMARY KEY,
			published_table_id  TEXT NOT NULL REFERENCES published_table(id) ON DELETE CASCADE,
			name                TEXT NOT NULL,
			sort_order          INTEGER NOT NULL DEFAULT 0,
			per_mapping_pick    INTEGER NOT NULL DEFAULT 0,
			total_pick          INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ptl_table ON published_table_level(published_table_id, sort_order)`,
		`CREATE TABLE IF NOT EXISTS published_table_level_mapping (
			id                         TEXT PRIMARY KEY,
			published_table_level_id   TEXT NOT NULL REFERENCES published_table_level(id) ON DELETE CASCADE,
			source_table_id            TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			source_level               TEXT NOT NULL,
			sort_order                 INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ptlm_level ON published_table_level_mapping(published_table_level_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_ptlm_source ON published_table_level_mapping(source_table_id)`,
	}
	for _, s := range v2Statements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// schema_version を書き込む
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO config(key, value) VALUES('schema_version', ?)`,
		schemaVersion,
	); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: テスト実行 → 通過を確認**

Run: `go test ./internal/adapter/persistence/... -run TestRunMigrations -v`
Expected: PASS（既存 v1 テストと新 v2 テスト両方）

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/migrations.go internal/adapter/persistence/migrations_test.go
git commit -m "feat(persistence): schema_version=2 マイグレーション。published_table を再作成し level/mapping テーブルを追加"
```

---

## Task 4: PublishedTableRepoSQL を Levels/Mappings 込みに書き換える

**Files:**
- Modify: `internal/adapter/persistence/published_table_repo.go`
- Modify: `internal/adapter/persistence/published_table_repo_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/persistence/published_table_repo_test.go` を全置換するか、既存テストを下記内容に書き換え:

```go
package persistence

import (
	"context"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

// fixtureSourceTable は外部キー充足のため最小限の source_table 行を用意する。
func fixtureSourceTable(t *testing.T, db *PublishedTableRepoTestDB, id string) {
	t.Helper()
	_, err := db.raw.Exec(
		`INSERT INTO source_table(id, input_url, input_kind, last_fetch_status)
		 VALUES(?, ?, 'html', 'ok')`, id, "http://example.com/"+id)
	require.NoError(t, err)
}

// PublishedTableRepoTestDB は PublishedTableRepoSQL とその下の DB をまとめて持つテスト用の構造。
type PublishedTableRepoTestDB struct {
	repo *PublishedTableRepoSQL
	raw  *sql.DB
}

func newPublishedTableRepoTestDB(t *testing.T) *PublishedTableRepoTestDB {
	t.Helper()
	db := openMemoryDB(t)
	require.NoError(t, RunMigrations(db))
	return &PublishedTableRepoTestDB{
		repo: NewPublishedTableRepoSQL(db),
		raw:  db,
	}
}

func TestPublishedTableRepoSQL_CreateAndGet_RoundTripsLevelsAndMappings(t *testing.T) {
	d := newPublishedTableRepoTestDB(t)
	defer d.raw.Close()
	fixtureSourceTable(t, d, "src-A")
	fixtureSourceTable(t, d, "src-B")

	pub := domain.PublishedTable{
		ID: "pub-1", Slug: "lv5", DisplayName: "Mixed Lv5", Symbol: "★",
		OwnedOnly: true,
		Pick:      domain.PickConfig{RefreshMode: domain.RefreshModeDaily},
		SortOrder: 0,
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "5", SortOrder: 0,
				PerMappingPick: 2, TotalPick: 5,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "map-1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "5", SortOrder: 0},
					{ID: "map-2", PublishedTableLevelID: "lvl-1", SourceTableID: "src-B", SourceLevel: "5", SortOrder: 1},
				},
			},
			{
				ID: "lvl-2", PublishedTableID: "pub-1", Name: "5-6", SortOrder: 1,
				PerMappingPick: 1, TotalPick: 4,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "map-3", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "5", SortOrder: 0},
					{ID: "map-4", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "6", SortOrder: 1},
				},
			},
		},
	}

	id, err := d.repo.Create(context.Background(), pub)
	require.NoError(t, err)
	require.Equal(t, "pub-1", id)

	got, err := d.repo.Get(context.Background(), "pub-1")
	require.NoError(t, err)
	require.Equal(t, pub.Slug, got.Slug)
	require.Equal(t, pub.DisplayName, got.DisplayName)
	require.Equal(t, pub.Symbol, got.Symbol)
	require.Equal(t, pub.OwnedOnly, got.OwnedOnly)
	require.Equal(t, pub.Pick.RefreshMode, got.Pick.RefreshMode)
	require.Len(t, got.Levels, 2)
	require.Equal(t, "5", got.Levels[0].Name)
	require.Equal(t, 2, got.Levels[0].PerMappingPick)
	require.Equal(t, 5, got.Levels[0].TotalPick)
	require.Len(t, got.Levels[0].Mappings, 2)
	require.Equal(t, "src-A", got.Levels[0].Mappings[0].SourceTableID)
	require.Equal(t, "5", got.Levels[0].Mappings[0].SourceLevel)
	require.Equal(t, "5-6", got.Levels[1].Name)
	require.Len(t, got.Levels[1].Mappings, 2)
}

func TestPublishedTableRepoSQL_Update_ReplacesAllLevelsAndMappings(t *testing.T) {
	d := newPublishedTableRepoTestDB(t)
	defer d.raw.Close()
	fixtureSourceTable(t, d, "src-A")

	initial := domain.PublishedTable{
		ID: "pub-1", Slug: "tbl", DisplayName: "T",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "old", SortOrder: 0,
				PerMappingPick: 1, TotalPick: 1,
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "m1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "old"},
				},
			},
		},
	}
	_, err := d.repo.Create(context.Background(), initial)
	require.NoError(t, err)

	updated := initial
	updated.DisplayName = "T2"
	updated.Levels = []domain.PublishedTableLevel{
		{
			ID: "lvl-2", PublishedTableID: "pub-1", Name: "new", SortOrder: 0,
			PerMappingPick: 3, TotalPick: 7,
			Mappings: []domain.PublishedTableLevelMapping{
				{ID: "m2", PublishedTableLevelID: "lvl-2", SourceTableID: "src-A", SourceLevel: "new"},
			},
		},
	}
	require.NoError(t, d.repo.Update(context.Background(), updated))

	got, err := d.repo.Get(context.Background(), "pub-1")
	require.NoError(t, err)
	require.Equal(t, "T2", got.DisplayName)
	require.Len(t, got.Levels, 1)
	require.Equal(t, "new", got.Levels[0].Name)
	require.Equal(t, "lvl-2", got.Levels[0].ID)
	require.Equal(t, 3, got.Levels[0].PerMappingPick)
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "m2", got.Levels[0].Mappings[0].ID)
}

func TestPublishedTableRepoSQL_List_DoesNotEagerLoadLevelsForListView(t *testing.T) {
	// List は一覧用なので Levels は空のままで良い（軽量）
	d := newPublishedTableRepoTestDB(t)
	defer d.raw.Close()
	fixtureSourceTable(t, d, "src-A")

	pub := domain.PublishedTable{
		ID: "pub-1", Slug: "x", DisplayName: "X",
		Pick:   domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{{ID: "lvl-1", PublishedTableID: "pub-1", Name: "5"}},
	}
	_, err := d.repo.Create(context.Background(), pub)
	require.NoError(t, err)

	list, err := d.repo.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "X", list[0].DisplayName)
	require.Empty(t, list[0].Levels) // List は Levels を埋めない
}

func TestPublishedTableRepoSQL_Delete_CascadesToLevelsAndMappings(t *testing.T) {
	d := newPublishedTableRepoTestDB(t)
	defer d.raw.Close()
	fixtureSourceTable(t, d, "src-A")

	pub := domain.PublishedTable{
		ID: "pub-1", Slug: "x", DisplayName: "X",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
		Levels: []domain.PublishedTableLevel{
			{
				ID: "lvl-1", PublishedTableID: "pub-1", Name: "5",
				Mappings: []domain.PublishedTableLevelMapping{
					{ID: "m1", PublishedTableLevelID: "lvl-1", SourceTableID: "src-A", SourceLevel: "5"},
				},
			},
		},
	}
	_, err := d.repo.Create(context.Background(), pub)
	require.NoError(t, err)

	require.NoError(t, d.repo.Delete(context.Background(), "pub-1"))

	var lvlCount, mapCount int
	require.NoError(t, d.raw.QueryRow(`SELECT COUNT(*) FROM published_table_level`).Scan(&lvlCount))
	require.NoError(t, d.raw.QueryRow(`SELECT COUNT(*) FROM published_table_level_mapping`).Scan(&mapCount))
	require.Equal(t, 0, lvlCount)
	require.Equal(t, 0, mapCount)
}
```

注: `openMemoryDB(t)` が既存ヘルパとして migrations_test.go か db_test.go にある前提（無ければ作る: `func openMemoryDB(t *testing.T) *sql.DB { t.Helper(); db, err := OpenDB(":memory:"); require.NoError(t, err); return db }`）。

- [ ] **Step 2: テスト実行 → 失敗を確認**

Run: `go test ./internal/adapter/persistence/... -run TestPublishedTableRepoSQL -v`
Expected: 失敗（コンパイルエラーまたは既存実装が新仕様に合っていない）

- [ ] **Step 3: published_table_repo.go を書き換える**

`internal/adapter/persistence/published_table_repo.go` を以下に置き換える:

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

// PublishedTableRepoSQL は published_table / _level / _level_mapping を一括で扱う実装。
type PublishedTableRepoSQL struct {
	db *sql.DB
}

func NewPublishedTableRepoSQL(db *sql.DB) *PublishedTableRepoSQL {
	return &PublishedTableRepoSQL{db: db}
}

const publishedTableSelectColumns = `SELECT
	id, slug, display_name, symbol, owned_only,
	pick_refresh_mode, sort_order
 FROM published_table`

func (r *PublishedTableRepoSQL) scanRow(s rowScanner) (domain.PublishedTable, error) {
	var (
		t         domain.PublishedTable
		ownedOnly int
		mode      string
	)
	if err := s.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.Symbol, &ownedOnly,
		&mode, &t.SortOrder,
	); err != nil {
		return domain.PublishedTable{}, err
	}
	t.OwnedOnly = ownedOnly != 0
	t.Pick.RefreshMode = domain.RefreshMode(mode)
	return t, nil
}

// Create は PublishedTable を Levels/Mappings 込みで一括 INSERT する（1 トランザクション）。
func (r *PublishedTableRepoSQL) Create(ctx context.Context, t domain.PublishedTable) (string, error) {
	if t.ID == "" {
		return "", errors.New("ID は必須です")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO published_table
		 (id, slug, display_name, symbol, owned_only, pick_refresh_mode, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Slug, t.DisplayName, t.Symbol, owned, string(t.Pick.RefreshMode), t.SortOrder,
	); err != nil {
		if isUniqueSlugViolation(err) {
			return "", fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return "", fmt.Errorf("insert published_table %q: %w", t.ID, err)
	}
	if err := r.insertLevels(ctx, tx, t.ID, t.Levels); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return t.ID, nil
}

// insertLevels は published_table_id 配下の levels と mappings をまとめて INSERT する。
func (r *PublishedTableRepoSQL) insertLevels(ctx context.Context, tx *sql.Tx, pubID string, levels []domain.PublishedTableLevel) error {
	for _, lv := range levels {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO published_table_level
			 (id, published_table_id, name, sort_order, per_mapping_pick, total_pick)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			lv.ID, pubID, lv.Name, lv.SortOrder, lv.PerMappingPick, lv.TotalPick,
		); err != nil {
			return fmt.Errorf("insert level %q: %w", lv.ID, err)
		}
		for _, mp := range lv.Mappings {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO published_table_level_mapping
				 (id, published_table_level_id, source_table_id, source_level, sort_order)
				 VALUES (?, ?, ?, ?, ?)`,
				mp.ID, lv.ID, mp.SourceTableID, mp.SourceLevel, mp.SortOrder,
			); err != nil {
				return fmt.Errorf("insert mapping %q: %w", mp.ID, err)
			}
		}
	}
	return nil
}

func isUniqueSlugViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "published_table.slug")
}

// Get は ID で取得する。Levels/Mappings も同時に読む。
func (r *PublishedTableRepoSQL) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE id = ?`, id)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table %q: %w", id, err)
	}
	if err := r.loadLevels(ctx, &t); err != nil {
		return domain.PublishedTable{}, err
	}
	return t, nil
}

func (r *PublishedTableRepoSQL) GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE slug = ?`, slug)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table by slug %q: %w", slug, err)
	}
	if err := r.loadLevels(ctx, &t); err != nil {
		return domain.PublishedTable{}, err
	}
	return t, nil
}

// loadLevels は対象公開表の levels と mappings を 2 クエリで取得して結合する。
func (r *PublishedTableRepoSQL) loadLevels(ctx context.Context, t *domain.PublishedTable) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, published_table_id, name, sort_order, per_mapping_pick, total_pick
		 FROM published_table_level
		 WHERE published_table_id = ?
		 ORDER BY sort_order ASC, id ASC`, t.ID)
	if err != nil {
		return fmt.Errorf("load levels: %w", err)
	}
	defer rows.Close()
	var levels []domain.PublishedTableLevel
	idx := map[string]int{}
	for rows.Next() {
		var lv domain.PublishedTableLevel
		if err := rows.Scan(&lv.ID, &lv.PublishedTableID, &lv.Name, &lv.SortOrder, &lv.PerMappingPick, &lv.TotalPick); err != nil {
			return fmt.Errorf("scan level: %w", err)
		}
		idx[lv.ID] = len(levels)
		levels = append(levels, lv)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(levels) == 0 {
		t.Levels = nil
		return nil
	}

	mrows, err := r.db.QueryContext(ctx,
		`SELECT m.id, m.published_table_level_id, m.source_table_id, m.source_level, m.sort_order
		 FROM published_table_level_mapping m
		 JOIN published_table_level l ON l.id = m.published_table_level_id
		 WHERE l.published_table_id = ?
		 ORDER BY m.sort_order ASC, m.id ASC`, t.ID)
	if err != nil {
		return fmt.Errorf("load mappings: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var mp domain.PublishedTableLevelMapping
		if err := mrows.Scan(&mp.ID, &mp.PublishedTableLevelID, &mp.SourceTableID, &mp.SourceLevel, &mp.SortOrder); err != nil {
			return fmt.Errorf("scan mapping: %w", err)
		}
		i, ok := idx[mp.PublishedTableLevelID]
		if !ok {
			continue
		}
		levels[i].Mappings = append(levels[i].Mappings, mp)
	}
	if err := mrows.Err(); err != nil {
		return err
	}
	t.Levels = levels
	return nil
}

// List は一覧用。Levels は埋めない（軽量）。
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

// Update は子テーブル全削除 → 再 INSERT で行う（バッチ的、レコード数も小さい）。
func (r *PublishedTableRepoSQL) Update(ctx context.Context, t domain.PublishedTable) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE published_table SET
		   slug=?, display_name=?, symbol=?, owned_only=?,
		   pick_refresh_mode=?, sort_order=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.Slug, t.DisplayName, t.Symbol, owned,
		string(t.Pick.RefreshMode), t.SortOrder, t.ID,
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

	// 子テーブルを全削除して再 INSERT
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM published_table_level WHERE published_table_id = ?`, t.ID); err != nil {
		return fmt.Errorf("delete levels: %w", err)
	}
	// mapping は ON DELETE CASCADE で連鎖削除される
	if err := r.insertLevels(ctx, tx, t.ID, t.Levels); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Delete は ID で削除する（Levels/Mappings は CASCADE で連鎖削除）。冪等。
func (r *PublishedTableRepoSQL) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM published_table WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete published_table %q: %w", id, err)
	}
	return nil
}

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

- [ ] **Step 4: テスト実行 → 通過を確認**

Run: `go test ./internal/adapter/persistence/... -v`
Expected: PASS（このパッケージは全 pass）。他パッケージは Task 5 以降で対応するため、`./...` は失敗のままで良い。

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/published_table_repo.go internal/adapter/persistence/published_table_repo_test.go
git commit -m "feat(persistence): PublishedTableRepoSQL を Levels/Mappings 込みの CRUD に書き換え"
```

---

## Task 5: PublishedTableUseCase を新仕様に書き換え + 新メソッドを追加する

**Files:**
- Modify: `internal/usecase/published_table_usecase.go`
- Modify: `internal/usecase/published_table_usecase_test.go`
- Modify: `internal/usecase/errors.go`（新エラー sentinel が必要なら）

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/published_table_usecase_test.go` を新仕様に合わせて全面更新（既存テストは新 Input 構造体に合わせて書き換え）。新規追加するテストケース:

```go
func TestPublishedTableUseCase_CreateFromSourceTable_GeneratesLevelsAndMappings(t *testing.T) {
	uc, _, srcRepo := newPubUCFixture(t)
	srcRepo.put(domain.SourceTable{
		ID: "src-A", DisplayName: "Stella", Name: "stella", Symbol: "★",
		LevelOrder: []string{"0", "1", "2"},
		LastFetchStatus: domain.FetchStatusOK,
	})

	id, err := uc.CreateFromSourceTable(context.Background(), "src-A", "stella", "Stella Public", "★")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := uc.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "stella", got.Slug)
	require.Equal(t, "★", got.Symbol)
	require.Len(t, got.Levels, 3)
	require.Equal(t, "0", got.Levels[0].Name)
	require.Equal(t, 0, got.Levels[0].PerMappingPick)
	require.Equal(t, 0, got.Levels[0].TotalPick)
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "src-A", got.Levels[0].Mappings[0].SourceTableID)
	require.Equal(t, "0", got.Levels[0].Mappings[0].SourceLevel)
}

func TestPublishedTableUseCase_ApplyBulkPickConfig_OverwritesAllLevels(t *testing.T) {
	uc, pubRepo, srcRepo := newPubUCFixture(t)
	srcRepo.put(domain.SourceTable{
		ID: "src-A", LevelOrder: []string{"1", "2"}, LastFetchStatus: domain.FetchStatusOK,
	})
	id, err := uc.CreateFromSourceTable(context.Background(), "src-A", "stella", "S", "")
	require.NoError(t, err)

	require.NoError(t, uc.ApplyBulkPickConfig(context.Background(), id, 3, 7))

	got, err := pubRepo.Get(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, got.Levels, 2)
	for _, lv := range got.Levels {
		require.Equal(t, 3, lv.PerMappingPick)
		require.Equal(t, 7, lv.TotalPick)
	}
}

func TestPublishedTableUseCase_Create_ValidatesDuplicateLevelNames(t *testing.T) {
	uc, _, srcRepo := newPubUCFixture(t)
	srcRepo.put(domain.SourceTable{ID: "src-A", LastFetchStatus: domain.FetchStatusOK})

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "x", DisplayName: "X", RefreshMode: domain.RefreshModeManual,
		Levels: []usecase.PublishedTableLevelInput{
			{Name: "5", Mappings: []usecase.PublishedTableLevelMappingInput{{SourceTableID: "src-A", SourceLevel: "5"}}},
			{Name: "5", Mappings: []usecase.PublishedTableLevelMappingInput{{SourceTableID: "src-A", SourceLevel: "6"}}},
		},
	})
	require.ErrorIs(t, err, usecase.ErrDuplicateLevelName)
}

func TestPublishedTableUseCase_Create_ValidatesDuplicateMappingWithinLevel(t *testing.T) {
	uc, _, srcRepo := newPubUCFixture(t)
	srcRepo.put(domain.SourceTable{ID: "src-A", LastFetchStatus: domain.FetchStatusOK})

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "x", DisplayName: "X", RefreshMode: domain.RefreshModeManual,
		Levels: []usecase.PublishedTableLevelInput{{
			Name: "5",
			Mappings: []usecase.PublishedTableLevelMappingInput{
				{SourceTableID: "src-A", SourceLevel: "5"},
				{SourceTableID: "src-A", SourceLevel: "5"},
			},
		}},
	})
	require.ErrorIs(t, err, usecase.ErrDuplicateMapping)
}
```

`newPubUCFixture` は新規ヘルパとして既存ファイルの上部に配置:

```go
type fakePublishedRepoForUC struct {
	rows map[string]domain.PublishedTable
	idGen int
}

// 必要な PublishedTableRepo メソッドを実装（List/Get/GetBySlug/Create/Update/Delete/SlugExists）。
// 既存の pick_usecase_test.go や他テストの fake と統合できるならそちらを再利用。

func newPubUCFixture(t *testing.T) (*usecase.PublishedTableUseCase, *fakePublishedRepoForUC, *fakeSourceRepoForUC) { ... }
```

実装は短く保ち、既存 `fakePublishedRepo`（`pick_usecase_test.go` にあれば）を共有してもよい。

- [ ] **Step 2: テスト実行 → 失敗を確認**

Run: `go test ./internal/usecase/... -run TestPublishedTableUseCase -v`
Expected: コンパイルエラー or テスト失敗

- [ ] **Step 3: errors.go に新 sentinel を追加**

`internal/usecase/errors.go` に追加:

```go
var (
	ErrDuplicateLevelName = errors.New("公開レベル名が重複しています")
	ErrDuplicateMapping   = errors.New("同一公開レベル内でマッピングが重複しています")
	ErrEmptyMappings      = errors.New("公開レベルにマッピングがありません")
)
```

- [ ] **Step 4: published_table_usecase.go を新仕様に書き換える**

主要差分:
- `CreatePublishedTableInput` / `UpdatePublishedTableInput` に `Levels []PublishedTableLevelInput` を追加
- `SourceTableID` / `PickPerLevel` フィールドを撤去
- `Levels` 内のバリデーション（Name 重複、Mapping 重複、`PerMappingPick >= 0`、`TotalPick >= 0`、各 Mapping の `SourceTableID` 存在確認）
- `CreateFromSourceTable(ctx, sourceTableID, slug, displayName, symbol) (string, error)` を新設
- `ApplyBulkPickConfig(ctx, publishedTableID, m, n int) error` を新設

```go
type PublishedTableLevelInput struct {
	Name           string
	PerMappingPick int
	TotalPick      int
	Mappings       []PublishedTableLevelMappingInput
}

type PublishedTableLevelMappingInput struct {
	SourceTableID string
	SourceLevel   string
}

type CreatePublishedTableInput struct {
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	RefreshMode domain.RefreshMode
	Levels      []PublishedTableLevelInput
}

type UpdatePublishedTableInput struct {
	ID          string
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	RefreshMode domain.RefreshMode
	SortOrder   int
	Levels      []PublishedTableLevelInput
}

func (u *PublishedTableUseCase) Create(ctx context.Context, in CreatePublishedTableInput) (string, error) {
	if err := u.validateBasic(ctx, in.Slug, "", in.RefreshMode); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return "", errors.New("表示名は必須です")
	}
	levels, err := u.buildLevelsFromInput(ctx, in.Levels, "")
	if err != nil {
		return "", err
	}
	id := u.idGen.New()
	t := domain.PublishedTable{
		ID: id, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		OwnedOnly: in.OwnedOnly,
		Pick:      domain.PickConfig{RefreshMode: in.RefreshMode},
		Levels:    levels,
	}
	// levels 内の PublishedTableID をセット
	for i := range t.Levels {
		t.Levels[i].PublishedTableID = id
	}
	out, err := u.repo.Create(ctx, t)
	if err != nil {
		return "", err
	}
	u.log.Info("published table created", "id", out, "slug", in.Slug, "levels", len(levels))
	return out, nil
}

func (u *PublishedTableUseCase) Update(ctx context.Context, in UpdatePublishedTableInput) error {
	if in.ID == "" {
		return errors.New("ID は必須です")
	}
	if err := u.validateBasic(ctx, in.Slug, in.ID, in.RefreshMode); err != nil {
		return err
	}
	if strings.TrimSpace(in.DisplayName) == "" {
		return errors.New("表示名は必須です")
	}
	levels, err := u.buildLevelsFromInput(ctx, in.Levels, in.ID)
	if err != nil {
		return err
	}
	t := domain.PublishedTable{
		ID: in.ID, Slug: in.Slug, DisplayName: in.DisplayName, Symbol: in.Symbol,
		OwnedOnly: in.OwnedOnly,
		Pick:      domain.PickConfig{RefreshMode: in.RefreshMode},
		SortOrder: in.SortOrder,
		Levels:    levels,
	}
	for i := range t.Levels {
		t.Levels[i].PublishedTableID = in.ID
	}
	if err := u.repo.Update(ctx, t); err != nil {
		return err
	}
	u.log.Info("published table updated", "id", in.ID, "slug", in.Slug, "levels", len(levels))
	return nil
}

// CreateFromSourceTable はソース表の LevelOrder から公開レベル＋マッピングを自動生成する。
func (u *PublishedTableUseCase) CreateFromSourceTable(ctx context.Context, sourceTableID, slug, displayName, symbol string) (string, error) {
	src, err := u.srcRepo.Get(ctx, sourceTableID)
	if err != nil {
		return "", ErrSourceTableNotFound
	}
	levels := make([]PublishedTableLevelInput, 0, len(src.LevelOrder))
	for _, lvl := range src.LevelOrder {
		levels = append(levels, PublishedTableLevelInput{
			Name: lvl, PerMappingPick: 0, TotalPick: 0,
			Mappings: []PublishedTableLevelMappingInput{
				{SourceTableID: sourceTableID, SourceLevel: lvl},
			},
		})
	}
	return u.Create(ctx, CreatePublishedTableInput{
		Slug: slug, DisplayName: displayName, Symbol: symbol,
		RefreshMode: domain.RefreshModeManual,
		Levels:      levels,
	})
}

// ApplyBulkPickConfig は対象公開表の全公開レベルの (m, n) を一括上書きする。
func (u *PublishedTableUseCase) ApplyBulkPickConfig(ctx context.Context, id string, m, n int) error {
	if m < 0 || n < 0 {
		return ErrInvalidPickPerLevel
	}
	pub, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	for i := range pub.Levels {
		pub.Levels[i].PerMappingPick = m
		pub.Levels[i].TotalPick = n
	}
	if err := u.repo.Update(ctx, pub); err != nil {
		return err
	}
	u.log.Info("bulk pick config applied", "id", id, "m", m, "n", n)
	return nil
}

// buildLevelsFromInput は Input から domain 型への変換 + バリデーション。
// pubID が空のとき（Create 時）は ID を新規採番。
func (u *PublishedTableUseCase) buildLevelsFromInput(ctx context.Context, inputs []PublishedTableLevelInput, pubID string) ([]domain.PublishedTableLevel, error) {
	seenName := map[string]struct{}{}
	out := make([]domain.PublishedTableLevel, 0, len(inputs))
	for i, lin := range inputs {
		name := strings.TrimSpace(lin.Name)
		if name == "" {
			return nil, fmt.Errorf("公開レベル %d: 名前が空です", i+1)
		}
		if _, dup := seenName[name]; dup {
			return nil, ErrDuplicateLevelName
		}
		seenName[name] = struct{}{}
		if lin.PerMappingPick < 0 || lin.TotalPick < 0 {
			return nil, ErrInvalidPickPerLevel
		}
		// マッピング検証
		seenMap := map[string]struct{}{}
		ms := make([]domain.PublishedTableLevelMapping, 0, len(lin.Mappings))
		for j, mp := range lin.Mappings {
			if _, err := u.srcRepo.Get(ctx, mp.SourceTableID); err != nil {
				return nil, ErrSourceTableNotFound
			}
			key := mp.SourceTableID + "\x00" + mp.SourceLevel
			if _, dup := seenMap[key]; dup {
				return nil, ErrDuplicateMapping
			}
			seenMap[key] = struct{}{}
			ms = append(ms, domain.PublishedTableLevelMapping{
				ID:                    u.idGen.New(),
				PublishedTableLevelID: "", // Caller が後で埋める or buildLevels で既に紐付いている前提
				SourceTableID:         mp.SourceTableID,
				SourceLevel:           mp.SourceLevel,
				SortOrder:             j,
			})
		}
		lvlID := u.idGen.New()
		for k := range ms {
			ms[k].PublishedTableLevelID = lvlID
		}
		out = append(out, domain.PublishedTableLevel{
			ID:               lvlID,
			PublishedTableID: pubID, // Caller がさらに上書き可能
			Name:             name,
			SortOrder:        i,
			PerMappingPick:   lin.PerMappingPick,
			TotalPick:        lin.TotalPick,
			Mappings:         ms,
		})
	}
	return out, nil
}

// validateBasic は slug / RefreshMode / 重複検査の共通部分。
func (u *PublishedTableUseCase) validateBasic(ctx context.Context, slug, excludeID string, mode domain.RefreshMode) error {
	if err := validateSlugFormat(slug); err != nil {
		return err
	}
	switch mode {
	case domain.RefreshModePerRequest, domain.RefreshModeDaily, domain.RefreshModeManual:
	default:
		return ErrInvalidRefreshMode
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
```

既存の `validateInput` メソッドは削除（`validateBasic` に統合）。

- [ ] **Step 5: テスト実行 → 通過を確認**

Run: `go test ./internal/usecase/... -run TestPublishedTableUseCase -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/usecase/published_table_usecase.go internal/usecase/published_table_usecase_test.go internal/usecase/errors.go
git commit -m "feat(usecase): PublishedTableUseCase を Levels/Mappings 仕様に書き換え。CreateFromSourceTable / ApplyBulkPickConfig 追加"
```

---

## Task 6: Weighter port + UniformWeighter を追加する

**Files:**
- Create: `internal/port/weighter.go`
- Create: `internal/adapter/weighter/uniform.go`
- Create: `internal/adapter/weighter/uniform_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/weighter/uniform_test.go`:

```go
package weighter

import (
	"context"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUniformWeighter_AlwaysReturnsOne(t *testing.T) {
	w := UniformWeighter{}
	now := time.Now()
	ctx := context.Background()

	chart := domain.EnrichedChart{
		SourceChart: domain.SourceChart{MD5: "x", Title: "T"},
	}
	require.Equal(t, 1.0, w.Weight(ctx, chart, now))
}
```

- [ ] **Step 2: テスト実行 → 失敗（パッケージ未作成）**

Run: `go test ./internal/adapter/weighter/... -v`
Expected: FAIL

- [ ] **Step 3: port/weighter.go を作成**

```go
// Package port 配下、weighter.go
package port

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// Weighter はピック時の重み関数。0 以下を返した譜面は対象外として扱う。
// 最終プレイ日時優先など将来の重み付けはこの差し替え点で実装する。
type Weighter interface {
	Weight(ctx context.Context, ch domain.EnrichedChart, now time.Time) float64
}
```

- [ ] **Step 4: adapter/weighter/uniform.go を作成**

```go
// Package weighter は port.Weighter の実装群。
package weighter

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// UniformWeighter は全譜面に等しく 1 を返す。MVP デフォルト。
type UniformWeighter struct{}

func (UniformWeighter) Weight(_ context.Context, _ domain.EnrichedChart, _ time.Time) float64 {
	return 1
}
```

- [ ] **Step 5: テスト実行 → 通過を確認**

Run: `go test ./internal/adapter/weighter/... -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/port/weighter.go internal/adapter/weighter/uniform.go internal/adapter/weighter/uniform_test.go
git commit -m "feat(port,adapter): Weighter インターフェース + UniformWeighter を追加"
```

---

## Task 7: PickUseCase をフェーズ 1+2 ロジックに書き換える

**Files:**
- Modify: `internal/usecase/pick_usecase.go`
- Modify: `internal/usecase/pick_usecase_test.go`

- [ ] **Step 1: 失敗するテストを書く**

既存の `pick_usecase_test.go` は旧仕様に依存しているので大幅に書き直す。フィクスチャは `EnrichedChart` を mapping (source, level) ごとに用意できる形に変える。新規テストケース（必須）:

```go
// テストヘルパ: 公開表（Levels/Mappings 込み）と source charts をセットアップ
type pickSetup struct {
	pub  domain.PublishedTable
	src  *fakeSourceRepo // mapping ごとに source charts を返す
}

func TestPickUseCase_PhaseOneOnly_NEqualsZero(t *testing.T) {
	f := newPickUCFixture(t)
	// 公開レベル "5" にマッピング 2 件 (src-A:5, src-B:5)、m=1, n=0
	// src-A:5 に 3 曲, src-B:5 に 2 曲
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 1, n: 0, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
			{srcID: "src-B", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5",
		fixChart("src-A", "5", 1, "A1"),
		fixChart("src-A", "5", 2, "A2"),
		fixChart("src-A", "5", 3, "A3"),
	)
	f.srcRepo.putCharts("src-B", "5",
		fixChart("src-B", "5", 1, "B1"),
		fixChart("src-B", "5", 2, "B2"),
	)

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	// フェーズ 2 スキップなので合計 2 曲 (m=1 × 2 mappings)
	require.Len(t, r.Charts, 2)
	require.Equal(t, []string{"5"}, r.LevelOrder)
}

func TestPickUseCase_PhaseTwoFillsToTotalN(t *testing.T) {
	// m=1, n=4, 2 mappings -> phase 1 で 2 曲、phase 2 で +2 曲 = 合計 4 曲
	f := newPickUCFixture(t)
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 1, n: 4, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
			{srcID: "src-B", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5",
		fixChart("src-A", "5", 1, "A1"), fixChart("src-A", "5", 2, "A2"),
		fixChart("src-A", "5", 3, "A3"), fixChart("src-A", "5", 4, "A4"),
	)
	f.srcRepo.putCharts("src-B", "5",
		fixChart("src-B", "5", 1, "B1"), fixChart("src-B", "5", 2, "B2"),
	)

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 4)
}

func TestPickUseCase_SumOfMExceedsN_PhaseTwoSkipped(t *testing.T) {
	// m=3, n=4, 2 mappings -> phase 1 で 6 曲（n を超過）、phase 2 はスキップ -> 合計 6 曲
	f := newPickUCFixture(t)
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 3, n: 4, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
			{srcID: "src-B", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5",
		fixChart("src-A", "5", 1, "A1"), fixChart("src-A", "5", 2, "A2"),
		fixChart("src-A", "5", 3, "A3"), fixChart("src-A", "5", 4, "A4"),
	)
	f.srcRepo.putCharts("src-B", "5",
		fixChart("src-B", "5", 1, "B1"), fixChart("src-B", "5", 2, "B2"),
		fixChart("src-B", "5", 3, "B3"), fixChart("src-B", "5", 4, "B4"),
	)

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 6)
}

func TestPickUseCase_Dedup_SameMD5InMultipleMappings(t *testing.T) {
	// 2 mappings 共通の MD5 "X" を持つ -> 結果は 1 回のみ
	f := newPickUCFixture(t)
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 1, n: 1, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
			{srcID: "src-B", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5", fixChart("src-A", "5", 1, "X"))
	f.srcRepo.putCharts("src-B", "5", fixChart("src-B", "5", 1, "X"))

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	// dedup 後にプール 1 曲、m=1 × 2 mappings だが 2 つ目は既選で残らず合計 1 曲
	require.Len(t, r.Charts, 1)
	require.Equal(t, "X", r.Charts[0].MD5)
}

func TestPickUseCase_InsufficientSupply_NoCompensation(t *testing.T) {
	// m=2 だが pool 1 曲しかない -> 取れた分のみ
	f := newPickUCFixture(t)
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 2, n: 0, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5", fixChart("src-A", "5", 1, "A1"))

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 1)
}

func TestPickUseCase_Deterministic_DailyMode(t *testing.T) {
	// 同一日内なら同じ結果になることを確認
	f := newPickUCFixture(t)
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 0, n: 2, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
		}},
	})
	pub.Pick.RefreshMode = domain.RefreshModeDaily
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5",
		fixChart("src-A", "5", 1, "A1"),
		fixChart("src-A", "5", 2, "A2"),
		fixChart("src-A", "5", 3, "A3"),
		fixChart("src-A", "5", 4, "A4"),
	)

	r1, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	f.uc.InvalidateAll() // キャッシュクリア
	r2, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Equal(t, md5sOf(r1.Charts), md5sOf(r2.Charts))
}

func TestPickUseCase_WeighterFiltersZeroWeights(t *testing.T) {
	// Weighter が "A1" に対して重み 0 を返す -> A1 は選ばれない
	f := newPickUCFixtureWithWeighter(t, zeroWeightFor("A1"))
	pub := buildPub("pub-1", []levelSpec{
		{name: "5", m: 0, n: 1, mappings: []mappingSpec{
			{srcID: "src-A", level: "5"},
		}},
	})
	f.pubRepo.put(pub)
	f.srcRepo.putCharts("src-A", "5",
		fixChart("src-A", "5", 1, "A1"),
		fixChart("src-A", "5", 2, "A2"),
	)

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 1)
	require.Equal(t, "A2", r.Charts[0].MD5)
}

// newPickUCFixtureWithWeighter は newPickUCFixture と同じだが Weighter を差し替える。
// 実装は newPickUCFixture をコピーして NewPickUseCase の最後の引数を変えるだけ。
// zeroWeightFor は指定 MD5 に対して 0、それ以外は 1 を返す Weighter を返すヘルパ。
```

ヘルパ関数 `buildPub`, `fixChart`, `md5sOf`, `zeroWeightFor` を同ファイル内に追加。`fakeSourceRepo` は `(sourceID, level)` ごとの charts を返すように `putCharts` を新設（既存 `LoadCharts` は OwnedOnly フィルタのみで、メモリで返す）。

- [ ] **Step 2: テスト実行 → 失敗を確認**

Run: `go test ./internal/usecase/... -run TestPickUseCase -v`
Expected: 失敗

- [ ] **Step 3: pick_usecase.go を書き換える**

主要変更:
1. `NewPickUseCase` に `Weighter` パラメータ追加（または functional option `WithWeighter`）。デフォルトは `weighter.UniformWeighter{}` を `Bootstrap` で渡す。
2. `regenerate` を以下のように書き換え:

```go
func (u *PickUseCase) regenerate(ctx context.Context, pub domain.PublishedTable) (domain.PickResult, error) {
	baseSeed, seedKey := u.makeSeed(pub)
	now := u.clock.Now()

	var finalCharts []domain.EnrichedChart
	var finalLevelOrder []string

	for _, lv := range pub.Levels {
		// レベル別シード: baseSeed XOR fnv(level.ID)
		levelSeed := baseSeed ^ int64(fnv32(lv.ID))
		rng := rand.New(u.randNew(levelSeed))

		picked, err := u.pickLevel(ctx, pub, lv, rng, now)
		if err != nil {
			return domain.PickResult{}, fmt.Errorf("pick level %q: %w", lv.Name, err)
		}
		if len(picked) == 0 {
			continue // マッピング 0 件 / プール空のレベルはスキップ
		}
		finalCharts = append(finalCharts, picked...)
		finalLevelOrder = append(finalLevelOrder, lv.Name)
	}

	return domain.PickResult{
		PublishedTableID: pub.ID,
		GeneratedAt:      u.clock.Now(),
		SeedKey:          seedKey,
		Charts:           finalCharts,
		LevelOrder:       finalLevelOrder,
	}, nil
}

// pickLevel は 1 公開レベル分のフェーズ 1 + フェーズ 2 を実行する。
func (u *PickUseCase) pickLevel(ctx context.Context, pub domain.PublishedTable, lv domain.PublishedTableLevel, rng *rand.Rand, now time.Time) ([]domain.EnrichedChart, error) {
	// 1. ソース表ごとに LoadCharts（重複避けキャッシュ）して mapping ごとのプールを作る
	sources := map[string][]domain.EnrichedChart{}
	for _, mp := range lv.Mappings {
		if _, ok := sources[mp.SourceTableID]; !ok {
			cs, err := u.srcRepo.LoadCharts(ctx, mp.SourceTableID, port.ChartQuery{OwnedOnly: pub.OwnedOnly})
			if err != nil {
				return nil, fmt.Errorf("load charts %q: %w", mp.SourceTableID, err)
			}
			sources[mp.SourceTableID] = cs
		}
	}

	pools := make([][]domain.EnrichedChart, len(lv.Mappings))
	for i, mp := range lv.Mappings {
		pools[i] = filterByLevel(sources[mp.SourceTableID], mp.SourceLevel)
	}

	// 2. dedup 主キーでプールユニオンを作る（フェーズ 2 用）
	type dedupKey struct{ k string }
	keyOf := func(c domain.EnrichedChart) dedupKey {
		if c.MD5 != "" {
			return dedupKey{c.MD5}
		}
		return dedupKey{"sha:" + c.SHA256}
	}
	seen := map[dedupKey]struct{}{}
	var unionPool []domain.EnrichedChart
	for _, p := range pools {
		for _, c := range p {
			k := keyOf(c)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			unionPool = append(unionPool, c)
		}
	}

	// 3. フェーズ 1
	var picked []domain.EnrichedChart
	pickedKeys := map[dedupKey]struct{}{}
	for i := range pools {
		avail := make([]domain.EnrichedChart, 0, len(pools[i]))
		for _, c := range pools[i] {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			avail = append(avail, c)
		}
		// 安定ソート（決定論性）
		sort.SliceStable(avail, func(a, b int) bool { return avail[a].Position < avail[b].Position })
		taken := weightedSampleWithoutReplacement(avail, lv.PerMappingPick, u.weighter, rng, ctx, now)
		for _, c := range taken {
			pickedKeys[keyOf(c)] = struct{}{}
		}
		picked = append(picked, taken...)
	}

	// 4. フェーズ 2
	need := lv.TotalPick - len(picked)
	if need > 0 {
		remaining := make([]domain.EnrichedChart, 0, len(unionPool))
		for _, c := range unionPool {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			remaining = append(remaining, c)
		}
		sort.SliceStable(remaining, func(a, b int) bool { return remaining[a].Position < remaining[b].Position })
		taken := weightedSampleWithoutReplacement(remaining, need, u.weighter, rng, ctx, now)
		picked = append(picked, taken...)
	}

	return picked, nil
}

// filterByLevel は同一ソース内で level が一致する譜面のみ返す。
func filterByLevel(charts []domain.EnrichedChart, level string) []domain.EnrichedChart {
	out := make([]domain.EnrichedChart, 0, len(charts))
	for _, c := range charts {
		if c.Level == level {
			out = append(out, c)
		}
	}
	return out
}

// weightedSampleWithoutReplacement は重み付き非復元サンプリング（k 件まで）。
// 重み 0 以下の譜面は対象外。実装は「累積重み → 二分探索 → 削除」の単純実装で十分。
func weightedSampleWithoutReplacement(
	pool []domain.EnrichedChart, k int,
	w port.Weighter, rng *rand.Rand,
	ctx context.Context, now time.Time,
) []domain.EnrichedChart {
	if k <= 0 || len(pool) == 0 {
		return nil
	}
	// 重みを計算
	weights := make([]float64, len(pool))
	totalWeight := 0.0
	for i, c := range pool {
		wt := w.Weight(ctx, c, now)
		if wt <= 0 {
			weights[i] = 0
			continue
		}
		weights[i] = wt
		totalWeight += wt
	}
	taken := make([]domain.EnrichedChart, 0, k)
	used := make([]bool, len(pool))
	for len(taken) < k && totalWeight > 0 {
		r := rng.Float64() * totalWeight
		cum := 0.0
		picked := -1
		for i, wt := range weights {
			if used[i] || wt <= 0 {
				continue
			}
			cum += wt
			if r <= cum {
				picked = i
				break
			}
		}
		if picked < 0 {
			break
		}
		taken = append(taken, pool[picked])
		totalWeight -= weights[picked]
		used[picked] = true
	}
	// 出力は安定性のため Position 昇順
	sort.SliceStable(taken, func(a, b int) bool { return taken[a].Position < taken[b].Position })
	return taken
}
```

- [ ] **Step 4: NewPickUseCase シグネチャを更新（Weighter 必須にするか functional option にするか）**

ファクトリ関数を書き換え:

```go
type PickUseCase struct {
	pubRepo  port.PublishedTableRepo
	srcRepo  port.SourceTableRepo
	store    *PickResultStore
	clock    port.Clock
	randNew  port.RandSourceFactory
	log      *slog.Logger
	weighter port.Weighter
}

func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
	weighter port.Weighter,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, store: store,
		clock: clock, randNew: randNew, log: log, weighter: weighter,
	}
}
```

呼び出し側（`bootstrap.go` 含む）はこの後の Task 8 で更新する。

- [ ] **Step 5: テスト実行 → 通過を確認**

Run: `go test ./internal/usecase/... -run TestPickUseCase -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/usecase/pick_usecase.go internal/usecase/pick_usecase_test.go
git commit -m "feat(usecase): PickUseCase をフェーズ 1 (m 曲ずつ) + フェーズ 2 (合計 n 曲補填) に書き換え + Weighter 注入"
```

---

## Task 8: Bootstrap で Weighter を注入し、コンパイルを通す

**Files:**
- Modify: `internal/app/bootstrap.go`

- [ ] **Step 1: bootstrap.go の `NewPickUseCase` 呼び出しを更新**

import に `"github.com/meta-BE/bms-random-table-compositor/internal/adapter/weighter"` を追加し、`pickUC := usecase.NewPickUseCase(...)` の最後の引数に `weighter.UniformWeighter{}` を渡す:

```go
pickUC := usecase.NewPickUseCase(pubRepo, sourceRepo, pickStore, systemClock, randFactory, lg, weighter.UniformWeighter{})
```

- [ ] **Step 2: 全体ビルド確認**

Run: `go build ./...`
Expected: 成功（ハンドラ層の DTO 不整合が残っていれば次の Task 9 で対応）

Run: `go test ./...`
Expected: handler 層が落ちる可能性。落ちた場合は Task 9 で fix。

- [ ] **Step 3: コミット**

```bash
git add internal/app/bootstrap.go
git commit -m "wire(app): Bootstrap で UniformWeighter を PickUseCase に注入"
```

---

## Task 9: PublishedTableHandler の DTO を Levels 込みに更新する

**Files:**
- Modify: `internal/app/handler/published_table_handler.go`
- Modify: `internal/app/handler/published_table_handler_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/handler/published_table_handler_test.go` の既存テストは旧 DTO 仕様なので新仕様に更新。新規:

```go
func TestPublishedTableHandler_CreateFromSourceTable_DelegatesToUseCase(t *testing.T) {
	uc, _, srcRepo := newPubUCFixture(t)
	srcRepo.put(domain.SourceTable{
		ID: "src-A", LevelOrder: []string{"1", "2"}, LastFetchStatus: domain.FetchStatusOK,
	})
	h := handler.NewPublishedTableHandler(uc)

	id, err := h.CreatePublishedTableFromSource(handler.CreateFromSourceRequest{
		SourceTableID: "src-A",
		Slug:          "stella",
		DisplayName:   "Stella",
		Symbol:        "★",
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)
}

func TestPublishedTableHandler_ApplyBulkPickConfig_DelegatesToUseCase(t *testing.T) { ... }

func TestPublishedTableHandler_ListPublishedTables_DTOExcludesSourceTableID(t *testing.T) {
	// DTO に sourceTableId が「無い」ことを確認（フロントの古い参照を防ぐ）
	// JSON 化して "sourceTableId" 文字列を含まないことをチェック
}
```

- [ ] **Step 2: published_table_handler.go を新仕様に書き換える**

DTO を以下のように変更:

```go
type PublishedTableLevelDTO struct {
	ID             string                            `json:"id"`
	Name           string                            `json:"name"`
	SortOrder      int                               `json:"sortOrder"`
	PerMappingPick int                               `json:"perMappingPick"`
	TotalPick      int                               `json:"totalPick"`
	Mappings       []PublishedTableLevelMappingDTO   `json:"mappings"`
}

type PublishedTableLevelMappingDTO struct {
	ID            string `json:"id"`
	SourceTableID string `json:"sourceTableId"`
	SourceLevel   string `json:"sourceLevel"`
	SortOrder     int    `json:"sortOrder"`
}

type PublishedTableDTO struct {
	ID          string                   `json:"id"`
	Slug        string                   `json:"slug"`
	DisplayName string                   `json:"displayName"`
	Symbol      string                   `json:"symbol"`
	OwnedOnly   bool                     `json:"ownedOnly"`
	RefreshMode string                   `json:"refreshMode"`
	SortOrder   int                      `json:"sortOrder"`
	Levels      []PublishedTableLevelDTO `json:"levels"` // List では空配列
}

type PublishedTableLevelInputDTO struct {
	Name           string                                  `json:"name"`
	PerMappingPick int                                     `json:"perMappingPick"`
	TotalPick      int                                     `json:"totalPick"`
	Mappings       []PublishedTableLevelMappingInputDTO    `json:"mappings"`
}

type PublishedTableLevelMappingInputDTO struct {
	SourceTableID string `json:"sourceTableId"`
	SourceLevel   string `json:"sourceLevel"`
}

type CreatePublishedTableRequest struct {
	Slug        string                        `json:"slug"`
	DisplayName string                        `json:"displayName"`
	Symbol      string                        `json:"symbol"`
	OwnedOnly   bool                          `json:"ownedOnly"`
	RefreshMode string                        `json:"refreshMode"`
	Levels      []PublishedTableLevelInputDTO `json:"levels"`
}

type UpdatePublishedTableRequest struct {
	ID          string                        `json:"id"`
	Slug        string                        `json:"slug"`
	DisplayName string                        `json:"displayName"`
	Symbol      string                        `json:"symbol"`
	OwnedOnly   bool                          `json:"ownedOnly"`
	RefreshMode string                        `json:"refreshMode"`
	SortOrder   int                           `json:"sortOrder"`
	Levels      []PublishedTableLevelInputDTO `json:"levels"`
}

type CreateFromSourceRequest struct {
	SourceTableID string `json:"sourceTableId"`
	Slug          string `json:"slug"`
	DisplayName   string `json:"displayName"`
	Symbol        string `json:"symbol"`
}

type ApplyBulkPickConfigRequest struct {
	ID             string `json:"id"`
	PerMappingPick int    `json:"perMappingPick"`
	TotalPick      int    `json:"totalPick"`
}
```

新メソッド:

```go
func (h *PublishedTableHandler) CreatePublishedTableFromSource(req CreateFromSourceRequest) (string, error) {
	return h.uc.CreateFromSourceTable(h.ctx, req.SourceTableID, req.Slug, req.DisplayName, req.Symbol)
}

func (h *PublishedTableHandler) ApplyBulkPickConfig(req ApplyBulkPickConfigRequest) error {
	return h.uc.ApplyBulkPickConfig(h.ctx, req.ID, req.PerMappingPick, req.TotalPick)
}
```

`toPublishedTableDTO` を Levels 込みに変換するように更新（List 用と Get 用で挙動を分けるなら 2 関数に分割）。

- [ ] **Step 3: テスト実行 → 通過を確認**

Run: `go test ./internal/app/handler/... -v`
Expected: PASS

Run: `go test ./...`
Expected: PASS（ここで全体テストが green）

Run: `make lint`
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add internal/app/handler/published_table_handler.go internal/app/handler/published_table_handler_test.go
git commit -m "feat(handler): PublishedTableHandler の DTO を Levels/Mappings 込みに刷新 + 新メソッド 2 つ追加"
```

---

## Task 10: HTTP `data.json` の level を公開レベル名で上書き出力する

**Files:**
- Modify: `internal/adapter/httpserver/handler_data.go`
- Modify: `internal/adapter/httpserver/handler_data_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`handler_data_test.go` に追加:

```go
func TestDataHandler_LevelIsOverriddenWithPublicLevelName(t *testing.T) {
	// 公開レベル "5-mix" にマップされた譜面の data.json 内 level が "5-mix" になる
	// （元のソース譜面 level が "5" でも）
	// fixture: PickResult.Charts に level="5" の譜面、PickResult.LevelOrder=["5-mix"]
	// → 公開レベル名で上書き
}
```

詳細: `PickResult` には現在 `LevelOrder []string` のみ。各 chart にどの公開レベル名が紐付くかをどう持つかが論点。ピック結果に「公開レベル名」を埋め込む形に拡張する必要がある。

`domain.PickResult` の Charts 各要素は `EnrichedChart` を埋め込んだ `SourceChart` だが、その `Level` は **ソース表側** の level。HTTP 出力で公開レベル名にしたいなら:

選択肢 A: `domain.PickResult` に `Charts []PickedChart` を導入し、`PickedChart{ EnrichedChart, PublicLevel string }` を持つ。
選択肢 B: `pickLevel` から返すときに `EnrichedChart.Level` を **公開レベル名で上書きしたコピー** を作る。

選択肢 B の方が変更点が少ない。`Level` が最終的に表示されるラベルになるという見方。

→ 本プランは **選択肢 B** を採用。`pickLevel` の戻り値を作るときに `c.Level = lv.Name` で上書きしたコピーを返す。これにより `data.json`/`header.json`/`HTML` 全てが自動的に公開レベル名で表示される。

- [ ] **Step 2: pick_usecase.go を更新（公開レベル名で上書き）**

`pickLevel` の最後で `picked` を返す前に:

```go
// 公開レベル名で Level フィールドを上書き（HTTP 出力で公開レベル名を見せるため）
out := make([]domain.EnrichedChart, 0, len(picked))
for _, c := range picked {
	cc := c
	cc.Level = lv.Name
	out = append(out, cc)
}
return out, nil
```

`pick_usecase_test.go` の dedup / phase テストを `r.Charts[i].Level == "5-mix"` 等の検証も含む形に更新。

- [ ] **Step 3: handler_data_test.go の新テストを書く**

`handler_test_helpers_test.go` にある `fakePickUC`（or 既存のモック）を使って PickResult を組み立てる:

```go
func TestDataHandler_LevelFieldIsPublicLevelName(t *testing.T) {
	pickUC := &fakePickUC{
		bySlug: map[string]fakePickResult{
			"mix5": {
				result: domain.PickResult{
					LevelOrder: []string{"5-mix"},
					Charts: []domain.EnrichedChart{
						{
							SourceChart: domain.SourceChart{
								MD5: "abc", Title: "T1", Artist: "A1",
								Level: "5-mix", // 公開レベル名で上書き済み
								Raw:   map[string]any{"md5": "abc", "level": "5", "url": "http://x"},
							},
						},
					},
				},
				pub: domain.PublishedTable{Slug: "mix5"},
			},
		},
	}
	deps := Deps{Pick: pickUC, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mix5/data.json", nil)
	req.SetPathValue("slug", "mix5")
	newDataHandler(deps).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var entries []map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&entries))
	require.Len(t, entries, 1)
	require.Equal(t, "5-mix", entries[0]["level"])
	// Raw の url 等はパススルーされている
	require.Equal(t, "http://x", entries[0]["url"])
}
```

`fakePickUC` の最低限の実装が既存テストヘルパに無ければ追加（`PickBySlug` だけ実装）。

- [ ] **Step 4: テスト実行 → 通過を確認**

Run: `go test ./internal/adapter/httpserver/... -v`
Expected: PASS

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/pick_usecase.go internal/usecase/pick_usecase_test.go internal/adapter/httpserver/handler_data_test.go
git commit -m "feat(pick,httpserver): ピック結果の Level を公開レベル名で上書きし data.json/header.json に伝播"
```

---

## Task 11: フロントエンド api.ts と Wails bind を更新する

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Run: `wails generate module`

- [ ] **Step 1: api.ts の型定義を新仕様に更新**

```ts
export type RefreshMode = 'per_request' | 'daily' | 'manual';

export interface PublishedTableLevelMappingDTO {
  id: string;
  sourceTableId: string;
  sourceLevel: string;
  sortOrder: number;
}

export interface PublishedTableLevelDTO {
  id: string;
  name: string;
  sortOrder: number;
  perMappingPick: number;
  totalPick: number;
  mappings: PublishedTableLevelMappingDTO[];
}

export interface PublishedTableDTO {
  id: string;
  slug: string;
  displayName: string;
  symbol: string;
  ownedOnly: boolean;
  refreshMode: RefreshMode;
  sortOrder: number;
  levels: PublishedTableLevelDTO[];
}

export interface PublishedTableLevelInputDTO {
  name: string;
  perMappingPick: number;
  totalPick: number;
  mappings: { sourceTableId: string; sourceLevel: string }[];
}

export interface CreatePublishedTableRequest {
  slug: string;
  displayName: string;
  symbol: string;
  ownedOnly: boolean;
  refreshMode: RefreshMode;
  levels: PublishedTableLevelInputDTO[];
}

export interface UpdatePublishedTableRequest extends CreatePublishedTableRequest {
  id: string;
  sortOrder: number;
}

export interface CreateFromSourceRequest {
  sourceTableId: string;
  slug: string;
  displayName: string;
  symbol: string;
}

export interface ApplyBulkPickConfigRequest {
  id: string;
  perMappingPick: number;
  totalPick: number;
}
```

`api` オブジェクトに新メソッド追加:

```ts
export const api = {
  // ... 既存メソッド
  createPublishedTableFromSource(req: CreateFromSourceRequest): Promise<string> {
    return CreatePublishedTableFromSource(req);
  },
  applyBulkPickConfig(req: ApplyBulkPickConfigRequest): Promise<void> {
    return ApplyBulkPickConfig(req);
  },
  getPublishedTable(id: string): Promise<PublishedTableDTO> {
    return GetPublishedTable(id);
  },
};
```

`getPublishedTable`(単一取得・Levels 込み) ハンドラが現在無いので、Task 9 で `Get` メソッドをハンドラに追加すること。`PublishedTableHandler` に以下を追加:

```go
func (h *PublishedTableHandler) GetPublishedTable(id string) (PublishedTableDTO, error) {
	t, err := h.uc.Get(h.ctx, id)
	if err != nil {
		return PublishedTableDTO{}, err
	}
	return toPublishedTableDTO(t, true /* withLevels */), nil
}
```

（このメソッド追加が Task 9 で漏れていれば、本タスク内で追記する）

- [ ] **Step 2: wails generate module を実行**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/handler/PublishedTableHandler.ts` 等が再生成される（gitignore 対象）

- [ ] **Step 3: 型チェック**

Run: `cd frontend && npm run check`
Expected: PASS（`PublishedTablesTab.svelte` 等で旧 DTO 参照のエラーが出る → Task 12 で修正）。

このタスクの段階では `npm run check` が **失敗のままでも構わない**（Task 12 で UI を完全更新）。エラーリストを控えておく。

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/api.ts internal/app/handler/published_table_handler.go
git commit -m "feat(api,handler): フロント API 型と GetPublishedTable バインドを Levels 仕様に更新"
```

---

## Task 12: 公開表編集 UI を新仕様に書き換える（コア）

**Files:**
- Create: `frontend/src/lib/components/PublishedTableLevelEditor.svelte`
- Modify: `frontend/src/lib/tabs/PublishedTablesTab.svelte`

- [ ] **Step 1: PublishedTableLevelEditor.svelte を作成**

`frontend/src/lib/components/PublishedTableLevelEditor.svelte`:

```svelte
<script lang="ts">
  import type {
    PublishedTableLevelInputDTO,
    SourceTableDTO,
  } from '../api';

  // 親から levels と sources を受け取り、編集後の levels を双方向バインド
  export let levels: PublishedTableLevelInputDTO[] = [];
  export let sources: SourceTableDTO[] = [];

  // バルク適用用
  let bulkM = 0;
  let bulkN = 0;

  function addLevel() {
    levels = [
      ...levels,
      { name: `Lv${levels.length + 1}`, perMappingPick: 0, totalPick: 0, mappings: [] },
    ];
  }

  function removeLevel(i: number) {
    levels = levels.filter((_, idx) => idx !== i);
  }

  function moveLevel(i: number, delta: number) {
    const j = i + delta;
    if (j < 0 || j >= levels.length) return;
    const next = [...levels];
    [next[i], next[j]] = [next[j], next[i]];
    levels = next;
  }

  function addMapping(i: number) {
    if (sources.length === 0) return;
    const next = [...levels];
    next[i].mappings = [
      ...next[i].mappings,
      { sourceTableId: sources[0].id, sourceLevel: '' },
    ];
    levels = next;
  }

  function removeMapping(i: number, j: number) {
    const next = [...levels];
    next[i].mappings = next[i].mappings.filter((_, idx) => idx !== j);
    levels = next;
  }

  function applyBulk() {
    levels = levels.map((lv) => ({ ...lv, perMappingPick: bulkM, totalPick: bulkN }));
  }

  function sourceLevelOptions(sourceId: string): string[] {
    const s = sources.find((x) => x.id === sourceId);
    return s ? s.levelOrder : [];
  }
</script>

<div class="space-y-4">
  <!-- バルク適用パネル -->
  <div class="bg-base-200 p-3 rounded">
    <div class="font-semibold mb-2">全レベル一括適用</div>
    <div class="flex gap-2 items-end">
      <label class="form-control">
        <span class="label-text text-xs">レベルごとピック曲数 (m)</span>
        <input type="number" min="0" class="input input-bordered input-sm w-24" bind:value={bulkM} />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">全体ピック曲数 (n)</span>
        <input type="number" min="0" class="input input-bordered input-sm w-24" bind:value={bulkN} />
      </label>
      <button class="btn btn-sm btn-primary" on:click={applyBulk}>全レベルに適用</button>
    </div>
    <p class="text-xs opacity-70 mt-2">
      各マッピングから m 曲を最低保証し、合計 n 曲になるよう全体プールから補填します
      （n=0 または m × マッピング数 ≥ n のときは補填なし）。
    </p>
  </div>

  <!-- レベル一覧テーブル -->
  <div>
    <div class="flex justify-between items-center mb-2">
      <h3 class="font-semibold">公開レベル一覧</h3>
      <button class="btn btn-sm btn-outline" on:click={addLevel}>+ レベル追加</button>
    </div>
    <div class="overflow-x-auto">
      <table class="table table-zebra table-sm">
        <thead>
          <tr>
            <th>並び</th>
            <th>名前</th>
            <th>マッピング</th>
            <th title="レベルごとピック曲数（マッピング 1 件あたり）">m</th>
            <th title="全体ピック曲数（公開レベル合計目標）">n</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each levels as lv, i (i)}
            <tr>
              <td>
                <button class="btn btn-xs" on:click={() => moveLevel(i, -1)}>▲</button>
                <button class="btn btn-xs" on:click={() => moveLevel(i, 1)}>▼</button>
              </td>
              <td>
                <input type="text" class="input input-bordered input-xs w-32" bind:value={lv.name} />
              </td>
              <td>
                <div class="flex flex-wrap gap-1 items-center">
                  {#each lv.mappings as mp, j (j)}
                    <div class="badge badge-outline gap-1">
                      <select class="select select-xs" bind:value={mp.sourceTableId}>
                        {#each sources as s}
                          <option value={s.id}>{s.displayName || s.name}</option>
                        {/each}
                      </select>
                      <select class="select select-xs" bind:value={mp.sourceLevel}>
                        {#each sourceLevelOptions(mp.sourceTableId) as lvl}
                          <option value={lvl}>{lvl}</option>
                        {/each}
                      </select>
                      <button class="btn btn-xs btn-ghost" on:click={() => removeMapping(i, j)}>✕</button>
                    </div>
                  {/each}
                  <button class="btn btn-xs btn-outline" on:click={() => addMapping(i)}>+</button>
                </div>
              </td>
              <td><input type="number" min="0" class="input input-bordered input-xs w-16" bind:value={lv.perMappingPick} /></td>
              <td><input type="number" min="0" class="input input-bordered input-xs w-16" bind:value={lv.totalPick} /></td>
              <td>
                <button class="btn btn-xs btn-error btn-outline" on:click={() => removeLevel(i)}>削除</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>
```

- [ ] **Step 2: PublishedTablesTab.svelte を新仕様に書き換える**

主要な変更点:
1. 一覧の表示は変えない（`displayName / slug / refreshMode / レベル数`）
2. 「+ 公開表追加」ボタン → 作成方法選択ダイアログ
   - 「ソース表からウィザード生成」: ソース表 select + slug + displayName + symbol → `createPublishedTableFromSource`
   - 「ブランクから作成」: 通常のフォーム（slug, displayName, symbol, refreshMode のみ） → `createPublishedTable`（Levels=[]）
3. 各行の「編集」: モーダルを開き、`getPublishedTable(id)` で Levels 込み取得 → `PublishedTableLevelEditor` 経由で編集 → `updatePublishedTable`
4. バルク適用専用ボタン（任意） or `PublishedTableLevelEditor` 内のバルクパネル

具体的な書き換えは長いので、既存ファイルの構造を踏襲しつつ:
- `form.sourceTableId` / `form.pickPerLevel` の参照を全削除
- `form.levels: PublishedTableLevelInputDTO[]` を追加
- 「保存」ハンドラで `levels` を含めて Create/Update を呼ぶ

実装の詳細スケルトン:

```svelte
<script lang="ts">
  // ...既存 import...
  import PublishedTableLevelEditor from '../components/PublishedTableLevelEditor.svelte';

  // 作成導線
  let createKind: 'wizard' | 'blank' | null = null;

  let form = {
    slug: '',
    displayName: '',
    symbol: '',
    ownedOnly: false,
    refreshMode: 'per_request' as RefreshMode,
    sortOrder: 0,
    levels: [] as PublishedTableLevelInputDTO[],
  };
  let wizardSourceId = '';

  function startCreateWizard() {
    createKind = 'wizard';
    wizardSourceId = sources[0]?.id ?? '';
    form = {
      slug: '',
      displayName: '',
      symbol: '',
      ownedOnly: false,
      refreshMode: 'manual',
      sortOrder: 0,
      levels: [],
    };
    formMode = 'create';
    formOpen = true;
  }

  function startCreateBlank() {
    createKind = 'blank';
    form = {
      slug: '',
      displayName: '',
      symbol: '',
      ownedOnly: false,
      refreshMode: 'manual',
      sortOrder: 0,
      levels: [],
    };
    formMode = 'create';
    formOpen = true;
  }

  async function saveCreate() {
    if (createKind === 'wizard') {
      await api.createPublishedTableFromSource({
        sourceTableId: wizardSourceId,
        slug: form.slug,
        displayName: form.displayName,
        symbol: form.symbol,
      });
    } else {
      await api.createPublishedTable({
        slug: form.slug, displayName: form.displayName, symbol: form.symbol,
        ownedOnly: form.ownedOnly, refreshMode: form.refreshMode,
        levels: form.levels,
      });
    }
    await reload();
  }

  async function openEdit(id: string) {
    const pub = await api.getPublishedTable(id);
    form = {
      slug: pub.slug, displayName: pub.displayName, symbol: pub.symbol,
      ownedOnly: pub.ownedOnly, refreshMode: pub.refreshMode, sortOrder: pub.sortOrder,
      levels: pub.levels.map((lv) => ({
        name: lv.name, perMappingPick: lv.perMappingPick, totalPick: lv.totalPick,
        mappings: lv.mappings.map((mp) => ({ sourceTableId: mp.sourceTableId, sourceLevel: mp.sourceLevel })),
      })),
    };
    formMode = { kind: 'edit', id };
    formOpen = true;
  }
</script>

<!-- テンプレート: 既存の構造を踏襲しつつ
     formOpen 内に PublishedTableLevelEditor を埋め込み、
     「作成方法選択ダイアログ」を別 modal で表示 -->
```

- [ ] **Step 3: 型チェック**

Run: `cd frontend && npm run check`
Expected: PASS

- [ ] **Step 4: 手動動作確認**

Run: `make dev`

ブラウザ等で:
- 「+ 公開表追加」 → 「ソース表からウィザード生成」を選び、stella を選択 → 公開表が作られ、編集画面で各レベルにマッピング 1 件が入っていることを確認
- 「ブランク作成」を選び、空の公開表を作る → 編集画面で「+ レベル追加」「+ マッピング追加」を試す
- バルク適用パネルで `m=2, n=5` を入れて「全レベルに適用」 → 全レベルの m, n が 2, 5 になる
- 保存後、再度開いて Levels が永続化されていることを確認
- HTTP サーバ起動状態で `http://127.0.0.1:<port>/<slug>` を開き、公開レベル名で表示されることを確認

- [ ] **Step 5: コミット**

```bash
git add frontend/src/lib/components/PublishedTableLevelEditor.svelte frontend/src/lib/tabs/PublishedTablesTab.svelte
git commit -m "feat(frontend): 公開表編集 UI を Levels/Mappings + バルク適用 + 作成導線 2 種に刷新"
```

---

## Task 13: マニュアル更新と最終検証

**Files:**
- Modify: `docs/manual.md`
- Modify: `docs/TODO.md`

- [ ] **Step 1: docs/manual.md を新仕様に合わせて更新**

公開表の章に以下を追記/書き換え:
- 「公開表の作成: ソース表からウィザード生成 / ブランク作成 の 2 通り」
- 「公開レベルとマッピング」: 1 公開レベルに複数のソース表レベルを紐付けられる旨と例
- 「ピック設定 (m, n)」: フェーズ 1 + フェーズ 2 のセマンティクス + バルク適用ボタン
- 起動時の告知（移行時にデータが失われた経緯を説明）

- [ ] **Step 2: docs/TODO.md の v2 機能項目をマーク**

```markdown
- ✅ **複数ソース表合成**: 複数の難易度表を 1 つの公開表にマージ (v0.2.0 で実装)
```

- [ ] **Step 3: 全体テスト + lint + build**

Run: `go test ./...`
Expected: PASS

Run: `make lint`
Expected: PASS

Run: `cd frontend && npm run check`
Expected: PASS

Run: `go build ./...`
Expected: 成功

Run: `make build`
Expected: 成果物生成

- [ ] **Step 4: コミット + push**

```bash
git add docs/manual.md docs/TODO.md
git commit -m "docs(manual,todo): 複数ソース表合成 機能のマニュアル更新と TODO 完了マーク"
git push origin main
```

- [ ] **Step 5: 動作確認チェックリスト**

リモート push 後に Windows ビルドを取得し、以下を実機で確認:
- [ ] 既存 DB を持って起動しても crash せず、旧公開表が破棄されている
- [ ] ソース表ウィザード生成で公開表が作れる
- [ ] ブランク作成 → レベル追加 → マッピング追加 → 保存 → 再度開くと永続化されている
- [ ] バルク適用ボタンで全レベルの (m, n) が一括変更される
- [ ] beatoraja から公開表 URL を読み込ませて、公開レベル名で表示される
- [ ] m, n の組合せ (n=0 / m × k < n / m × k >= n) 各パターンで合計曲数が想定通り

---

## 自己レビュー: スペックカバレッジ

スペック §1 概要、§2 In/Out スコープ:
- §3 ドメインモデル → Task 1, 2
- §4 DB スキーマ → Task 3
- §5 ピックアルゴリズム + Weighter → Task 6, 7
- §6 UseCase / Repository → Task 4, 5
- §7 HTTP 出力 → Task 10
- §8 UI → Task 11, 12
- §9 バリデーション → Task 5
- §10 テスト方針 → 各タスク内のテスト
- §11 マイグレーション方針 → Task 3 + Task 13 マニュアル
- §12 拡張ポイント → Task 6 (Weighter)
- §13 実装順 → 本プランの Task 1-13

**警告系バリデーション** (`m * len(Mappings) > n` 警告 / マッピング 0 件警告) は UI で行う想定。Task 12 のレベルエディタに「`m × マッピング数 ≥ n` のとき n 列にラベル付け」「マッピング 0 件のレベルに警告 badge」を加える微調整が必要 → Task 12 の Step 2 範囲で対応する。

**プレースホルダ:** 本プランに `TBD` / `TODO 後で` などの未完成記述は無し（あれば修正）。

**型一貫性:** `PublishedTableLevelInput` (Go) ↔ `PublishedTableLevelInputDTO` (TS / DTO) の名前差は意図的（DTO 側に Suffix）。`PerMappingPick` / `TotalPick` フィールド名は全レイヤで統一済み。
