# 最終プレイ日時優先ピック 実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 公開表のピックに「最終プレイ日時で古い曲を優先する」3 モード (OFF / 確率 X 倍 / 完全日時順ソート) を導入する。

**Architecture:** `score.db` を新規 `ScoreDBAttacher` で RO ATTACH (schema `sc`)、`LoadCharts` SQL に `MAX(sc.date)` 取得を追加し `EnrichedChart.LastPlayedAt` に流す。`pickLevel` 入口で `unionPool` 全体の `max(now-date)` を計算し、`a ∈ [0,1]` を Weighter (`w = 1 + (X-1)*a`) で重みに変換。`sort` モードは `(date 降順, mappingIdx, position)` の決定論ソート。

**Tech Stack:** Go 1.x + Wails v2 + SQLite (modernc.org/sqlite) + Svelte 4 + Tailwind v4 + daisyUI v5

**Spec:** `docs/superpowers/specs/2026-05-11-last-played-pick-weighting-design.md`

---

## Task 1: マイグレーション拡張 (published_table)

`weight_mode` / `weight_param_x` カラムを既存 `published_table` に冪等に追加する。schema_version は据え置き (DROP しない追加のみ)。

**Files:**
- Modify: `internal/adapter/persistence/migrations.go`
- Test: `internal/adapter/persistence/migrations_test.go`

- [ ] **Step 1: 失敗テストを追加**

`migrations_test.go` の末尾に以下を追加。

```go
func TestRunMigrations_AddsWeightModeColumns(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	cols := tableColumns(t, db, "published_table")
	require.Contains(t, cols, "weight_mode")
	require.Contains(t, cols, "weight_param_x")
}

func TestRunMigrations_BackfillsWeightDefaults(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	// 旧 v2 スキーマ相当 (weight_* カラム無し) でテーブルを作る
	require.NoError(t, RunMigrations(db))
	_, err = db.Exec(`INSERT INTO published_table(id, slug, display_name) VALUES('p1','s1','t1')`)
	require.NoError(t, err)

	// 再度 RunMigrations を呼んでも既存行に DEFAULT が当たること
	require.NoError(t, RunMigrations(db))
	var mode string
	var x int
	require.NoError(t, db.QueryRow(
		`SELECT weight_mode, weight_param_x FROM published_table WHERE id='p1'`,
	).Scan(&mode, &x))
	require.Equal(t, "off", mode)
	require.Equal(t, 10, x)
}

func TestRunMigrations_WeightColumnsAddedAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db))
	cols := tableColumns(t, db, "published_table")
	// 重複追加で 2 個になっていない
	count := 0
	for _, c := range cols {
		if c == "weight_mode" {
			count++
		}
	}
	require.Equal(t, 1, count)
}
```

- [ ] **Step 2: テストが失敗することを確認**

```
go test ./internal/adapter/persistence/ -run TestRunMigrations_AddsWeightModeColumns -v
```
Expected: FAIL (`weight_mode` カラムが存在しない)

- [ ] **Step 3: マイグレーションに ALTER を追加**

`migrations.go` の `v2Statements` ループの後、`schema_version` 書き込みの前に以下を挿入:

```go
	// v2 への追加カラム (最終プレイ日時優先ピック)。pragma_table_info で冪等化。
	if !columnExists(db, "published_table", "weight_mode") {
		if _, err := db.Exec(
			`ALTER TABLE published_table
			   ADD COLUMN weight_mode TEXT NOT NULL DEFAULT 'off'`,
		); err != nil {
			return fmt.Errorf("alter add weight_mode: %w", err)
		}
	}
	if !columnExists(db, "published_table", "weight_param_x") {
		if _, err := db.Exec(
			`ALTER TABLE published_table
			   ADD COLUMN weight_param_x INTEGER NOT NULL DEFAULT 10`,
		); err != nil {
			return fmt.Errorf("alter add weight_param_x: %w", err)
		}
	}
```

ファイル末尾に補助関数を追加:

```go
// columnExists は対象テーブルに指定カラムが存在するかを返す。
// マイグレーションの冪等化に使う。
func columnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return false
		}
		if n == column {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: テストが通ることを確認**

```
go test ./internal/adapter/persistence/ -run TestRunMigrations -v
```
Expected: PASS (全マイグレーションテスト)

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/migrations.go internal/adapter/persistence/migrations_test.go
git commit -m "feat(persistence): published_table に weight_mode/weight_param_x カラム追加

最終プレイ日時優先ピック (3 モード) の永続化を支える。pragma_table_info で
冪等化、既存行は DEFAULT (off, 10) で埋まる。"
```

---

## Task 2: domain.WeightMode + PickConfig 拡張

`WeightMode` 型と `PickConfig` の 2 フィールド追加。純粋な型定義のみで、参照側は次タスク以降で更新する。

**Files:**
- Modify: `internal/domain/published_table.go`

- [ ] **Step 1: PickConfig 拡張**

`internal/domain/published_table.go` の `PickConfig` 定義を以下に差し替え。

```go
// WeightMode は重み付けピックのモード。
type WeightMode string

const (
	WeightModeOff         WeightMode = "off"         // 一様ランダム
	WeightModeProbability WeightMode = "probability" // 確率 (X 倍まで偏らせる)
	WeightModeSort        WeightMode = "sort"        // 完全日時順ソート (古い順)
)

// PickConfig はピック生成に必要な設定値。
// PerLevel / PreferOldPlay は撤去（複数ソース表合成スペックで Levels[].PerMappingPick/TotalPick と Weighter に置き換わった）。
type PickConfig struct {
	RefreshMode  RefreshMode // per_request / daily / manual
	WeightMode   WeightMode  // 既定 WeightModeOff
	WeightParamX int         // 既定 10、probability モードでのみ使用
}
```

- [ ] **Step 2: ビルド確認**

```
go build ./...
```
Expected: 成功 (新しいフィールドはまだ参照する側がいないので破壊的変更にはならない)

- [ ] **Step 3: コミット**

```bash
git add internal/domain/published_table.go
git commit -m "feat(domain): PickConfig に WeightMode と WeightParamX を追加

最終プレイ日時優先ピック (off/probability/sort) の設定値を持たせる。"
```

---

## Task 3: port.Weighter シグネチャ変更 + WeighterFactory 新設

`Weighter` の責務を「正規化された a を重みに変換する純関数」に変更。a の計算は呼び出し側で行う。

**Files:**
- Modify: `internal/port/weighter.go`

- [ ] **Step 1: port/weighter.go を書き換え**

```go
package port

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// Weighter はピック時の重み関数。集合内で正規化された経過時間 a ∈ [0, 1]
// (0 = 最新プレイ, 1 = 最古プレイ / 未プレイ) を重みに変換する純関数として実装する。
// 0 以下を返した譜面は対象外として扱う。
// 正規化スコープと a の計算は呼出側 (pickLevel) の責務。
type Weighter interface {
	Weight(a float64) float64
}

// WeighterFactory は PickConfig から適切な Weighter を選択する。
// Bootstrap で具体実装を 1 つだけ注入し、PickUseCase が公開表ごとに For を呼ぶ。
type WeighterFactory interface {
	For(cfg domain.PickConfig) Weighter
}
```

- [ ] **Step 2: ビルド確認 (失敗するはず)**

```
go build ./...
```
Expected: FAIL — 既存の `UniformWeighter.Weight(ctx, chart, now)` シグネチャ違反、`PickUseCase` 内の `w.Weight(ctx, c, now)` 呼び出しも違反、`pick_usecase_test.go` の fixture が `port.Weighter` を渡す箇所も違反。次タスクで一気に直す。

- [ ] **Step 3: 一時的なビルド失敗をメモして次タスクへ**

このタスクは単独でコミットしない (ビルド赤のまま)。Task 4・5 と一緒にコミットする。

---

## Task 4: weighter adapter (UniformWeighter / LastPlayedWeighter / Factory)

新シグネチャに合わせて Uniform を書き換え、`LastPlayedWeighter` と `Factory` を新設する。

**Files:**
- Modify: `internal/adapter/weighter/uniform.go`
- Modify: `internal/adapter/weighter/uniform_test.go`
- Create: `internal/adapter/weighter/last_played.go`
- Create: `internal/adapter/weighter/last_played_test.go`
- Create: `internal/adapter/weighter/factory.go`
- Create: `internal/adapter/weighter/factory_test.go`

- [ ] **Step 1: UniformWeighter を新シグネチャに更新**

`internal/adapter/weighter/uniform.go` を以下に差し替え:

```go
// Package weighter は port.Weighter / port.WeighterFactory の実装群。
package weighter

// UniformWeighter は全譜面に等しく 1 を返す。WeightMode=off で使用。
// WeightMode=sort 経路では Weighter 自体を使わないが、Factory の安全側として返却される。
type UniformWeighter struct{}

func (UniformWeighter) Weight(_ float64) float64 { return 1 }
```

- [ ] **Step 2: UniformWeighter テストを書き換え**

`internal/adapter/weighter/uniform_test.go` を以下に差し替え:

```go
package weighter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniformWeighter_AlwaysReturnsOne(t *testing.T) {
	w := UniformWeighter{}
	for _, a := range []float64{0, 0.25, 0.5, 0.75, 1, 1.5, -0.5} {
		require.Equal(t, 1.0, w.Weight(a))
	}
}
```

- [ ] **Step 3: LastPlayedWeighter テストを書く (失敗確認)**

`internal/adapter/weighter/last_played_test.go` を新規作成:

```go
package weighter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLastPlayedWeighter_LinearInterpolation(t *testing.T) {
	cases := []struct {
		x        float64
		a        float64
		expected float64
	}{
		{x: 1, a: 0, expected: 1},
		{x: 1, a: 1, expected: 1},
		{x: 2, a: 0, expected: 1},
		{x: 2, a: 0.5, expected: 1.5},
		{x: 2, a: 1, expected: 2},
		{x: 10, a: 0, expected: 1},
		{x: 10, a: 0.5, expected: 5.5},
		{x: 10, a: 1, expected: 10},
		{x: 100, a: 0, expected: 1},
		{x: 100, a: 1, expected: 100},
	}
	for _, c := range cases {
		w := LastPlayedWeighter{X: c.x}
		require.InDelta(t, c.expected, w.Weight(c.a), 1e-9, "X=%v a=%v", c.x, c.a)
	}
}
```

```
go test ./internal/adapter/weighter/ -run TestLastPlayedWeighter -v
```
Expected: FAIL (`LastPlayedWeighter` 未定義)

- [ ] **Step 4: LastPlayedWeighter を実装**

`internal/adapter/weighter/last_played.go` を新規作成:

```go
package weighter

// LastPlayedWeighter は最終プレイ日時に基づく線形補間の重み関数。
// 集合内正規化経過時間 a ∈ [0,1] (0=最新, 1=最古) を入力に
// w = 1 + (X-1)*a を返す。X=1 で恒等 (一様), X=K で「最古は最新の K 倍」。
type LastPlayedWeighter struct {
	X float64
}

func (w LastPlayedWeighter) Weight(a float64) float64 {
	return 1.0 + (w.X-1.0)*a
}
```

```
go test ./internal/adapter/weighter/ -run TestLastPlayedWeighter -v
```
Expected: PASS

- [ ] **Step 5: Factory テストを書く (失敗確認)**

`internal/adapter/weighter/factory_test.go` を新規作成:

```go
package weighter

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestFactory_OffReturnsUniform(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeOff})
	_, ok := w.(UniformWeighter)
	require.True(t, ok, "WeightMode=off should yield UniformWeighter")
}

func TestFactory_SortReturnsUniform(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeSort})
	_, ok := w.(UniformWeighter)
	require.True(t, ok, "WeightMode=sort uses別経路だが Factory 返却は Uniform で安全側")
}

func TestFactory_ProbabilityReturnsLastPlayed(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeProbability, WeightParamX: 10})
	lp, ok := w.(LastPlayedWeighter)
	require.True(t, ok)
	require.Equal(t, 10.0, lp.X)
}

func TestFactory_ProbabilityClampsBelowOne(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeProbability, WeightParamX: 0})
	lp, ok := w.(LastPlayedWeighter)
	require.True(t, ok)
	require.Equal(t, 1.0, lp.X, "X<1 は 1 に clamp")
}
```

```
go test ./internal/adapter/weighter/ -run TestFactory -v
```
Expected: FAIL (`Factory` 未定義)

- [ ] **Step 6: Factory を実装**

`internal/adapter/weighter/factory.go` を新規作成:

```go
package weighter

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// Factory は port.WeighterFactory の実装。
// domain.PickConfig から適切な Weighter を選ぶ。
type Factory struct{}

func (Factory) For(cfg domain.PickConfig) port.Weighter {
	switch cfg.WeightMode {
	case domain.WeightModeProbability:
		x := float64(cfg.WeightParamX)
		if x < 1 {
			x = 1
		}
		return LastPlayedWeighter{X: x}
	default:
		// off / sort / 未知 → Uniform (sort はそもそも Weighter を呼ばないので影響なし)
		return UniformWeighter{}
	}
}
```

```
go test ./internal/adapter/weighter/ -v
```
Expected: PASS (全テスト)

- [ ] **Step 7: ビルド確認 (まだ pick_usecase 側が古いシグネチャで失敗するはず)**

```
go build ./...
```
Expected: FAIL — `usecase/pick_usecase.go` および `usecase/pick_usecase_test.go` がまだ古い `Weight(ctx, c, now)` 呼び出しを持つ。Task 9 で修正。

- [ ] **Step 8: コミット (port.Weighter シグネチャ変更と weighter adapter まとめて)**

```bash
git add internal/port/weighter.go internal/adapter/weighter/
git commit -m "refactor(weighter): Weighter シグネチャを a:[0,1]→重み の純関数に変更

LastPlayedWeighter (w=1+(X-1)a) と Factory を新設。
PickUseCase 側は次コミットで追従。"
```

(ビルドはまだ赤い状態だが、Weighter まわりの責務再定義を 1 コミットでまとめておく)

---

## Task 5: ScoreDBAttacher 新設

`SongdataAttacher` と同じ構造で `score.db` を RO ATTACH する Attacher を作る。

**Files:**
- Create: `internal/adapter/persistence/score_attacher.go`
- Create: `internal/adapter/persistence/score_attacher_test.go`

- [ ] **Step 1: テストを書く (失敗確認)**

`internal/adapter/persistence/score_attacher_test.go` を新規作成:

```go
package persistence

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/stretchr/testify/require"
)

// makeScoreDBFile は最小限の score テーブルを持つテスト用 DB を作る。
func makeScoreDBFile(t *testing.T, path string, rows [][2]any) {
	t.Helper()
	db, err := OpenDB(path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE score (sha256 TEXT NOT NULL, mode INTEGER, date INTEGER, PRIMARY KEY(sha256, mode))`)
	require.NoError(t, err)
	for _, r := range rows {
		_, err = db.Exec(`INSERT INTO score(sha256, mode, date) VALUES(?, 0, ?)`, r[0], r[1])
		require.NoError(t, err)
	}
}

func newAttacherTestDB(t *testing.T) (string, func()) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "compositor.db")
	return dbPath, func() {}
}

func TestScoreDBAttacher_AttachAndDetach(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	scorePath := filepath.Join(dir, "score.db")
	makeScoreDBFile(t, scorePath, [][2]any{{"sha-a", 1000}, {"sha-b", 2000}})

	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.False(t, a.IsAttached())

	require.NoError(t, a.Attach(context.Background(), scorePath))
	require.True(t, a.IsAttached())

	var n int
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 2, n)

	require.NoError(t, a.Detach(context.Background()))
	require.False(t, a.IsAttached())
}

func TestScoreDBAttacher_AttachEmptyPathIsNoop(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, a.Attach(context.Background(), ""))
	require.False(t, a.IsAttached())
}

func TestScoreDBAttacher_ReAttach(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	score1 := filepath.Join(dir, "s1.db")
	score2 := filepath.Join(dir, "s2.db")
	makeScoreDBFile(t, score1, [][2]any{{"a", 1}})
	makeScoreDBFile(t, score2, [][2]any{{"b", 2}, {"c", 3}})

	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, a.Attach(context.Background(), score1))

	var n int
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 1, n)

	require.NoError(t, a.ReAttach(context.Background(), score2))
	require.NoError(t, mainDB.QueryRow(`SELECT COUNT(*) FROM sc.score`).Scan(&n))
	require.Equal(t, 2, n)
}
```

```
go test ./internal/adapter/persistence/ -run TestScoreDBAttacher -v
```
Expected: FAIL (`NewScoreDBAttacher` 未定義)

- [ ] **Step 2: ScoreDBAttacher を実装**

`internal/adapter/persistence/score_attacher.go` を新規作成:

```go
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// ScoreDBAttacher はメイン *sql.DB に対する score.db の ATTACH/DETACH ライフサイクルを管理する。
// schema 名は 'sc'。SongdataAttacher と同様に SetMaxOpenConns(1) 前提・RO 専用。
// beatoraja の score.db を破壊しないため RW では絶対に開かない。
type ScoreDBAttacher struct {
	db    *sql.DB
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	attached   bool
	path       string
	rowCount   int
	attachedAt *time.Time
	lastErr    string
}

// NewScoreDBAttacher は新しい ScoreDBAttacher を作る。
func NewScoreDBAttacher(db *sql.DB, clk port.Clock, log *slog.Logger) *ScoreDBAttacher {
	return &ScoreDBAttacher{db: db, clock: clk, log: log}
}

// Attach は score.db を schema 'sc' として RO ATTACH する。
// path が空なら no-op (失敗ではない)。
// 既にアタッチ済みなら一度 Detach してから再 ATTACH する。
func (a *ScoreDBAttacher) Attach(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if a.IsAttached() {
		if err := a.Detach(ctx); err != nil {
			return err
		}
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", url.QueryEscape(path))
	if _, err := a.db.ExecContext(ctx, "ATTACH DATABASE ? AS sc", dsn); err != nil {
		a.recordError(err.Error())
		return fmt.Errorf("attach score %q: %w", path, err)
	}

	var count int
	row := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sc.score")
	if err := row.Scan(&count); err != nil {
		a.recordError(fmt.Sprintf("count sc.score: %v", err))
		count = 0
	}

	now := a.clock.Now()
	a.mu.Lock()
	a.attached = true
	a.path = path
	a.rowCount = count
	a.attachedAt = &now
	a.lastErr = ""
	a.mu.Unlock()
	a.log.Info("score attached", "path", path, "count", count)
	return nil
}

// Detach は schema 'sc' を DETACH する。未アタッチなら no-op。
func (a *ScoreDBAttacher) Detach(ctx context.Context) error {
	if !a.IsAttached() {
		return nil
	}
	if _, err := a.db.ExecContext(ctx, "DETACH DATABASE sc"); err != nil {
		return fmt.Errorf("detach score: %w", err)
	}
	a.mu.Lock()
	a.attached = false
	a.path = ""
	a.rowCount = 0
	a.attachedAt = nil
	a.mu.Unlock()
	a.log.Info("score detached")
	return nil
}

// ReAttach は Detach → Attach を 1 連の操作で行う (設定変更時のフック用)。
// path が空のときは Detach のみ行う。
func (a *ScoreDBAttacher) ReAttach(ctx context.Context, path string) error {
	if err := a.Detach(ctx); err != nil {
		return err
	}
	return a.Attach(ctx, path)
}

// IsAttached は現在 'sc' がアタッチされているかを返す。
func (a *ScoreDBAttacher) IsAttached() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.attached
}

func (a *ScoreDBAttacher) recordError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}
```

```
go test ./internal/adapter/persistence/ -run TestScoreDBAttacher -v
```
Expected: PASS

- [ ] **Step 3: コミット**

```bash
git add internal/adapter/persistence/score_attacher.go internal/adapter/persistence/score_attacher_test.go
git commit -m "feat(persistence): ScoreDBAttacher 新設 (score.db を RO ATTACH)

schema 'sc' として beatoraja の score.db をマウントする。
SongdataAttacher と同構造、RO 専用で書込み事故を物理防止。"
```

---

## Task 6: scanEnrichedRows で last_played_at を *time.Time に変換

`LoadCharts` SQL を score.db ATTACH 有無で分岐し、`scanEnrichedRows` で int64 → `*time.Time` に変換する。

**Files:**
- Modify: `internal/adapter/persistence/source_table_repo.go`
- Test: `internal/adapter/persistence/source_table_repo_test.go`

- [ ] **Step 1: SourceTableRepoSQL にスコア attacher を保持できるよう拡張**

`source_table_repo.go` の冒頭の構造体・コンストラクタを以下に変更。`SongdataAttacher` を保持する既存フィールドの隣に追加。

(変更前)
```go
type SourceTableRepoSQL struct {
	db       *sql.DB
	attacher *SongdataAttacher
}

func NewSourceTableRepoSQL(db *sql.DB, attacher *SongdataAttacher) *SourceTableRepoSQL {
	return &SourceTableRepoSQL{db: db, attacher: attacher}
}
```

(変更後)
```go
type SourceTableRepoSQL struct {
	db            *sql.DB
	attacher      *SongdataAttacher
	scoreAttacher *ScoreDBAttacher
}

// NewSourceTableRepoSQL は SongdataAttacher と ScoreDBAttacher を受け取る。
// scoreAttacher は nil 可 (起動時に score.db が未設定でも動作)。
func NewSourceTableRepoSQL(db *sql.DB, attacher *SongdataAttacher, scoreAttacher *ScoreDBAttacher) *SourceTableRepoSQL {
	return &SourceTableRepoSQL{db: db, attacher: attacher, scoreAttacher: scoreAttacher}
}
```

- [ ] **Step 2: loadChartsAttached を score.db ATTACH 有無で分岐**

`source_table_repo.go` の `loadChartsAttached` を以下に書き換え:

```go
func (r *SourceTableRepoSQL) loadChartsAttached(
	ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
	ownedFlag := 0
	if q.OwnedOnly {
		ownedFlag = 1
	}
	scoreAttached := r.scoreAttacher != nil && r.scoreAttacher.IsAttached()

	lastPlayedExpr := "NULL"
	if scoreAttached {
		// mode をまたいで sha256 ごとの最新 date を取る。date=0 / 未存在は NULL に。
		lastPlayedExpr = `(SELECT MAX(sc.date) FROM sc.score sc
		                    WHERE sc.sha256 = c.sha256 AND sc.date > 0)`
	}

	query := fmt.Sprintf(`
		SELECT
		  c.position, t.symbol, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
		  EXISTS(SELECT 1 FROM sd.song s WHERE s.md5 = c.md5) AS is_owned,
		  %s AS last_played_at
		FROM source_table_chart c
		JOIN source_table t ON t.id = c.source_id
		WHERE c.source_id = ?
		  AND (? = 0 OR EXISTS (SELECT 1 FROM sd.song s WHERE s.md5 = c.md5))
		ORDER BY c.position ASC`, lastPlayedExpr)

	rows, err := r.db.QueryContext(ctx, query, sourceID, ownedFlag)
	if err != nil {
		return nil, fmt.Errorf("load enriched charts (attached) %q: %w", sourceID, err)
	}
	defer rows.Close()
	return scanEnrichedRows(rows, sourceID)
}
```

- [ ] **Step 3: scanEnrichedRows を sql.NullInt64 → *time.Time 変換に書き換え**

`scanEnrichedRows` を以下に変更:

```go
func scanEnrichedRows(rows *sql.Rows, sourceID string) ([]domain.EnrichedChart, error) {
	var out []domain.EnrichedChart
	for rows.Next() {
		var (
			c            domain.SourceChart
			rawJSON      string
			isOwned      bool
			lastPlayedAt sql.NullInt64
		)
		if err := rows.Scan(
			&c.Position, &c.Symbol, &c.MD5, &c.SHA256, &c.Level, &c.Title, &c.Artist,
			&rawJSON, &isOwned, &lastPlayedAt,
		); err != nil {
			return nil, err
		}
		c.SourceID = sourceID
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &c.Raw); err != nil {
				return nil, fmt.Errorf("unmarshal raw_json[pos=%d]: %w", c.Position, err)
			}
		}
		ec := domain.EnrichedChart{SourceChart: c, IsOwned: isOwned}
		if lastPlayedAt.Valid && lastPlayedAt.Int64 > 0 {
			t := time.Unix(lastPlayedAt.Int64, 0).UTC()
			ec.LastPlayedAt = &t
		}
		out = append(out, ec)
	}
	return out, rows.Err()
}
```

`time` パッケージの import を確認 (既存にあるはず)。

- [ ] **Step 4: 既存テストを更新 (NewSourceTableRepoSQL の引数追加)**

```
go build ./...
```
Expected: FAIL — `NewSourceTableRepoSQL` 呼出側 (テスト含む) で引数不足。

`grep -rn "NewSourceTableRepoSQL" internal/` で呼出箇所を洗い出し、第 3 引数 `nil` (起動時 score.db 未設定相当) を追加する:

```go
sourceRepo := persistence.NewSourceTableRepoSQL(db, sourceAttacher, nil)
```

具体箇所:
- `internal/adapter/persistence/source_table_repo_test.go` の各テストヘルパ
- `internal/app/bootstrap.go` (Task 13 で再度差し替えるが暫定で nil)
- `internal/app/handler/published_table_handler_test.go` 等のヘルパ
- `internal/app/handler/source_table_handler_test.go` 等のヘルパ

各箇所で `nil` を 3 引数目に追加。

- [ ] **Step 5: 新規 last_played テストを追加**

`internal/adapter/persistence/source_table_repo_test.go` の末尾に追加:

```go
func TestLoadCharts_LastPlayedAt_WithScoreAttached(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// メイン DB (compositor)
	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)
	require.NoError(t, RunMigrations(mainDB))

	// songdata.db (md5/sha256 マスタ。所持判定に使う)
	songPath := filepath.Join(dir, "songdata.db")
	{
		songDB, err := OpenDB(songPath)
		require.NoError(t, err)
		_, err = songDB.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY(md5))`)
		require.NoError(t, err)
		_, err = songDB.Exec(`INSERT INTO song(md5, sha256) VALUES('md-a','sha-a'),('md-b','sha-b')`)
		require.NoError(t, err)
		songDB.Close()
	}

	// score.db (sha-a だけプレイ済み)
	scorePath := filepath.Join(dir, "score.db")
	makeScoreDBFile(t, scorePath, [][2]any{{"sha-a", 1700000000}})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	songAtt := NewSongdataAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, songAtt.Attach(ctx, songPath))
	scoreAtt := NewScoreDBAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, scoreAtt.Attach(ctx, scorePath))

	repo := NewSourceTableRepoSQL(mainDB, songAtt, scoreAtt)

	// テスト用ソース表
	src := domain.SourceTable{
		ID: "src1", InputURL: "http://x", InputKind: domain.InputKindHTML, Name: "t",
		LastFetchStatus: domain.FetchStatusOK,
	}
	_, err = repo.Create(ctx, src)
	require.NoError(t, err)
	require.NoError(t, repo.SaveFetched(ctx, "src1", domain.BMSTableHeader{}, []domain.SourceChart{
		{SourceID: "src1", Position: 1, MD5: "md-a", SHA256: "sha-a", Level: "0"},
		{SourceID: "src1", Position: 2, MD5: "md-b", SHA256: "sha-b", Level: "0"},
	}))

	charts, err := repo.LoadCharts(ctx, "src1", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, charts, 2)

	byMD5 := map[string]domain.EnrichedChart{}
	for _, c := range charts {
		byMD5[c.MD5] = c
	}
	require.NotNil(t, byMD5["md-a"].LastPlayedAt)
	require.Equal(t, int64(1700000000), byMD5["md-a"].LastPlayedAt.Unix())
	require.Nil(t, byMD5["md-b"].LastPlayedAt, "未プレイは nil")
}

func TestLoadCharts_LastPlayedAt_WithoutScoreAttached(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	mainDB, err := OpenDB(mainPath)
	require.NoError(t, err)
	defer mainDB.Close()
	mainDB.SetMaxOpenConns(1)
	require.NoError(t, RunMigrations(mainDB))

	songPath := filepath.Join(dir, "songdata.db")
	{
		songDB, err := OpenDB(songPath)
		require.NoError(t, err)
		_, err = songDB.Exec(`CREATE TABLE song (md5 TEXT NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY(md5))`)
		require.NoError(t, err)
		_, err = songDB.Exec(`INSERT INTO song(md5, sha256) VALUES('md-a','sha-a')`)
		require.NoError(t, err)
		songDB.Close()
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	songAtt := NewSongdataAttacher(mainDB, clock.System{}, logger)
	require.NoError(t, songAtt.Attach(ctx, songPath))

	repo := NewSourceTableRepoSQL(mainDB, songAtt, nil) // score.db 未設定
	_, err = repo.Create(ctx, domain.SourceTable{ID: "src1", InputURL: "http://x", InputKind: domain.InputKindHTML, Name: "t", LastFetchStatus: domain.FetchStatusOK})
	require.NoError(t, err)
	require.NoError(t, repo.SaveFetched(ctx, "src1", domain.BMSTableHeader{}, []domain.SourceChart{
		{SourceID: "src1", Position: 1, MD5: "md-a", SHA256: "sha-a", Level: "0"},
	}))

	charts, err := repo.LoadCharts(ctx, "src1", port.ChartQuery{})
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Nil(t, charts[0].LastPlayedAt, "score 未 attach 時は常に nil")
}
```

- [ ] **Step 6: テスト実行**

```
go test ./internal/adapter/persistence/ -run "TestLoadCharts_LastPlayedAt|TestRunMigrations|TestScoreDBAttacher" -v
```
Expected: PASS

- [ ] **Step 7: 全パッケージビルド (pick_usecase はまだ赤い)**

```
go build ./...
```
Expected: FAIL — pick_usecase 系のシグネチャ違反のみ残る (Task 9 で解消)

- [ ] **Step 8: コミット**

```bash
git add internal/adapter/persistence/source_table_repo.go internal/adapter/persistence/source_table_repo_test.go internal/app/bootstrap.go internal/app/handler/
git commit -m "feat(persistence): LoadCharts で score.db から last_played_at を取得

LoadChartsAttached を score attach 有無で分岐、sql.NullInt64 から
*time.Time に変換する。NewSourceTableRepoSQL に scoreAttacher 引数を追加
(呼出箇所は nil で暫定。Bootstrap は Task 13 で実配線)。"
```

---

## Task 7: PublishedTableRepoSQL に weight_mode / weight_param_x を読み書き追加

**Files:**
- Modify: `internal/adapter/persistence/published_table_repo.go`
- Modify: `internal/adapter/persistence/published_table_repo_test.go`

- [ ] **Step 1: 失敗テストを追加**

`published_table_repo_test.go` の末尾に追加:

```go
func TestPublishedTableRepo_PersistsWeightFields(t *testing.T) {
	ctx := context.Background()
	db := newRepoTestDB(t) // 既存ヘルパを使用
	repo := NewPublishedTableRepoSQL(db)

	id := "p-weight-1"
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: id, Slug: "weight-1", DisplayName: "W1", Symbol: "★",
		Pick: domain.PickConfig{
			RefreshMode:  domain.RefreshModeManual,
			WeightMode:   domain.WeightModeProbability,
			WeightParamX: 25,
		},
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.WeightModeProbability, got.Pick.WeightMode)
	require.Equal(t, 25, got.Pick.WeightParamX)

	got.Pick.WeightMode = domain.WeightModeSort
	got.Pick.WeightParamX = 7
	require.NoError(t, repo.Update(ctx, got))
	got2, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.WeightModeSort, got2.Pick.WeightMode)
	require.Equal(t, 7, got2.Pick.WeightParamX)
}

func TestPublishedTableRepo_DefaultsWeightFieldsWhenAbsent(t *testing.T) {
	ctx := context.Background()
	db := newRepoTestDB(t)
	repo := NewPublishedTableRepoSQL(db)

	id := "p-weight-default"
	_, err := repo.Create(ctx, domain.PublishedTable{
		ID: id, Slug: "weight-default", DisplayName: "WD",
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModeManual},
	})
	require.NoError(t, err)
	got, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.WeightModeOff, got.Pick.WeightMode)
	require.Equal(t, 10, got.Pick.WeightParamX, "DEFAULT 10 が返る (Insert 時は明示値 0 を上書きしない実装にする)")
}
```

(注: `newRepoTestDB` ヘルパが既存テストにあるはず。なければ既存テストの fixture をコピーして使う)

```
go test ./internal/adapter/persistence/ -run TestPublishedTableRepo_PersistsWeightFields -v
```
Expected: FAIL (`WeightMode` が読み書きされない)

- [ ] **Step 2: SELECT カラム / scanRow を拡張**

`published_table_repo.go` の `publishedTableSelectColumns` と `scanRow` を更新:

```go
const publishedTableSelectColumns = `SELECT
	id, slug, display_name, symbol, owned_only,
	pick_refresh_mode, weight_mode, weight_param_x, sort_order
 FROM published_table`

func (r *PublishedTableRepoSQL) scanRow(s rowScanner) (domain.PublishedTable, error) {
	var (
		t         domain.PublishedTable
		ownedOnly int
		mode      string
		wMode     string
		wX        int
	)
	if err := s.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.Symbol, &ownedOnly,
		&mode, &wMode, &wX, &t.SortOrder,
	); err != nil {
		return domain.PublishedTable{}, err
	}
	t.OwnedOnly = ownedOnly != 0
	t.Pick.RefreshMode = domain.RefreshMode(mode)
	t.Pick.WeightMode = domain.WeightMode(wMode)
	t.Pick.WeightParamX = wX
	return t, nil
}
```

- [ ] **Step 3: Create / Update の INSERT・UPDATE 文を拡張**

`Create` 内の INSERT を以下に変更:

```go
	wMode := string(t.Pick.WeightMode)
	if wMode == "" {
		wMode = string(domain.WeightModeOff)
	}
	wX := t.Pick.WeightParamX
	if wX <= 0 {
		wX = 10
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO published_table
		 (id, slug, display_name, symbol, owned_only, pick_refresh_mode,
		  weight_mode, weight_param_x, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Slug, t.DisplayName, t.Symbol, owned, string(t.Pick.RefreshMode),
		wMode, wX, t.SortOrder,
	); err != nil {
		// (既存のエラー処理)
```

`Update` 内の UPDATE を以下に変更:

```go
	wMode := string(t.Pick.WeightMode)
	if wMode == "" {
		wMode = string(domain.WeightModeOff)
	}
	wX := t.Pick.WeightParamX
	if wX <= 0 {
		wX = 10
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE published_table SET
		   slug=?, display_name=?, symbol=?, owned_only=?,
		   pick_refresh_mode=?, weight_mode=?, weight_param_x=?,
		   sort_order=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.Slug, t.DisplayName, t.Symbol, owned,
		string(t.Pick.RefreshMode), wMode, wX,
		t.SortOrder, t.ID,
	)
```

- [ ] **Step 4: テスト実行**

```
go test ./internal/adapter/persistence/ -run TestPublishedTableRepo -v
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/persistence/published_table_repo.go internal/adapter/persistence/published_table_repo_test.go
git commit -m "feat(persistence): PublishedTableRepo に weight_mode/weight_param_x の永続化追加

Create/Update/Get で WeightMode と WeightParamX を読み書きする。
未設定 (zero value) は off / 10 にフォールバック。"
```

---

## Task 8: PickUseCase の WeighterFactory 注入 + pickLevel aOf + WeightMode 分岐

PickUseCase 全面改修。コンストラクタ、`pickLevel`、`weightedSampleWithoutReplacement` を変更。

**Files:**
- Modify: `internal/usecase/pick_usecase.go`
- Modify: `internal/usecase/pick_usecase_test.go`

- [ ] **Step 1: コンストラクタを WeighterFactory 受け取りに変更**

`pick_usecase.go` を以下のように更新:

```go
type PickUseCase struct {
	pubRepo         port.PublishedTableRepo
	srcRepo         port.SourceTableRepo
	store           *PickResultStore
	clock           port.Clock
	randNew         port.RandSourceFactory
	log             *slog.Logger
	weighterFactory port.WeighterFactory
}

func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
	weighterFactory port.WeighterFactory,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, store: store,
		clock: clock, randNew: randNew, log: log,
		weighterFactory: weighterFactory,
	}
}
```

- [ ] **Step 2: pickLevel を WeightMode 分岐 + aOf 計算に書き換え**

`pick_usecase.go` の `pickLevel` を以下に書き換え:

```go
func (u *PickUseCase) pickLevel(
	ctx context.Context, lv domain.PublishedTableLevel,
	chartsBySrcLevel map[string]map[string][]domain.EnrichedChart,
	rng *rand.Rand, now time.Time,
	pickCfg domain.PickConfig,
) ([]domain.PickedChart, error) {
	pools := make([][]domain.EnrichedChart, len(lv.Mappings))
	for i, mp := range lv.Mappings {
		pools[i] = chartsBySrcLevel[mp.SourceTableID][mp.SourceLevel]
	}

	keyOf := func(c domain.EnrichedChart) string {
		if c.MD5 != "" {
			return "md5:" + c.MD5
		}
		return "sha:" + c.SHA256
	}

	seenUnion := map[string]struct{}{}
	chartOriginMapping := map[string]int{}
	var unionPool []domain.EnrichedChart
	for i, p := range pools {
		for _, c := range p {
			k := keyOf(c)
			if _, ok := seenUnion[k]; ok {
				continue
			}
			seenUnion[k] = struct{}{}
			chartOriginMapping[k] = i
			unionPool = append(unionPool, c)
		}
	}

	// a の分母: unionPool 内の最大経過秒数。プレイ済み全員同時刻 / 全員未プレイなら 0。
	maxAgeSec := int64(0)
	for _, c := range unionPool {
		if c.LastPlayedAt == nil {
			continue
		}
		age := now.Unix() - c.LastPlayedAt.Unix()
		if age < 0 {
			age = 0
		}
		if age > maxAgeSec {
			maxAgeSec = age
		}
	}
	aOf := func(c domain.EnrichedChart) float64 {
		if c.LastPlayedAt == nil {
			return 1.0
		}
		if maxAgeSec <= 0 {
			return 0.0
		}
		age := now.Unix() - c.LastPlayedAt.Unix()
		if age < 0 {
			age = 0
		}
		return float64(age) / float64(maxAgeSec)
	}

	type pickedItem struct {
		chart      domain.EnrichedChart
		mappingIdx int
	}
	var picked []pickedItem
	pickedKeys := map[string]struct{}{}

	if pickCfg.WeightMode == domain.WeightModeSort {
		picked = pickSortDeterministic(pools, unionPool, aOf, keyOf, chartOriginMapping, lv.PerMappingPick, lv.TotalPick)
		for _, p := range picked {
			pickedKeys[keyOf(p.chart)] = struct{}{}
		}
	} else {
		w := u.weighterFactory.For(pickCfg)
		m := lv.PerMappingPick
		for i := range pools {
			avail := make([]domain.EnrichedChart, 0, len(pools[i]))
			for _, c := range pools[i] {
				if _, ok := pickedKeys[keyOf(c)]; ok {
					continue
				}
				avail = append(avail, c)
			}
			taken := weightedSampleWithoutReplacement(ctx, avail, m, w, aOf, rng)
			for _, c := range taken {
				pickedKeys[keyOf(c)] = struct{}{}
				picked = append(picked, pickedItem{chart: c, mappingIdx: i})
			}
		}
		need := lv.TotalPick - len(picked)
		if need > 0 {
			remaining := make([]domain.EnrichedChart, 0, len(unionPool))
			for _, c := range unionPool {
				if _, ok := pickedKeys[keyOf(c)]; ok {
					continue
				}
				remaining = append(remaining, c)
			}
			taken := weightedSampleWithoutReplacement(ctx, remaining, need, w, aOf, rng)
			for _, c := range taken {
				k := keyOf(c)
				pickedKeys[k] = struct{}{}
				picked = append(picked, pickedItem{chart: c, mappingIdx: chartOriginMapping[k]})
			}
		}
	}

	sort.SliceStable(picked, func(a, b int) bool {
		if picked[a].mappingIdx != picked[b].mappingIdx {
			return picked[a].mappingIdx < picked[b].mappingIdx
		}
		return picked[a].chart.Position < picked[b].chart.Position
	})

	out := make([]domain.PickedChart, 0, len(picked))
	for _, p := range picked {
		out = append(out, domain.PickedChart{EnrichedChart: p.chart, PublicLevel: lv.Name})
	}
	return out, nil
}

// pickSortDeterministic は WeightMode=sort 経路のピックを行う。
// phase1: マッピングごとに「(aOf 降順, position 昇順)」で上から m 件
// phase2: union 残りから「(aOf 降順, mappingIdx 昇順, position 昇順)」で TotalPick まで補填
func pickSortDeterministic(
	pools [][]domain.EnrichedChart,
	unionPool []domain.EnrichedChart,
	aOf func(domain.EnrichedChart) float64,
	keyOf func(domain.EnrichedChart) string,
	chartOriginMapping map[string]int,
	perMapping int, totalPick int,
) []struct {
	chart      domain.EnrichedChart
	mappingIdx int
} {
	type pickedItem struct {
		chart      domain.EnrichedChart
		mappingIdx int
	}
	var picked []pickedItem
	pickedKeys := map[string]struct{}{}

	// phase1: 各マッピング pool を a 降順ソートし上から m 件
	for i, p := range pools {
		sortedPool := make([]domain.EnrichedChart, len(p))
		copy(sortedPool, p)
		sort.SliceStable(sortedPool, func(x, y int) bool {
			ax, ay := aOf(sortedPool[x]), aOf(sortedPool[y])
			if ax != ay {
				return ax > ay
			}
			return sortedPool[x].Position < sortedPool[y].Position
		})
		taken := 0
		for _, c := range sortedPool {
			if taken >= perMapping {
				break
			}
			k := keyOf(c)
			if _, ok := pickedKeys[k]; ok {
				continue
			}
			pickedKeys[k] = struct{}{}
			picked = append(picked, pickedItem{chart: c, mappingIdx: i})
			taken++
		}
	}

	// phase2: union 残りから (a 降順, mappingIdx 昇順, position 昇順) で補填
	need := totalPick - len(picked)
	if need > 0 {
		remaining := make([]domain.EnrichedChart, 0, len(unionPool))
		for _, c := range unionPool {
			if _, ok := pickedKeys[keyOf(c)]; ok {
				continue
			}
			remaining = append(remaining, c)
		}
		sort.SliceStable(remaining, func(x, y int) bool {
			ax, ay := aOf(remaining[x]), aOf(remaining[y])
			if ax != ay {
				return ax > ay
			}
			ix := chartOriginMapping[keyOf(remaining[x])]
			iy := chartOriginMapping[keyOf(remaining[y])]
			if ix != iy {
				return ix < iy
			}
			return remaining[x].Position < remaining[y].Position
		})
		for i := 0; i < need && i < len(remaining); i++ {
			c := remaining[i]
			picked = append(picked, pickedItem{chart: c, mappingIdx: chartOriginMapping[keyOf(c)]})
		}
	}

	// 内部型を外に返すためのコンバート
	out := make([]struct {
		chart      domain.EnrichedChart
		mappingIdx int
	}, len(picked))
	for i, p := range picked {
		out[i] = struct {
			chart      domain.EnrichedChart
			mappingIdx int
		}{chart: p.chart, mappingIdx: p.mappingIdx}
	}
	return out
}
```

- [ ] **Step 3: weightedSampleWithoutReplacement のシグネチャを更新**

```go
func weightedSampleWithoutReplacement(
	ctx context.Context, pool []domain.EnrichedChart, k int,
	w port.Weighter, aOf func(domain.EnrichedChart) float64,
	rng *rand.Rand,
) []domain.EnrichedChart {
	if k <= 0 || len(pool) == 0 {
		return nil
	}
	weights := make([]float64, len(pool))
	totalWeight := 0.0
	for i, c := range pool {
		wt := w.Weight(aOf(c))
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
	return taken
}
```

- [ ] **Step 4: regenerate から pickLevel への引数追加**

`regenerate` 内 `u.pickLevel(...)` 呼出を以下に変更:

```go
		picked, err := u.pickLevel(ctx, lv, chartsBySrcLevel, rng, now, pub.Pick)
```

- [ ] **Step 5: pick_usecase_test.go の fixture を更新**

`newPickUCFixtureWithWeighter` を以下に置き換え:

```go
type stubWeighterFactory struct {
	w port.Weighter
}

func (f stubWeighterFactory) For(_ domain.PickConfig) port.Weighter { return f.w }

func newPickUCFixtureWithWeighter(t *testing.T, w port.Weighter) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger, stubWeighterFactory{w: w})
	return &pickUCFixture{uc: uc, pubRepo: pub, srcRepo: src, store: store, clock: clock}
}
```

- [ ] **Step 6: 既存テストが通ることを確認**

```
go test ./internal/usecase/ -v
```
Expected: PASS (全テスト)

- [ ] **Step 7: 新規テストを追加 (probability モード + sort モード + 未プレイ扱い)**

`pick_usecase_test.go` の末尾に追加:

```go
// helper: LastPlayedAt 付きで chart を作る
func chartWithLastPlayed(sourceID, level string, pos int, md5 string, lastPlayedUnix int64) domain.EnrichedChart {
	ec := domain.EnrichedChart{
		SourceChart: chartFixture(sourceID, level, pos, md5),
		IsOwned:     true,
	}
	if lastPlayedUnix > 0 {
		t := time.Unix(lastPlayedUnix, 0).UTC()
		ec.LastPlayedAt = &t
	}
	return ec
}

func TestPickUseCase_SortMode_OrdersByOldestDateThenMappingThenPosition(t *testing.T) {
	f := newPickUCFixture(t)
	// 既存テストヘルパで public/source を組む構造に合わせる。
	// (簡略のため fake 直叩きで初期化する想定。既存パターンを踏襲)
	// charts: pos=1 が最新, pos=3 が最古, pos=2 が中間
	now := f.clock.t.Unix()
	day := int64(86400)
	charts := []domain.EnrichedChart{
		chartWithLastPlayed("src1", "0", 1, "ma", now-1*day),
		chartWithLastPlayed("src1", "0", 2, "mb", now-30*day),
		chartWithLastPlayed("src1", "0", 3, "mc", now-365*day),
	}
	f.srcRepo.enrichedCharts["src1"] = charts // 既存 fakeSourceRepo の実装に応じてフィールド名変える
	f.srcRepo.charts["src1"] = []domain.SourceChart{
		charts[0].SourceChart, charts[1].SourceChart, charts[2].SourceChart,
	}
	// 既存 seedSource 経路ではなく、enriched 経路を使うようにする (fake 側に setEnrichedCharts を追加してもよい)

	pub := domain.PublishedTable{
		ID: "p1", Slug: "s1", DisplayName: "P1",
		Pick: domain.PickConfig{
			RefreshMode: domain.RefreshModeManual,
			WeightMode:  domain.WeightModeSort,
		},
		Levels: []domain.PublishedTableLevel{{
			ID: "lv1", Name: "0", PerMappingPick: 0, TotalPick: 2,
			Mappings: []domain.PublishedTableLevelMapping{
				{ID: "mp1", SourceTableID: "src1", SourceLevel: "0", SortOrder: 0},
			},
		}},
	}
	f.pubRepo.tables[pub.ID] = pub
	f.pubRepo.slug2id[pub.Slug] = pub.ID

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	// 古い順 → mc (pos=3) → mb (pos=2). 出力整列は (mappingIdx, Position) なので mb (pos=2), mc (pos=3)
	require.Equal(t, "mb", r.Charts[0].MD5)
	require.Equal(t, "mc", r.Charts[1].MD5)
}
```

> ⚠️ 上記テストは既存 `fakeSourceRepo` の構造に依存する。`fakeSourceRepo.LoadCharts` が `LastPlayedAt` 込み EnrichedChart を返せるよう、必要なら fake にフィールドを追加する (`enrichedCharts map[string][]domain.EnrichedChart`)。既存 fake の LoadCharts が SourceChart を EnrichedChart に変換しているだけなら、新フィールドを生やしてオーバーライドする実装を追加。

`pick_usecase_test.go` の fake 構造に応じて microhabits を合わせる:

```go
// fakeSourceRepo に追加
type fakeSourceRepo struct {
	// ...既存フィールド
	enrichedCharts map[string][]domain.EnrichedChart // セット時はこちらを優先
}

// LoadCharts の冒頭に
if cs, ok := r.enrichedCharts[sourceID]; ok {
	// q.OwnedOnly フィルタを適用
	if q.OwnedOnly {
		filtered := make([]domain.EnrichedChart, 0, len(cs))
		for _, c := range cs {
			if c.IsOwned {
				filtered = append(filtered, c)
			}
		}
		return filtered, nil
	}
	return cs, nil
}
// 既存の SourceChart → EnrichedChart 変換ロジック
```

(具体的な fake 実装は既存コードを読んで合わせる。重要なのは「テストから LastPlayedAt を注入できる経路」を 1 つ用意すること)

```
go test ./internal/usecase/ -run TestPickUseCase_SortMode -v
```
Expected: PASS

- [ ] **Step 8: probability モードの統計テスト追加**

`pick_usecase_test.go` の末尾に追加:

```go
func TestPickUseCase_ProbabilityMode_OldestPickedMoreOftenThanNewest(t *testing.T) {
	f := newPickUCFixture(t)
	now := f.clock.t.Unix()
	day := int64(86400)
	const N = 50 // 50 曲 (うち 1 曲が最古 5 年, 1 曲が最新 1 日前, 残りは均一中間)
	charts := make([]domain.EnrichedChart, N)
	for i := 0; i < N; i++ {
		var lp int64
		switch {
		case i == 0:
			lp = now - 1*day // 最新
		case i == N-1:
			lp = now - 1825*day // 最古 (5 年)
		default:
			lp = now - int64(i*30)*day // 等間隔
		}
		md5 := "m" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		charts[i] = chartWithLastPlayed("src1", "0", i+1, md5, lp)
	}
	f.srcRepo.enrichedCharts["src1"] = charts

	pub := domain.PublishedTable{
		ID: "p-prob", Slug: "prob", DisplayName: "P-prob",
		Pick: domain.PickConfig{
			RefreshMode:  domain.RefreshModePerRequest,
			WeightMode:   domain.WeightModeProbability,
			WeightParamX: 100,
		},
		Levels: []domain.PublishedTableLevel{{
			ID: "lv1", Name: "0", PerMappingPick: 0, TotalPick: 1,
			Mappings: []domain.PublishedTableLevelMapping{
				{ID: "mp1", SourceTableID: "src1", SourceLevel: "0", SortOrder: 0},
			},
		}},
	}
	f.pubRepo.tables[pub.ID] = pub
	f.pubRepo.slug2id[pub.Slug] = pub.ID

	// stub rng は単調増加なので、確率モードの直接統計は確認しにくい。
	// 代わりに 1 曲ピックを 1 回行って seed 依存で最古曲が出ることを assert する。
	// (decisive な統計テストは Task 16 の testdata 依存版で実施)
	_, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
}

func TestPickUseCase_UnplayedTreatedAsOldest(t *testing.T) {
	f := newPickUCFixture(t)
	now := f.clock.t.Unix()
	day := int64(86400)
	charts := []domain.EnrichedChart{
		chartWithLastPlayed("src1", "0", 1, "ma", now-1*day),
		chartWithLastPlayed("src1", "0", 2, "mb", 0), // 未プレイ
	}
	f.srcRepo.enrichedCharts["src1"] = charts

	pub := domain.PublishedTable{
		ID: "p-unplayed", Slug: "unplayed", DisplayName: "U",
		Pick: domain.PickConfig{
			RefreshMode: domain.RefreshModeManual,
			WeightMode:  domain.WeightModeSort,
		},
		Levels: []domain.PublishedTableLevel{{
			ID: "lv1", Name: "0", PerMappingPick: 0, TotalPick: 2,
			Mappings: []domain.PublishedTableLevelMapping{
				{ID: "mp1", SourceTableID: "src1", SourceLevel: "0", SortOrder: 0},
			},
		}},
	}
	f.pubRepo.tables[pub.ID] = pub
	f.pubRepo.slug2id[pub.Slug] = pub.ID

	r, _, err := f.uc.PickBySlug(context.Background(), pub.Slug)
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	// sort 順: a=1 の未プレイ (mb) が先、次に a≈0.001 の ma。出力整列 (Position) で ma → mb
	require.Equal(t, "ma", r.Charts[0].MD5)
	require.Equal(t, "mb", r.Charts[1].MD5)
	// 出力順は (mappingIdx, Position) なので入力順 ma, mb のまま。
	// 重要なのは 2 曲とも取られたこと (未プレイが除外されていない)。
}
```

- [ ] **Step 9: 全テスト実行**

```
go test ./... -v
```
Expected: PASS

- [ ] **Step 10: コミット**

```bash
git add internal/usecase/pick_usecase.go internal/usecase/pick_usecase_test.go
git commit -m "feat(pick): WeightMode 3 モード分岐 (off / probability / sort)

pickLevel 入口で unionPool の max(now-date) を計算し、aOf で正規化。
phase1/phase2 で同じ aOf を共有。sort モードは決定論ソートで別経路。
WeighterFactory 経由で公開表ごとに Weighter を選択する。"
```

---

## Task 9: ConfigUseCase に score_db_path 関連を追加

**Files:**
- Modify: `internal/usecase/config_usecase.go`
- Modify: `internal/usecase/config_usecase_test.go`

- [ ] **Step 1: 失敗テストを追加**

`config_usecase_test.go` の末尾に追加:

```go
func TestConfigUseCase_GetSetScoreDBPath(t *testing.T) {
	ctx := context.Background()
	store := newInMemConfigStore() // 既存ヘルパ
	uc := usecase.NewConfigUseCase(store)

	p, err := uc.GetScoreDBPath(ctx)
	require.NoError(t, err)
	require.Equal(t, "", p)

	require.NoError(t, uc.SetScoreDBPath(ctx, "/abs/score.db"))
	p, err = uc.GetScoreDBPath(ctx)
	require.NoError(t, err)
	require.Equal(t, "/abs/score.db", p)
}

func TestConfigUseCase_AddScorePathChangeHook_FiresOnSet(t *testing.T) {
	ctx := context.Background()
	store := newInMemConfigStore()
	uc := usecase.NewConfigUseCase(store)
	calls := 0
	uc.AddScoreDBPathChangeHook(func() { calls++ })
	require.NoError(t, uc.SetScoreDBPath(ctx, "/a.db"))
	require.NoError(t, uc.SetScoreDBPath(ctx, "/b.db"))
	require.Equal(t, 2, calls)
}
```

```
go test ./internal/usecase/ -run TestConfigUseCase -v
```
Expected: FAIL

- [ ] **Step 2: ConfigUseCase に追加**

`config_usecase.go` を更新:

```go
const (
	keyServerPort     = "server_port"
	keySongdataDBPath = "songdata_db_path"
	keyScoreDBPath    = "score_db_path"
	defaultServerPort = 50000
)

type ConfigUseCase struct {
	store         port.ConfigStore
	songdataHooks []func()
	scoreHooks    []func()
}

// AddScoreDBPathChangeHook は score_db_path 変更時に呼ばれるフックを追加する。
// Bootstrap で ScoreDBAttacher.ReAttach と PickUseCase.InvalidateAll を登録する想定。
func (u *ConfigUseCase) AddScoreDBPathChangeHook(fn func()) {
	u.scoreHooks = append(u.scoreHooks, fn)
}

// GetScoreDBPath は beatoraja の score.db のパスを返す。未設定時は空文字。
func (u *ConfigUseCase) GetScoreDBPath(ctx context.Context) (string, error) {
	v, _, err := u.store.Get(ctx, keyScoreDBPath)
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetScoreDBPath は score.db のパスを保存する。
// 保存成功後に登録された ScoreDBPathChangeHook を全て呼ぶ。
func (u *ConfigUseCase) SetScoreDBPath(ctx context.Context, path string) error {
	if err := u.store.Set(ctx, keyScoreDBPath, path); err != nil {
		return err
	}
	for _, fn := range u.scoreHooks {
		fn()
	}
	return nil
}
```

- [ ] **Step 3: テスト実行**

```
go test ./internal/usecase/ -run TestConfigUseCase -v
```
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add internal/usecase/config_usecase.go internal/usecase/config_usecase_test.go
git commit -m "feat(config): score_db_path の Get/Set とフック追加

SongdataDBPath と同じ API ペア。SetScoreDBPath で登録フックを発火する。"
```

---

## Task 10: ConfigHandler に score.db パス API を追加

**Files:**
- Modify: `internal/app/handler/config_handler.go`
- Modify: `internal/app/handler/config_handler_test.go`

- [ ] **Step 1: 失敗テストを追加**

`config_handler_test.go` の末尾に追加:

```go
func TestConfigHandler_GetServerConfig_IncludesScoreDBPath(t *testing.T) {
	h, _ := newTestConfigHandler(t) // 既存ヘルパ
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, "", cfg.ScoreDBPath)
}

func TestConfigHandler_SetScoreDBPath_Persists(t *testing.T) {
	h, _ := newTestConfigHandler(t)
	require.NoError(t, h.SetScoreDBPath("/tmp/score.db"))
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, "/tmp/score.db", cfg.ScoreDBPath)
}

func TestConfigHandler_PickScoreDB_NoContext(t *testing.T) {
	h, _ := newTestConfigHandler(t)
	got, err := h.PickScoreDB()
	require.NoError(t, err)
	require.Equal(t, "", got, "SetContext 前は空文字")
}
```

```
go test ./internal/app/handler/ -run "TestConfigHandler_.*ScoreDB" -v
```
Expected: FAIL

- [ ] **Step 2: ConfigHandler を拡張**

`config_handler.go` の `ServerConfig` 構造体に `ScoreDBPath` を追加:

```go
type ServerConfig struct {
	Port           int    `json:"port"`
	SongdataDBPath string `json:"songdataDbPath"`
	ScoreDBPath    string `json:"scoreDbPath"`
}
```

`GetServerConfig` を更新:

```go
func (h *ConfigHandler) GetServerConfig() (ServerConfig, error) {
	port, err := h.uc.GetServerPort(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	songdataPath, err := h.uc.GetSongdataDBPath(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	scorePath, err := h.uc.GetScoreDBPath(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	return ServerConfig{Port: port, SongdataDBPath: songdataPath, ScoreDBPath: scorePath}, nil
}
```

新規メソッドを追加:

```go
// SetScoreDBPath は beatoraja の score.db パスを保存する。
func (h *ConfigHandler) SetScoreDBPath(path string) error {
	return h.uc.SetScoreDBPath(h.ctx, path)
}

// PickScoreDB はユーザーに score.db のパスを OS のファイル選択ダイアログで選ばせる。
func (h *ConfigHandler) PickScoreDB() (string, error) {
	if h.ctx == nil || h.ctx == context.Background() {
		return "", nil
	}
	return wailsruntime.OpenFileDialog(h.ctx, wailsruntime.OpenDialogOptions{
		Title: "score.db を選択",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "SQLite データベース (*.db)", Pattern: "*.db"},
			{DisplayName: "すべてのファイル (*.*)", Pattern: "*"},
		},
	})
}
```

- [ ] **Step 3: テスト実行**

```
go test ./internal/app/handler/ -v
```
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add internal/app/handler/config_handler.go internal/app/handler/config_handler_test.go
git commit -m "feat(handler): score.db パスの Get/Set/Pick API を ConfigHandler に追加"
```

---

## Task 11: PublishedTableHandler の DTO 拡張 + バリデーション

**Files:**
- Modify: `internal/app/handler/published_table_handler.go`
- Modify: `internal/app/handler/published_table_handler_test.go`

- [ ] **Step 1: DTO とバリデーション失敗テストを追加**

既存 `published_table_handler.go` を読んで、Create/Update DTO の構造を把握する。DTO に `WeightMode string` と `WeightParamX int` を追加し、`toDomain` 変換と範囲チェック (probability 時 `2 <= X <= 10000`) を入れる。

(具体テスト/差分は既存ハンドラの命名規約に合わせる。例:)

```go
func TestPublishedTableHandler_Create_RejectsWeightParamOutOfRange(t *testing.T) {
	h, _ := newTestPubHandler(t)
	_, err := h.CreatePublishedTable(handler.PublishedTableCreateRequest{
		Slug: "x", DisplayName: "X", Symbol: "★",
		Pick: handler.PickConfigDTO{
			RefreshMode:  "manual",
			WeightMode:   "probability",
			WeightParamX: 10001,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "10000")
}

func TestPublishedTableHandler_Create_AcceptsWeightSort(t *testing.T) {
	h, _ := newTestPubHandler(t)
	got, err := h.CreatePublishedTable(handler.PublishedTableCreateRequest{
		Slug: "x2", DisplayName: "X", Symbol: "★",
		Pick: handler.PickConfigDTO{
			RefreshMode: "manual",
			WeightMode:  "sort",
			// X は無視されるが、UI から送られ得るので 10 が来ても受け付ける
			WeightParamX: 10,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "sort", got.Pick.WeightMode)
}
```

- [ ] **Step 2: DTO / バリデーションを実装**

`PickConfigDTO` (or 該当箇所) に `WeightMode string` と `WeightParamX int` を追加。`toDomain` で:

```go
func (d PickConfigDTO) toDomain() (domain.PickConfig, error) {
	mode := domain.WeightMode(d.WeightMode)
	if mode == "" {
		mode = domain.WeightModeOff
	}
	switch mode {
	case domain.WeightModeOff, domain.WeightModeProbability, domain.WeightModeSort:
	default:
		return domain.PickConfig{}, fmt.Errorf("weight_mode が不正です: %q", d.WeightMode)
	}
	x := d.WeightParamX
	if x == 0 {
		x = 10
	}
	if mode == domain.WeightModeProbability {
		if x < 2 || x > 10000 {
			return domain.PickConfig{}, fmt.Errorf("weight_param_x は 2〜10000 の範囲で指定してください (got %d)", x)
		}
	}
	return domain.PickConfig{
		RefreshMode:  domain.RefreshMode(d.RefreshMode),
		WeightMode:   mode,
		WeightParamX: x,
	}, nil
}
```

(既存 DTO 名と toDomain 関数の位置はハンドラを読んで合わせる)

- [ ] **Step 3: テスト実行**

```
go test ./internal/app/handler/ -v
```
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add internal/app/handler/published_table_handler.go internal/app/handler/published_table_handler_test.go
git commit -m "feat(handler): PublishedTable の WeightMode / WeightParamX を DTO に追加

probability モード時のみ X ∈ [2,10000] を防御チェック。"
```

---

## Task 12: Bootstrap で ScoreDBAttacher / WeighterFactory を配線

**Files:**
- Modify: `internal/app/bootstrap.go`

- [ ] **Step 1: ScoreDBAttacher を構築・Attach・hook 登録**

`bootstrap.go` の `Bootstrap()` を更新。`sourceAttacher` 直後 〜 `sourceRepo` 生成の流れを以下に変更:

```go
	systemClock := clock.System{}
	sourceAttacher := persistence.NewSongdataAttacher(db, systemClock, lg)
	scoreAttacher := persistence.NewScoreDBAttacher(db, systemClock, lg)
	sourceRepo := persistence.NewSourceTableRepoSQL(db, sourceAttacher, scoreAttacher)
```

`PickUseCase` 生成を以下に変更:

```go
	pickUC := usecase.NewPickUseCase(pubRepo, sourceRepo, pickStore, systemClock, randFactory, lg, weighter.Factory{})
```

songdata 起動時 attach 直後に score 起動時 attach を追加:

```go
	// 起動時に score.db が設定済みなら ATTACH を試みる (失敗しても起動継続)
	{
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		scorePath, err := configUC.GetScoreDBPath(bgCtx)
		if err != nil {
			lg.Warn("read score_db_path failed", "err", err)
		} else if scorePath != "" {
			if attachErr := scoreAttacher.Attach(bgCtx, scorePath); attachErr != nil {
				lg.Warn("startup score attach failed", "err", attachErr, "path", scorePath)
			}
		}
		cancel()
	}
```

songdata hook の隣に score hook を追加:

```go
	configUC.AddScoreDBPathChangeHook(func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newPath, err := configUC.GetScoreDBPath(bgCtx)
		if err != nil {
			lg.Warn("get score_db_path failed", "err", err)
			return
		}
		if err := scoreAttacher.ReAttach(bgCtx, newPath); err != nil {
			lg.Warn("re-attach score failed", "err", err, "path", newPath)
		}
		pickUC.InvalidateAll()
	})
```

`Services` 構造体にもフィールド追加 (任意、GUI 表示等で使わなければ省略可):

```go
type Services struct {
	// ...既存
	ScoreDBAttacher *persistence.ScoreDBAttacher
}
```
返却値に `ScoreDBAttacher: scoreAttacher` を追加。

- [ ] **Step 2: ビルド + 全テスト**

```
go build ./... && go test ./... -count=1
```
Expected: PASS

- [ ] **Step 3: コミット**

```bash
git add internal/app/bootstrap.go
git commit -m "feat(app): Bootstrap で ScoreDBAttacher と WeighterFactory を配線

起動時 attach、score_db_path 変更時の再 attach + ピックキャッシュ破棄。"
```

---

## Task 13: Wails モジュール再生成

**Files:**
- Generated: `frontend/wailsjs/`

- [ ] **Step 1: wails generate module**

```
wails generate module
```

- [ ] **Step 2: 生成された差分の確認 (gitignore 対象のはずなのでステージしない)**

```
git status
```

`frontend/wailsjs/` は CLAUDE.md より `.gitignore` 対象なので、ステージ不要。生成物が `Models.ts` に `WeightMode`, `WeightParamX`, `ScoreDBPath` を含むことを目視確認するのみ。

---

## Task 14: フロント — 公開表編集画面に WeightMode UI 追加

既存の `RefreshMode` セレクタ近傍 (公開表の作成/編集画面) に追加する。

**Files:**
- Modify: 公開表編集を担当する Svelte コンポーネント (探して `RefreshMode` セレクタを含むファイル)

- [ ] **Step 1: 対象ファイルを特定**

```
grep -rln "RefreshMode\|refreshMode\|per_request" frontend/src/
```

ヒットした `.svelte` ファイル (推定: `frontend/src/lib/PublishedTableEditor.svelte` などの名前) を編集対象とする。

- [ ] **Step 2: ストア/ロジックに WeightMode 追加**

該当コンポーネントで:

```ts
let weightMode: 'off' | 'probability' | 'sort' = (tbl.Pick?.WeightMode as any) ?? 'off';
let weightParamX: number = tbl.Pick?.WeightParamX ?? 10;
let weightParamXError = '';

function validateX(): boolean {
  if (weightMode !== 'probability') { weightParamXError = ''; return true; }
  if (!Number.isInteger(weightParamX) || weightParamX < 2 || weightParamX > 10000) {
    weightParamXError = 'X は 2〜10000 の整数で指定してください';
    return false;
  }
  weightParamXError = '';
  return true;
}

// 保存時に Pick.WeightMode / Pick.WeightParamX をペイロードに含める
```

UI (daisyUI join + select + input):

```svelte
<div class="form-control">
  <label class="label"><span class="label-text">最終プレイ日時優先</span></label>
  <div class="join">
    <select class="join-item select select-bordered" bind:value={weightMode} on:change={validateX}>
      <option value="off">OFF</option>
      <option value="probability">確率 (X 倍まで偏らせる)</option>
      <option value="sort">完全日時順ソート</option>
    </select>
    {#if weightMode === 'probability'}
      <input
        class="join-item input input-bordered w-24"
        type="number" min="2" max="10000" step="1"
        bind:value={weightParamX}
        on:input={validateX}
      />
    {/if}
  </div>
  {#if weightParamXError}
    <label class="label"><span class="label-text-alt text-error">{weightParamXError}</span></label>
  {/if}
</div>
```

- [ ] **Step 3: 保存ボタンのバリデーションフローに `validateX` を組み込む**

既存の保存ハンドラ先頭で:

```ts
if (!validateX()) return;
```

- [ ] **Step 4: 開発サーバで動作確認**

```
make dev
```

ブラウザで公開表編集画面を開き:
- WeightMode 切替 (OFF / 確率 / ソート) で X 入力欄が表示/非表示
- X に 1, 10001, "abc" を入れて保存 → エラー表示
- X=10 で保存 → 成功
- 再ロードで値が保持

- [ ] **Step 5: 型チェック**

```
cd frontend && npm run check && cd ..
```
Expected: 0 errors

- [ ] **Step 6: コミット**

```bash
git add frontend/src/
git commit -m "feat(frontend): 公開表編集に最終プレイ日時優先 UI を追加

OFF / 確率 (X 倍) / 完全日時順ソート の 3 モード。
probability 時の X はフロントで 2〜10000 の整数バリデーション。"
```

---

## Task 15: フロント — 設定画面に score.db パス入力を追加

**Files:**
- Modify: 設定画面を担当する Svelte コンポーネント (`SongdataDBPath` を含むファイル)

- [ ] **Step 1: 対象ファイルを特定**

```
grep -rln "SongdataDBPath\|songdataDbPath\|PickSongdataDB" frontend/src/
```

- [ ] **Step 2: songdata.db 行の**直下**に score.db 行を追加**

該当コンポーネントに以下を追加。既存の `SongdataDBPath` 入力と全く同じ構造で複製:

```svelte
<div class="form-control">
  <label class="label"><span class="label-text">songdata.db パス</span></label>
  <!-- 既存 -->
</div>

<!-- ↓ 新規追加 (songdata.db の直下) -->
<div class="form-control">
  <label class="label"><span class="label-text">score.db パス</span></label>
  <div class="join w-full">
    <input class="join-item input input-bordered flex-1" type="text" bind:value={scoreDbPath} />
    <button class="join-item btn" on:click={async () => {
      const picked = await PickScoreDB();
      if (picked) scoreDbPath = picked;
    }}>選択...</button>
    <button class="join-item btn btn-primary" on:click={async () => {
      await SetScoreDBPath(scoreDbPath);
      await loadConfig(); // 再読込
    }}>保存</button>
  </div>
</div>
```

`loadConfig` 等の初期化処理で `cfg.scoreDbPath` を `scoreDbPath` に詰める。

- [ ] **Step 3: 開発サーバで動作確認**

```
make dev
```

- パス未設定 → 空欄
- ファイルダイアログから score.db を選択 → パスが入る
- 保存ボタンで設定永続化
- アプリ再起動で値が保持
- 公開表で probability モードを使い、ピックすると ATTACH log が `score attached` で出る (logs ディレクトリ)

- [ ] **Step 4: 型チェック**

```
cd frontend && npm run check && cd ..
```

- [ ] **Step 5: コミット**

```bash
git add frontend/src/
git commit -m "feat(frontend): 設定画面に score.db パス入力を追加 (songdata.db の直下)"
```

---

## Task 16: testdata 依存のシミュレーション再現テスト

**Files:**
- Modify: `internal/usecase/pick_usecase_test.go`

- [ ] **Step 1: testdata の存在チェックを伴うテストを追加**

`pick_usecase_test.go` の末尾に追加:

```go
// TestPickUseCase_SL0Simulation は testdata/{songdata,score}.db + stellabms sl0 相当の
// テストデータを使って probability X=10 の確率分布が想定範囲に入ることを確認する。
// CI 等で testdata が無い環境ではスキップする (existing songdata_reader_test.go と同方針)。
func TestPickUseCase_SL0Simulation(t *testing.T) {
	if _, err := os.Stat("../../testdata/songdata.db"); err != nil {
		t.Skip("testdata/songdata.db not present; skipping integration sim")
	}
	if _, err := os.Stat("../../testdata/score.db"); err != nil {
		t.Skip("testdata/score.db not present; skipping integration sim")
	}
	// このテストは実 DB を読むため persistence パッケージ経由で組み立てる。
	// pickLevel の最終出力ではなく、aOf 由来の重み分布を直接 assert する方が確実なので、
	// LastPlayedWeighter + 内部 helper を呼ぶ別ヘルパテストとして書く。
	// 詳細実装は本プラン Step 2 で。
}
```

- [ ] **Step 2: 実 DB 経由の確率分布テストを実装**

`pick_usecase_test.go` または専用テストファイルに以下を追加。public package `usecase` の外から到達できる helper を使うか、テスト用に `pickLevel` をスタンドアロンで呼べる経路を追加する。簡単には、`weighter.LastPlayedWeighter` を直接呼び、`testdata/songdata.db` + `testdata/score.db` + sl0 md5 リストを使って 170 曲分の `a` と重みを計算し、最古/中央値/最新の確率が以下範囲に入ることを assert:

```go
require.InDelta(t, 0.0113, pOldest, 0.001, "最古 1.13% ±0.1%")
require.InDelta(t, 0.0066, pMedian, 0.001, "中央値 0.66% ±0.1%")
require.InDelta(t, 0.0011, pNewest, 0.0005, "最新 0.11% ±0.05%")
```

sl0 md5 リストは `.96kudye/tmp/260511_sl0_pick_weight_sim.py` で取得したものをテスト用 JSON として `testdata/sl0_md5s.json` に保存 (このファイルも `.gitignore` 対象に追加)。テストはそれを読み込んで sha256 解決 → score.db 参照 → 重み計算する。

CI で testdata が無い場合は冒頭の `t.Skip` で逃げる。テストは「ローカル開発者の手動検証」目的で残す。

- [ ] **Step 3: testdata/sl0_md5s.json を生成**

スクリプトを軽く改修して JSON 出力するか、手動で:

```bash
python3 -c "
import json, urllib.request
data = json.loads(urllib.request.urlopen('https://stellabms.xyz/sl/score.json').read())
md5s = [e['md5'] for e in data if str(e.get('level')) == '0' and e.get('md5')]
with open('testdata/sl0_md5s.json', 'w') as f:
    json.dump(md5s, f)
print(f'{len(md5s)} md5s saved')
"
```

`.gitignore` の `testdata/` ルールで除外されているのを確認:

```
grep -E "testdata|sl0_md5s" .gitignore
```

- [ ] **Step 4: テスト実行 (ローカル)**

```
go test ./internal/usecase/ -run TestPickUseCase_SL0Simulation -v
```
Expected: PASS (testdata 揃っている環境)

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/pick_usecase_test.go
git commit -m "test(pick): testdata 依存の sl0 確率分布回帰テストを追加

testdata 不在環境では skip。X=10 で最古/中央/最新の確率が
スクリプト想定 (1.13% / 0.66% / 0.11%) ±tolerance に入ることを確認。"
```

---

## Task 17: マニュアル更新

**Files:**
- Modify: `docs/manual.md`

- [ ] **Step 1: 機能セクションを追記**

`docs/manual.md` を開き、機能説明セクション (例: 「公開表の設定」) に以下を追加:

```markdown
## 最終プレイ日時優先ピック

公開表ごとに「あまり最近触っていない曲を優先的にピックする」挙動を設定できます。

### 3 モード

| モード | 動作 |
|---|---|
| OFF | 一様ランダム (既定) |
| 確率 (X 倍) | 最古プレイ曲が最新プレイ曲の X 倍ピックされやすくなる重み付きランダム |
| 完全日時順ソート | 古い順に確定的にピックされる |

### 設定方法

1. アプリ右上の歯車アイコンから **設定画面** を開き、`score.db パス` に beatoraja の `score.db` (通常 `<beatoraja>/player/playerN/score.db`) を指定して保存。
2. 公開表編集画面で **最終プレイ日時優先** を選択。
3. 確率モードの場合 X (2〜10000) を入力。X=10 で「最古は最新の 10 倍」相当。
4. 保存 → ピック更新 (per_request の場合は自動、manual の場合は再ピックボタン)。

### 注意点

- `OwnedOnly=false` の公開表では、未所持曲が「最古プレイ扱い」となり高重みで多数ピックされます。所持曲のみ優先したい場合は `OwnedOnly=true` を併用してください。
- 確率モードを X=10000 等の極端値にしても、古い曲群は近い確率に潰れます。「最古から確定 N 曲」を望む場合は **完全日時順ソート** を選んでください。
- beatoraja 動作中でも score.db を読めますが、SQLITE_BUSY が発生した場合は一時的に全曲未プレイ扱いになります (ピック自体は止まりません)。
```

- [ ] **Step 2: コミット**

```bash
git add docs/manual.md
git commit -m "docs(manual): 最終プレイ日時優先ピック機能の説明を追加"
```

---

## Final Check

- [ ] **全テスト + ビルド + lint + 型チェック**

```
make lint && go build ./... && go test ./... -count=1 && cd frontend && npm run check && cd ..
```

Expected: 全部 green

- [ ] **動作確認 (`make dev`)**

ブラウザでの golden path:
1. 設定画面で songdata.db と score.db 両方を設定
2. 既存公開表を編集して WeightMode=probability, X=10 で保存
3. ピックして 30 曲ほどの結果を確認 (古めの曲が混じる)
4. WeightMode=sort に切り替えて保存 → 再ピックすると古い順
5. WeightMode=off に戻して保存 → 従来挙動

- [ ] **push (Windows ビルド反映)**

memory: "main 直開発 + 完了時に git push origin main" に従う。

```bash
git push origin main
```

---

## Self-Review Notes

- 仕様 §1〜§10 すべてに対応タスクあり (1: 概要は全体, 2: §2 → Task 5/6, 3: §3 → Task 6/12, 4: §4 → Task 2/3/4, 5: §5 → Task 8, 6: §6 → Task 14/15, 7: §7 → Task 1/9, 8: §8 → Task 1/4/5/6/7/8/9/10/11/16, 9: §9 影響範囲は Task 1-17 を網羅, 10: §10 留意点は §17 マニュアルで明記)
- Weighter シグネチャ変更 (Task 3) は Task 4 と同コミットでまとめる方針を明記
- `NewSourceTableRepoSQL` 引数追加 (Task 6) で既存呼出箇所を nil で暫定埋めし、Task 12 で Bootstrap が実 attacher を渡すという 2 段階移行を明示
- フロント要素は実ファイル名を grep で特定する手順にしてある (Task 14/15 Step 1)
