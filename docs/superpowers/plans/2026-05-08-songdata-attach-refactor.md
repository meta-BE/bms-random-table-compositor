# songdata.db ATTACH リファクタ実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `OwnedMD5Cache` のメモリ常駐を削除し、`songdata.db` を `ATTACH DATABASE` してメイン `*sql.DB` で SQL 一発 JOIN できる構成に移行する。`source_table_chart` の SELECT に `IsOwned`/`LastPlayedAt` (今回は NULL 固定) を JOIN で常時付与し、`EnrichedChart` で返す。

**Architecture:** bms-elsa パターン (`SetMaxOpenConns(1)` + 起動時 ATTACH RO) を踏襲。`SongdataAttacher` adapter が ATTACH/DETACH/Status を管理。`port.SourceTableRepo.LoadCharts` は `(sourceID, ChartQuery) → []EnrichedChart` に変更。設定変更時は `Attacher.ReAttach` + `PickUseCase.InvalidateAll`。

**Tech Stack:** Go 1.24+, `modernc.org/sqlite`, Wails v2, Svelte + TypeScript + Tailwind/daisyUI

**設計ドキュメント:** `docs/superpowers/specs/2026-05-08-songdata-attach-refactor-design.md`

**運用ルール (memory より):** main 直開発、PR なし。各タスク完了時に main へコミット、全タスク完了後に `git push origin main`。

---

## Task 1: `domain.EnrichedChart` 追加

**Files:**
- Create: `internal/domain/enriched_chart.go`

- [ ] **Step 1: 型定義を作成**

`internal/domain/enriched_chart.go`:
```go
package domain

import "time"

// EnrichedChart は SourceChart にローカル DB 由来の状態を載せた読み取り専用ビュー。
// 永続化はせず、リクエスト毎に SourceTableRepo.LoadCharts が SQL で組み立てる。
type EnrichedChart struct {
	SourceChart                 // 既存フィールドを埋め込み
	IsOwned      bool           // sd.song に存在するか (未アタッチ時は false)
	LastPlayedAt *time.Time     // sd.score 由来。実取得は v2、現状は常に nil
}
```

- [ ] **Step 2: ビルド確認**

Run: `go build ./...`
Expected: 成功 (型追加のみで既存への影響なし)

- [ ] **Step 3: コミット**

```bash
git add internal/domain/enriched_chart.go
git commit -m "feat(domain): EnrichedChart 型を追加 (IsOwned/LastPlayedAt)"
```

---

## Task 2: `domain.SongdataAttachStatus` + `port.ChartQuery` 追加

**Files:**
- Create: `internal/domain/songdata_attach_status.go`
- Modify: `internal/port/source_table_repo.go`

- [ ] **Step 1: SongdataAttachStatus 型を作成**

`internal/domain/songdata_attach_status.go`:
```go
package domain

import "time"

// SongdataAttachStatus は SongdataAttacher の状態スナップショット (GUI 表示用)。
type SongdataAttachStatus struct {
	Attached   bool
	Path       string
	SongCount  int        // SELECT COUNT(*) FROM sd.song の最終値
	AttachedAt *time.Time
	LastError  string
}
```

- [ ] **Step 2: ChartQuery 型を port に追加**

`internal/port/source_table_repo.go` の冒頭 (import 直後) に追加:
```go
// ChartQuery は SourceTableRepo.LoadCharts に渡す SQL レベルのフィルタ。
// IsOwned/LastPlayedAt 等の派生プロパティは戻り値の EnrichedChart に常に含まれる。
// このフィルタは「DB 段階で足切りしたい場合」だけ指定する (パフォーマンス目的)。
type ChartQuery struct {
	OwnedOnly bool // EXISTS sd.song で足切り (未アタッチ時は強制的に空配列を返す)
}
```

`LoadCharts` シグネチャの変更は Task 4 で行う。ここでは型のみ追加。

- [ ] **Step 3: ビルド確認**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 4: コミット**

```bash
git add internal/domain/songdata_attach_status.go internal/port/source_table_repo.go
git commit -m "feat(port,domain): SongdataAttachStatus / ChartQuery 型を追加"
```

---

## Task 3: `SongdataAttacher` 実装 (TDD)

**Files:**
- Create: `internal/adapter/persistence/songdata_attacher.go`
- Create: `internal/adapter/persistence/songdata_attacher_test.go`

- [ ] **Step 1: 失敗するテストを書く (空パスは no-op)**

`internal/adapter/persistence/songdata_attacher_test.go`:
```go
package persistence_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/stretchr/testify/require"
)

func newAttacherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "main.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	return db
}

func newAttacher(t *testing.T, db *sql.DB) *persistence.SongdataAttacher {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return persistence.NewSongdataAttacher(db, clock.System{}, logger)
}

func TestSongdataAttacher_Attach_EmptyPathIsNoop(t *testing.T) {
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	err := a.Attach(context.Background(), "")
	require.NoError(t, err)

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.Empty(t, st.Path)
	require.Empty(t, st.LastError)
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `go test ./internal/adapter/persistence/ -run TestSongdataAttacher_Attach_EmptyPathIsNoop -count=1`
Expected: ビルドエラー (`SongdataAttacher` 未定義)

- [ ] **Step 3: 最小実装で空パスケースだけ通す**

`internal/adapter/persistence/songdata_attacher.go`:
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

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// SongdataAttacher はメイン *sql.DB に対する songdata.db の ATTACH/DETACH ライフサイクルを管理する。
// SetMaxOpenConns(1) 前提 (ATTACH はコネクション単位)。
// GUI 表示用に最終アタッチ状態とエラーをスナップショット保持する。
type SongdataAttacher struct {
	db    *sql.DB
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	attached   bool
	path       string
	songCount  int
	attachedAt *time.Time
	lastErr    string
}

// NewSongdataAttacher は新しい SongdataAttacher を作る。
func NewSongdataAttacher(db *sql.DB, clk port.Clock, log *slog.Logger) *SongdataAttacher {
	return &SongdataAttacher{db: db, clock: clk, log: log}
}

// Attach は songdata.db を schema 'sd' として RO ATTACH する。
// path が空なら何もしない (失敗ではない)。
// 既にアタッチされている状態で呼ばれた場合は一度 DETACH してから ATTACH し直す。
func (a *SongdataAttacher) Attach(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if a.IsAttached() {
		if err := a.Detach(ctx); err != nil {
			return err
		}
	}
	dsn := fmt.Sprintf("file:%s?mode=ro", url.QueryEscape(path))
	if _, err := a.db.ExecContext(ctx, "ATTACH DATABASE ? AS sd", dsn); err != nil {
		a.recordError(err.Error())
		return fmt.Errorf("attach songdata %q: %w", path, err)
	}

	var count int
	row := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sd.song")
	if err := row.Scan(&count); err != nil {
		// COUNT 失敗時は ATTACH 状態を維持しつつエラー記録 (テーブル不在等)
		a.recordError(fmt.Sprintf("count sd.song: %v", err))
		count = 0
	}

	now := a.clock.Now()
	a.mu.Lock()
	a.attached = true
	a.path = path
	a.songCount = count
	a.attachedAt = &now
	a.lastErr = ""
	a.mu.Unlock()
	a.log.Info("songdata attached", "path", path, "count", count)
	return nil
}

// Detach は schema 'sd' を DETACH する。未アタッチなら no-op。
func (a *SongdataAttacher) Detach(ctx context.Context) error {
	if !a.IsAttached() {
		return nil
	}
	if _, err := a.db.ExecContext(ctx, "DETACH DATABASE sd"); err != nil {
		return fmt.Errorf("detach songdata: %w", err)
	}
	a.mu.Lock()
	a.attached = false
	a.path = ""
	a.songCount = 0
	a.attachedAt = nil
	a.mu.Unlock()
	a.log.Info("songdata detached")
	return nil
}

// ReAttach は Detach → Attach を 1 連の操作で行う (設定変更時のフック用)。
// path が空のときは Detach のみ行う。
func (a *SongdataAttacher) ReAttach(ctx context.Context, path string) error {
	if err := a.Detach(ctx); err != nil {
		return err
	}
	return a.Attach(ctx, path)
}

// IsAttached は現在 'sd' がアタッチされているかを返す。
func (a *SongdataAttacher) IsAttached() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.attached
}

// Status は GUI 表示用のスナップショットを返す。
func (a *SongdataAttacher) Status() domain.SongdataAttachStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return domain.SongdataAttachStatus{
		Attached:   a.attached,
		Path:       a.path,
		SongCount:  a.songCount,
		AttachedAt: a.attachedAt,
		LastError:  a.lastErr,
	}
}

func (a *SongdataAttacher) recordError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}
```

- [ ] **Step 4: テスト通過を確認**

Run: `go test ./internal/adapter/persistence/ -run TestSongdataAttacher_Attach_EmptyPathIsNoop -count=1`
Expected: PASS

- [ ] **Step 5: 存在しないパスを失敗させるテスト**

`songdata_attacher_test.go` に追加:
```go
func TestSongdataAttacher_Attach_NonexistentPathFails(t *testing.T) {
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	err := a.Attach(context.Background(), "/non/existent/path/songdata.db")
	require.Error(t, err)

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.NotEmpty(t, st.LastError)
}
```

Run: `go test ./internal/adapter/persistence/ -run TestSongdataAttacher_Attach_NonexistentPathFails -count=1`
Expected: PASS (modernc/sqlite は ATTACH 時にファイル不在で失敗する)

- [ ] **Step 6: testdata/songdata.db を使ったアタッチ成功テスト**

`songdata_attacher_test.go` に追加:
```go
// songdataPathOrSkip は testdata/songdata.db のパスを返す。
// ファイルが無ければ t.Skip でスキップ (CLAUDE.md: testdata は .gitignore 対象)。
func songdataPathOrSkip(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "songdata.db")
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("testdata/songdata.db が無いためスキップ: %v", err)
	}
	return abs
}

func TestSongdataAttacher_Attach_RealDB(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))

	require.True(t, a.IsAttached())
	st := a.Status()
	require.True(t, st.Attached)
	require.Equal(t, songdataPath, st.Path)
	require.Greater(t, st.SongCount, 0)
	require.NotNil(t, st.AttachedAt)
	require.Empty(t, st.LastError)
}

func TestSongdataAttacher_DetachThenStatus(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))
	require.NoError(t, a.Detach(context.Background()))

	require.False(t, a.IsAttached())
	st := a.Status()
	require.False(t, st.Attached)
	require.Empty(t, st.Path)
	require.Equal(t, 0, st.SongCount)
	require.Nil(t, st.AttachedAt)
}

func TestSongdataAttacher_ReAttach(t *testing.T) {
	songdataPath := songdataPathOrSkip(t)
	db := newAttacherTestDB(t)
	a := newAttacher(t, db)

	require.NoError(t, a.Attach(context.Background(), songdataPath))
	first := a.Status().SongCount

	// 同じパスで再 ATTACH しても問題なく成功する
	require.NoError(t, a.ReAttach(context.Background(), songdataPath))
	require.True(t, a.IsAttached())
	require.Equal(t, first, a.Status().SongCount)
}
```

import に `os` を追加:
```go
import (
	// ... 既存 ...
	"os"
)
```

Run: `go test ./internal/adapter/persistence/ -run TestSongdataAttacher -count=1`
Expected: PASS (testdata 不在ならスキップ表示)

- [ ] **Step 7: コミット**

```bash
git add internal/adapter/persistence/songdata_attacher.go \
        internal/adapter/persistence/songdata_attacher_test.go
git commit -m "feat(persistence): SongdataAttacher を追加 (RO ATTACH/DETACH + Status)"
```

---

## Task 4: `port.SourceTableRepo.LoadCharts` シグネチャ変更

このタスクで Go の interface 変更が起きるので、すべての実装・呼び出し側を 1 コミットで揃える。手順は「TDD で新挙動を先に書く」より「signature 変更 → 全箇所更新 → 一括ビルド」の方が現実的。

**Files:**
- Modify: `internal/port/source_table_repo.go`
- Modify: `internal/adapter/persistence/source_table_repo.go`
- Modify: `internal/adapter/persistence/source_table_repo_test.go`
- Modify: `internal/usecase/pick_usecase.go`
- Modify: `internal/usecase/pick_usecase_test.go`
- Modify: `internal/usecase/source_table_usecase_test.go`
- Modify: `internal/app/handler/source_table_handler_test.go`

- [ ] **Step 1: port のシグネチャを変更**

`internal/port/source_table_repo.go` の `LoadCharts` 行を置換:
```go
// LoadCharts は source_table_chart を position 昇順で返す。
// SongdataAttacher で sd がアタッチされている場合は EnrichedChart.IsOwned に
// EXISTS sd.song の結果が入り、未アタッチ時は IsOwned=false 一律。
// LastPlayedAt は v2 で実装予定 (現状は常に nil)。
LoadCharts(ctx context.Context, sourceID string, q ChartQuery) ([]domain.EnrichedChart, error)
```

- [ ] **Step 2: adapter 実装を更新 (attacher 依存を追加、bare 経路と attached 経路を分岐)**

`internal/adapter/persistence/source_table_repo.go` の `SourceTableRepoSQL` 構造体と `NewSourceTableRepoSQL` を変更:
```go
type SourceTableRepoSQL struct {
	db       *sql.DB
	attacher *SongdataAttacher
}

// NewSourceTableRepoSQL は新しい SourceTableRepoSQL を作る。
// attacher 経由で songdata.db のアタッチ状態を見て LoadCharts の SQL を切り替える。
func NewSourceTableRepoSQL(db *sql.DB, attacher *SongdataAttacher) *SourceTableRepoSQL {
	return &SourceTableRepoSQL{db: db, attacher: attacher}
}
```

`LoadCharts` メソッド全体を置換 (`source_table_repo.go:257-291`):
```go
// LoadCharts は source_table_chart を position 昇順で EnrichedChart として返す。
// SongdataAttacher が sd をアタッチ済みなら IsOwned を EXISTS sd.song で計算する。
// 未アタッチ時は IsOwned=false で返し、q.OwnedOnly=true なら空配列を返す
// (spec: DB 未設定時は owned_only の表は 0 件)。
// LastPlayedAt は今回 NULL 固定 (v2 で sd.score を見る形に拡張する)。
func (r *SourceTableRepoSQL) LoadCharts(
	ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
	if r.attacher != nil && r.attacher.IsAttached() {
		return r.loadChartsAttached(ctx, sourceID, q)
	}
	if q.OwnedOnly {
		// 未アタッチ + 所持絞り込み → 0 件
		return nil, nil
	}
	return r.loadChartsBare(ctx, sourceID)
}

func (r *SourceTableRepoSQL) loadChartsAttached(
	ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
	ownedFlag := 0
	if q.OwnedOnly {
		ownedFlag = 1
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
		  c.position, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
		  EXISTS(SELECT 1 FROM sd.song s WHERE s.md5 = c.md5)        AS is_owned,
		  NULL                                                        AS last_played_at
		FROM source_table_chart c
		WHERE c.source_id = ?
		  AND (? = 0 OR EXISTS (SELECT 1 FROM sd.song s WHERE s.md5 = c.md5))
		ORDER BY c.position ASC`,
		sourceID, ownedFlag,
	)
	if err != nil {
		return nil, fmt.Errorf("load enriched charts (attached) %q: %w", sourceID, err)
	}
	defer rows.Close()
	return scanEnrichedRows(rows, sourceID)
}

func (r *SourceTableRepoSQL) loadChartsBare(
	ctx context.Context, sourceID string,
) ([]domain.EnrichedChart, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.position, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
		       0 AS is_owned, NULL AS last_played_at
		FROM source_table_chart c
		WHERE c.source_id = ?
		ORDER BY c.position ASC`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("load enriched charts (bare) %q: %w", sourceID, err)
	}
	defer rows.Close()
	return scanEnrichedRows(rows, sourceID)
}

func scanEnrichedRows(rows *sql.Rows, sourceID string) ([]domain.EnrichedChart, error) {
	var out []domain.EnrichedChart
	for rows.Next() {
		var (
			c              domain.SourceChart
			rawJSON        string
			isOwned        bool
			lastPlayedAt   sql.NullString // 現状は常に NULL、v2 でカラム化予定
		)
		if err := rows.Scan(
			&c.Position, &c.MD5, &c.SHA256, &c.Level, &c.Title, &c.Artist,
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
		// lastPlayedAt は今回常に NULL のため、Valid=false → ec.LastPlayedAt は nil のまま
		_ = lastPlayedAt
		out = append(out, ec)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: 既存 adapter テスト (`source_table_repo_test.go`) を新シグネチャに追従**

`source_table_repo_test.go` 内の `setupSourceTableRepo` を変更:
```go
func setupSourceTableRepo(t *testing.T) *persistence.SourceTableRepoSQL {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attacher := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	return persistence.NewSourceTableRepoSQL(db, attacher)
}
```

import に追加:
```go
import (
	// ... 既存 ...
	"io"
	"log/slog"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)
```

`r.LoadCharts(ctx, "Z")` の呼び出しを `r.LoadCharts(ctx, "Z", port.ChartQuery{})` に全置換 (該当箇所: `:171, :201, :228, :251, :262`)。戻り値の型は `[]domain.EnrichedChart` に変わるが、既存テストは `len()` や `[i].MD5` 等 SourceChart 埋め込みフィールド経由で動くはず。型不一致が出た箇所は `chart.SourceChart.MD5` 形式に修正。

- [ ] **Step 4: `PickUseCase` を新シグネチャに追従**

`internal/usecase/pick_usecase.go` の構造体から `owned` を削除:
```go
type PickUseCase struct {
	pubRepo port.PublishedTableRepo
	srcRepo port.SourceTableRepo
	store   *PickResultStore
	clock   port.Clock
	randNew port.RandSourceFactory
	log     *slog.Logger
}

func NewPickUseCase(
	pubRepo port.PublishedTableRepo,
	srcRepo port.SourceTableRepo,
	store *PickResultStore,
	clock port.Clock,
	randNew port.RandSourceFactory,
	log *slog.Logger,
) *PickUseCase {
	return &PickUseCase{
		pubRepo: pubRepo, srcRepo: srcRepo, store: store,
		clock: clock, randNew: randNew, log: log,
	}
}
```

`regenerate` 内の `LoadCharts` 呼び出しと intersect ループを置換 (`pick_usecase.go:104-131` 相当部分):
```go
all, err := u.srcRepo.LoadCharts(ctx, pub.SourceTableID, port.ChartQuery{
	OwnedOnly: pub.OwnedOnly,
})
if err != nil {
	return domain.PickResult{}, fmt.Errorf("load charts %q: %w", pub.SourceTableID, err)
}
// 旧 intersect ロジックは削除 (SQL で足切り済み)
```

`byLevel` 構築以降は `[]EnrichedChart` を扱うように型を追従。`finalCharts` は **`[]SourceChart` のまま**にする (PickResult は外部応答にも使い、所持状態は流出させない):
```go
byLevel := map[string][]domain.EnrichedChart{}
for _, c := range all {
	byLevel[c.Level] = append(byLevel[c.Level], c)
}

// ... buildLevelOrder, makeSeed, rng は変更なし ...

var finalCharts []domain.SourceChart
var finalLevelOrder []string
for _, level := range levelOrder {
	charts, ok := byLevel[level]
	if !ok || len(charts) == 0 {
		continue
	}
	sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
	if pub.Pick.PerLevel > 0 && len(charts) > pub.Pick.PerLevel {
		rng.Shuffle(len(charts), func(i, j int) { charts[i], charts[j] = charts[j], charts[i] })
		charts = charts[:pub.Pick.PerLevel]
		sort.SliceStable(charts, func(i, j int) bool { return charts[i].Position < charts[j].Position })
	}
	for _, ec := range charts {
		finalCharts = append(finalCharts, ec.SourceChart)
	}
	finalLevelOrder = append(finalLevelOrder, level)
}
```

`buildLevelOrder` の引数も `map[string][]EnrichedChart` を受けるように変える:
```go
func buildLevelOrder(srcOrder []string, byLevel map[string][]domain.EnrichedChart) []string {
	// 既存実装の domain.SourceChart を domain.EnrichedChart に置換するだけ
	// ... (内部ロジックは同じ)
}
```

- [ ] **Step 5: `pick_usecase_test.go` の fake を更新**

`pick_usecase_test.go` の `chartFixture` を EnrichedChart 返却に変更し、`fakeSourceRepo.LoadCharts` のシグネチャを更新する。`fakeOwnedRepo` および `OwnedMD5Cache` 関連フィールドは pickUCFixture から除去。

`pickUCFixture` 構造体:
```go
type pickUCFixture struct {
	uc       *usecase.PickUseCase
	pubRepo  *fakePublishedRepo
	srcRepo  *fakeSourceRepo
	store    *usecase.PickResultStore
	clock    *mutableClock
}
```

`newPickUCFixture` から owned 関連を除去:
```go
func newPickUCFixture(t *testing.T) *pickUCFixture {
	t.Helper()
	pub := newFakePublishedRepo()
	src := newFakeSourceRepo()
	clock := &mutableClock{t: time.Date(2026, 5, 7, 12, 0, 0, 0, time.Local)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := usecase.NewPickResultStore()
	uc := usecase.NewPickUseCase(pub, src, store, clock, newStubFactory(), logger)
	return &pickUCFixture{uc: uc, pubRepo: pub, srcRepo: src, store: store, clock: clock}
}
```

`source_table_usecase_test.go` の `fakeSourceRepo` の `LoadCharts` シグネチャを `port.ChartQuery` 受け取り + `[]domain.EnrichedChart` 返却に変更:

`internal/usecase/source_table_usecase_test.go:113` 付近:
```go
func (r *fakeSourceRepo) LoadCharts(_ context.Context, id string, q port.ChartQuery) ([]domain.EnrichedChart, error) {
	src := r.charts[id]
	out := make([]domain.EnrichedChart, 0, len(src))
	for _, c := range src {
		ec := domain.EnrichedChart{SourceChart: c}
		// テストで OwnedOnly を再現する場合は ec.IsOwned を別経路で立てる必要があるが、
		// pick_usecase_test では setSourceOwned ヘルパで管理する (下記)
		out = append(out, ec)
	}
	if q.OwnedOnly {
		// テストヘルパで IsOwned=true マークされた譜面のみ返す
		filtered := out[:0]
		for _, ec := range out {
			if ec.IsOwned {
				filtered = append(filtered, ec)
			}
		}
		out = filtered
	}
	return out, nil
}
```

pick_usecase_test の `seedSource` も EnrichedChart に切替する。owned 表現のため、追加でヘルパを新設:
```go
// fakeSourceRepo に owned md5 を覚えさせて、LoadCharts で IsOwned を立てる
// (実 adapter は SongdataAttacher 経由で SQL JOIN するが、fake では in-memory set)
func (r *fakeSourceRepo) markOwned(md5s ...string) {
	if r.ownedSet == nil {
		r.ownedSet = map[string]struct{}{}
	}
	for _, m := range md5s {
		r.ownedSet[m] = struct{}{}
	}
}
```

`fakeSourceRepo` の構造体に `ownedSet map[string]struct{}` フィールドを足し、`LoadCharts` で `_, ok := r.ownedSet[c.MD5]` を見て `ec.IsOwned = ok` をスタンプする形に修正:
```go
func (r *fakeSourceRepo) LoadCharts(_ context.Context, id string, q port.ChartQuery) ([]domain.EnrichedChart, error) {
	src := r.charts[id]
	out := make([]domain.EnrichedChart, 0, len(src))
	for _, c := range src {
		_, owned := r.ownedSet[c.MD5]
		ec := domain.EnrichedChart{SourceChart: c, IsOwned: owned}
		if q.OwnedOnly && !owned {
			continue
		}
		out = append(out, ec)
	}
	return out, nil
}
```

`TestPickUseCase_OwnedOnlyFiltersBeforePick` を `f.srcRepo.markOwned("owned-1", "owned-2")` を呼ぶ形に書き換え:
```go
func TestPickUseCase_OwnedOnlyFiltersBeforePick(t *testing.T) {
	f := newPickUCFixture(t)
	f.seedSource(t, "SRC1", []string{"0"}, domain.FetchStatusOK, []domain.SourceChart{
		chartFixture("SRC1", "0", 0, "owned-1"),
		chartFixture("SRC1", "0", 1, "not-owned"),
		chartFixture("SRC1", "0", 2, "owned-2"),
	})
	f.seedPub(t, "PUB1", "p1", "SRC1", true, 0, domain.RefreshModePerRequest)
	f.srcRepo.markOwned("owned-1", "owned-2")

	r, _, err := f.uc.PickBySlug(context.Background(), "p1")
	require.NoError(t, err)
	require.Len(t, r.Charts, 2)
	for _, c := range r.Charts {
		require.NotEqual(t, "not-owned", c.MD5)
	}
}
```

`TestPickUseCase_OwnedOnly_NoOwnedReturnsEmpty` は `markOwned` を呼ばないだけで再現できる。`f.cfg` / `f.ownedRep` 関連の行は削除。

- [ ] **Step 6: handler テストの fake を更新**

`internal/app/handler/source_table_handler_test.go:69` の fake:
```go
func (r *sourceFakeRepo) LoadCharts(_ context.Context, _ string, _ port.ChartQuery) ([]domain.EnrichedChart, error) {
	return nil, nil
}
```
import に `port` が無ければ追加。

- [ ] **Step 7: ビルド確認**

Run: `go build ./...`
Expected: 失敗 → エラーメッセージで指摘された残りの呼び出し箇所を全て修正。`Bootstrap` (`internal/app/bootstrap.go:81`) で `NewSourceTableRepoSQL(db)` の呼び出しが attacher 引数不足で失敗するはずだが、これは Task 5 で本格対応する。**暫定対応**として Bootstrap 内で:
```go
sourceAttacher := persistence.NewSongdataAttacher(db, systemClock, lg)
sourceRepo := persistence.NewSourceTableRepoSQL(db, sourceAttacher)
```
として attacher を `Bootstrap` 関数内ローカル変数で構築する (Task 5 で `Services` に移す)。`systemClock` の宣言位置が後ろなら、宣言を上に移動する。

- [ ] **Step 8: テスト実行**

Run: `go test ./... -count=1`
Expected: PASS (testdata 不要なテスト全て、`testdata/songdata.db` 系はスキップ可)

- [ ] **Step 9: コミット**

```bash
git add internal/port/source_table_repo.go \
        internal/adapter/persistence/source_table_repo.go \
        internal/adapter/persistence/source_table_repo_test.go \
        internal/usecase/pick_usecase.go \
        internal/usecase/pick_usecase_test.go \
        internal/usecase/source_table_usecase_test.go \
        internal/app/handler/source_table_handler_test.go \
        internal/app/bootstrap.go
git commit -m "refactor(port,usecase): LoadCharts を ChartQuery+EnrichedChart シグネチャに変更"
```

---

## Task 5: `Bootstrap` を整理 (Attacher を Services に登録 + 起動時 ATTACH + Hook rewire)

**Files:**
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/usecase/config_usecase.go` (コメントの更新のみ、API 変更なし)

- [ ] **Step 1: Services 構造体に Attacher を追加**

`bootstrap.go` の `Services` に追加:
```go
type Services struct {
	// ... 既存フィールド ...
	SongdataAttacher *persistence.SongdataAttacher
}
```

- [ ] **Step 2: Bootstrap で SetMaxOpenConns(1) + 起動時 ATTACH を実行**

`bootstrap.go` の DB オープン部 (`:65` 周辺) に追加:
```go
db, err := persistence.OpenDB(dbPath)
if err != nil {
	_ = closeLog()
	return nil, fmt.Errorf("db open: %w", err)
}
// ATTACH DATABASE はコネクション単位なので 1 接続に固定する
db.SetMaxOpenConns(1)
```

systemClock を attacher 構築の前に移動 (既存 `systemClock := clock.System{}` を `sourceRepo` 構築前に置く)。

attacher 構築 + 起動時 ATTACH を `pubRepo` 構築前あたりに追加:
```go
systemClock := clock.System{}
sourceAttacher := persistence.NewSongdataAttacher(db, systemClock, lg)

// 起動時に songdata.db が設定済みなら ATTACH を試みる (失敗しても起動継続)
{
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	configuredPath, _, err := configStore.Get(bgCtx, "songdata_db_path")
	if err != nil {
		lg.Warn("read songdata_db_path failed", "err", err)
	} else if configuredPath != "" {
		if err := sourceAttacher.Attach(bgCtx, configuredPath); err != nil {
			lg.Warn("startup songdata attach failed", "err", err, "path", configuredPath)
		}
	}
}

sourceRepo := persistence.NewSourceTableRepoSQL(db, sourceAttacher)
```

- [ ] **Step 3: `songdata_db_path` 変更フックを `OwnedMD5Cache` から `SongdataAttacher` に切替**

`bootstrap.go:120-123` を置換:
```go
// songdata_db_path 変更時に sd を再アタッチ + ピックキャッシュを clear
configUC.AddSongdataPathChangeHook(func() {
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	newPath, _ := configUC.GetSongdataDBPath(bgCtx)
	if err := sourceAttacher.ReAttach(bgCtx, newPath); err != nil {
		lg.Warn("re-attach songdata failed", "err", err, "path", newPath)
	}
	pickUC.InvalidateAll()
})
```

`ownedCache` / `ownedRepo` / `ownedHandler` の構築行は **まだ削除しない** (Task 7 で削除)。`ownedCache` を引き続き受ける `pickUC := usecase.NewPickUseCase(...)` も Task 4 で `owned` 引数を消した想定。Task 4 完了後の Bootstrap は既に下記のようになっているはず:
```go
pickUC := usecase.NewPickUseCase(pubRepo, sourceRepo, pickStore, systemClock, randFactory, lg)
```

- [ ] **Step 4: Services の戻り値に Attacher を追加**

```go
return &Services{
	// ... 既存 ...
	SongdataAttacher: sourceAttacher,
}, nil
```

- [ ] **Step 5: ビルド + テスト**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/app/bootstrap.go
git commit -m "refactor(app): SongdataAttacher 起動時 ATTACH + ConfigUseCase hook rewire"
```

---

## Task 6: `OwnedChartHandler` を `SongdataHandler` に置換

**Files:**
- Create: `internal/app/handler/songdata_handler.go`
- Create: `internal/app/handler/songdata_handler_test.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `app.go`
- Modify: `main.go`
- Delete: `internal/app/handler/owned_chart_handler.go` (Task 8 でまとめて削除)

- [ ] **Step 1: 新ハンドラを作成**

`internal/app/handler/songdata_handler.go`:
```go
package handler

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// SongdataAttachStatusDTO は GetSongdataAttachStatus が返す DTO。
type SongdataAttachStatusDTO struct {
	Attached   bool   `json:"attached"`
	Path       string `json:"path"`
	SongCount  int    `json:"songCount"`
	AttachedAt string `json:"attachedAt"`
	LastError  string `json:"lastError"`
}

// SongdataHandler は Wails Bind 経由で songdata.db ATTACH 状態と再アタッチ API を公開する。
type SongdataHandler struct {
	attacher *persistence.SongdataAttacher
	configUC *usecase.ConfigUseCase
	pickUC   *usecase.PickUseCase
	ctx      context.Context
}

// NewSongdataHandler は新しい SongdataHandler を作る。
func NewSongdataHandler(
	attacher *persistence.SongdataAttacher,
	configUC *usecase.ConfigUseCase,
	pickUC *usecase.PickUseCase,
) *SongdataHandler {
	return &SongdataHandler{attacher: attacher, configUC: configUC, pickUC: pickUC, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *SongdataHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// GetSongdataAttachStatus は現在のアタッチ状態を返す。
func (h *SongdataHandler) GetSongdataAttachStatus() SongdataAttachStatusDTO {
	st := h.attacher.Status()
	out := SongdataAttachStatusDTO{
		Attached:  st.Attached,
		Path:      st.Path,
		SongCount: st.SongCount,
		LastError: st.LastError,
	}
	if st.AttachedAt != nil {
		out.AttachedAt = st.AttachedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ReattachSongdata は現在の songdata_db_path 設定で ATTACH をやり直す。
// GUI の「再アタッチ」ボタンから呼ばれる。
func (h *SongdataHandler) ReattachSongdata() error {
	path, err := h.configUC.GetSongdataDBPath(h.ctx)
	if err != nil {
		return err
	}
	if err := h.attacher.ReAttach(h.ctx, path); err != nil {
		return err
	}
	h.pickUC.InvalidateAll()
	return nil
}
```

- [ ] **Step 2: 簡単なハンドラテスト**

`internal/app/handler/songdata_handler_test.go`:
```go
package handler_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func setupHandlerDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	require.NoError(t, persistence.RunMigrations(db))
	return db
}

func TestSongdataHandler_GetStatus_NotAttached(t *testing.T) {
	db := setupHandlerDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := persistence.NewSongdataAttacher(db, clock.System{}, logger)
	configUC := usecase.NewConfigUseCase(persistence.NewConfigStoreSQL(db))
	// PickUseCase はテスト用に nil 引数で作成 (status のみ呼ぶので問題ない)
	pickUC := usecase.NewPickUseCase(nil, nil, usecase.NewPickResultStore(), clock.System{}, nil, logger)

	h := handler.NewSongdataHandler(a, configUC, pickUC)
	h.SetContext(context.Background())

	st := h.GetSongdataAttachStatus()
	require.False(t, st.Attached)
	require.Equal(t, 0, st.SongCount)
	require.Empty(t, st.AttachedAt)
}
```

- [ ] **Step 3: Bootstrap を更新**

`bootstrap.go`:
```go
// 旧:
ownedHandler := handler.NewOwnedChartHandler(ownedCache)

// 新 (ownedHandler 行を置換):
songdataHandler := handler.NewSongdataHandler(sourceAttacher, configUC, pickUC)
```

`Services` 構造体:
```go
type Services struct {
	// ... 既存 ...
	SongdataHandler  *handler.SongdataHandler
	// OwnedChartHandler は Task 8 で削除 (それまでは併存)
}
```

戻り値にも `SongdataHandler: songdataHandler` を追加。

- [ ] **Step 4: Wails Bind を切替**

`main.go` の `Bind:` に追加 (旧の OwnedChartHandler は併存させて Task 8 で除去):
```go
Bind: []any{
	myApp,
	services.ConfigHandler,
	services.SourceTableHandler,
	services.PublishedTableHandler,
	services.PickHandler,
	services.ServerStatusHandler,
	services.OwnedChartHandler,    // Task 8 で削除
	services.SongdataHandler,      // 追加
	services.DashboardHandler,
},
```

`app.go` の `startup` 内 SetContext 呼び出しに追加:
```go
a.services.SongdataHandler.SetContext(ctx)
```

- [ ] **Step 5: Wails 型再生成**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/handler/SongdataHandler.{js,d.ts}` が生成される

(失敗時は `cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor && wails generate module` を確認。`wails` バイナリは `~/go/bin/wails` か `go install github.com/wailsapp/wails/v2/cmd/wails@latest` で導入済み前提。)

- [ ] **Step 6: ビルド + テスト**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add internal/app/handler/songdata_handler.go \
        internal/app/handler/songdata_handler_test.go \
        internal/app/bootstrap.go \
        app.go main.go
git commit -m "feat(handler): SongdataHandler を追加 (ATTACH 状態と再アタッチ API)"
```

---

## Task 7: httpserver から `Owned` 依存を除去

`Owned` は実装上の参照箇所が `Deps` 構造体と `handler_test_helpers_test.go` のみ (実エンドポイントでは使われていない `// Task 15 で使用` コメント済み)。

**Files:**
- Modify: `internal/adapter/httpserver/server.go`
- Modify: `internal/adapter/httpserver/handler_test_helpers_test.go`
- Modify: `internal/app/bootstrap.go`

- [ ] **Step 1: `Deps` から `Owned` フィールドを削除**

`server.go` の `Deps`:
```go
type Deps struct {
	Pick      *usecase.PickUseCase
	Pub       *usecase.PublishedTableUseCase
	Dashboard *usecase.DashboardUseCase
	Log       *slog.Logger
}
```

- [ ] **Step 2: Bootstrap の Deps 組み立てから `Owned:` を除去**

`bootstrap.go` の `httpFactory`:
```go
httpFactory := func(addr string) usecase.HTTPServer {
	return httpserver.New(addr, httpserver.Deps{
		Pick:      pickUC,
		Pub:       pubUC,
		Dashboard: dashboardUC,
		Log:       lg,
	})
}
```

- [ ] **Step 3: テストヘルパ修正**

`handler_test_helpers_test.go` から:
- `fakeOwnedRepo`, `LoadOwnedMD5Set` 定義 (`:108-119`) を削除
- `newPrimedOwnedCache` ヘルパ (`:132-...`) を削除
- `Deps{}` 組み立てで `Owned:` を渡している箇所があれば除去
- 上記以外で `usecase.OwnedMD5Cache` / `port.OwnedChartRepo` への参照があれば全て除去

- [ ] **Step 4: ビルド + テスト**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/httpserver/server.go \
        internal/adapter/httpserver/handler_test_helpers_test.go \
        internal/app/bootstrap.go
git commit -m "refactor(httpserver): Deps から Owned 依存を除去"
```

---

## Task 8: 不要コード一掃

**Files (削除):**
- Delete: `internal/usecase/owned_md5_cache.go`
- Delete: `internal/usecase/owned_md5_cache_test.go`
- Delete: `internal/port/owned_chart_repo.go`
- Delete: `internal/adapter/persistence/songdata_reader.go`
- Delete: `internal/adapter/persistence/songdata_reader_test.go`
- Delete: `internal/app/handler/owned_chart_handler.go`

**Files (変更):**
- Modify: `internal/app/bootstrap.go`
- Modify: `main.go`

- [ ] **Step 1: 旧コードを削除**

```bash
rm internal/usecase/owned_md5_cache.go \
   internal/usecase/owned_md5_cache_test.go \
   internal/port/owned_chart_repo.go \
   internal/adapter/persistence/songdata_reader.go \
   internal/adapter/persistence/songdata_reader_test.go \
   internal/app/handler/owned_chart_handler.go
```

- [ ] **Step 2: Bootstrap から旧構築コードを除去**

`bootstrap.go` で以下の行を削除:
```go
ownedRepo := persistence.NewSongdataReader()                                  // 削除
ownedCache := usecase.NewOwnedMD5Cache(ownedRepo, configStore, systemClock, lg)  // 削除
// Services.OwnedChartHandler フィールドも削除
// 戻り値の OwnedChartHandler: ownedHandler 行も削除
```

`Services` 構造体から `OwnedChartHandler *handler.OwnedChartHandler` を削除。

- [ ] **Step 3: main.go の Wails Bind から OwnedChartHandler を削除**

```go
Bind: []any{
	myApp,
	services.ConfigHandler,
	services.SourceTableHandler,
	services.PublishedTableHandler,
	services.PickHandler,
	services.ServerStatusHandler,
	services.SongdataHandler,
	services.DashboardHandler,
},
```

`app.go` の startup から `a.services.OwnedChartHandler.SetContext(ctx)` を削除。

- [ ] **Step 4: Wails 型再生成**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/handler/OwnedChartHandler.{js,d.ts}` が削除される (もしくは `frontend/wailsjs/` 配下を一旦 `rm -rf` して再生成)

- [ ] **Step 5: ビルド + テスト**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS

- [ ] **Step 6: 残留参照のチェック**

Run:
```bash
grep -rn "OwnedMD5Cache\|OwnedChartRepo\|OwnedCacheStatus\|LoadOwnedMD5Set\|SongdataReader" \
  --include="*.go" /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
```
Expected: 出力なし (frontend 側は次のタスクで処理する)

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "refactor: OwnedMD5Cache / OwnedChartRepo / SongdataReader を削除"
```

---

## Task 9: フロントエンド更新 (api.ts + ServerTab.svelte)

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/lib/tabs/ServerTab.svelte`

- [ ] **Step 1: api.ts の型/import 切替**

`frontend/src/lib/api.ts`:

旧の `OwnedChartHandler` import 群 (`:33-35`) を新ハンドラに置換:
```ts
import {
  GetSongdataAttachStatus,
  ReattachSongdata,
} from '../../wailsjs/go/handler/SongdataHandler';
```

旧 `OwnedCacheStatusDTO` 型 (`:102` 周辺) を置換:
```ts
export type SongdataAttachStatusDTO = {
  attached: boolean;
  path: string;
  songCount: number;
  attachedAt: string;
  lastError: string;
};
```

旧メソッド (`getOwnedCacheStatus`, `reloadOwnedCache`) を置換:
```ts
getSongdataAttachStatus(): Promise<SongdataAttachStatusDTO> {
  return GetSongdataAttachStatus() as Promise<SongdataAttachStatusDTO>;
},
reattachSongdata(): Promise<void> {
  return ReattachSongdata();
},
```

`api.ts` 内で `OwnedCacheStatusDTO` / `getOwnedCacheStatus` / `reloadOwnedCache` を参照している箇所が他にあれば全て削除。

- [ ] **Step 2: ServerTab.svelte の状態変数を切替**

`ServerTab.svelte` の冒頭 type import を更新:
```ts
import { api, type ServerConfig, type ServerStatusDTO, type SongdataAttachStatusDTO } from '../api';
```

`owned` 変数を `attach` にリネーム + 型変更:
```ts
let attach: SongdataAttachStatusDTO = { attached: false, path: '', songCount: 0, attachedAt: '', lastError: '' };
let attachLoading = false;
let attachActing = false;
```

`owned = await api.getOwnedCacheStatus();` の呼び出しを `attach = await api.getSongdataAttachStatus();` に置換 (3 箇所)。

`async function reloadOwned()` を `reattach` に書き換え:
```ts
async function reattach() {
  attachActing = true;
  try {
    await api.reattachSongdata();
    attach = await api.getSongdataAttachStatus();
  } finally {
    attachActing = false;
  }
}
```

- [ ] **Step 3: songdata.db パス設定カードに ATTACH ステータス表示を追加**

ServerTab.svelte の「設定 (port + songdata.db のパス)」カード内、songdata.db パス入力 (`:146-152`) の直下に追加:
```svelte
<div class="text-sm space-y-1 mt-1">
  <div class="flex items-center gap-2">
    <span>状態:</span>
    {#if attach.attached}
      <span class="badge badge-success">アタッチ済</span>
      <span class="text-xs opacity-70">{attach.songCount} 曲</span>
    {:else if attach.lastError}
      <span class="badge badge-error">エラー</span>
    {:else}
      <span class="badge">未設定</span>
    {/if}
  </div>
  {#if attach.attachedAt}
    <div class="text-xs opacity-70">最終アタッチ: {formatJST(attach.attachedAt)}</div>
  {/if}
  {#if attach.lastError}
    <div class="alert alert-warning text-xs whitespace-pre-line">{attach.lastError}</div>
  {/if}
  <div class="flex justify-end">
    <button class="btn btn-xs" disabled={attachActing} on:click={reattach}>
      {#if attachActing}<span class="loading loading-spinner loading-xs"></span>{/if}
      再アタッチ
    </button>
  </div>
</div>
```

- [ ] **Step 4: 旧「所持キャッシュ」カードを削除**

ServerTab.svelte の `<!-- 所持キャッシュ -->` カード ブロック (`:194-220` 周辺) を **丸ごと削除**。

- [ ] **Step 5: 型チェックとビルド確認**

Run: `cd frontend && npm run check`
Expected: 型エラーなし

Run: `cd frontend && npm run build`
Expected: ビルド成功

- [ ] **Step 6: コミット**

```bash
git add frontend/src/lib/api.ts frontend/src/lib/tabs/ServerTab.svelte
git commit -m "feat(frontend): songdata.db カードに ATTACH ステータス表示を追加"
```

---

## Task 10: 統合確認 + push

**Files:** なし (検証のみ)

- [ ] **Step 1: 全テスト + lint**

Run:
```bash
go test $(go list ./... | grep -v internal/adapter/persistence) -count=1
make lint
```
Expected: PASS

(testdata/songdata.db を持っているなら `go test ./...` 全体を流す)

- [ ] **Step 2: 残留 grep**

Run:
```bash
grep -rn "OwnedMD5Cache\|OwnedChartRepo\|OwnedCacheStatus\|LoadOwnedMD5Set\|SongdataReader" \
  --include="*.go" --include="*.ts" --include="*.svelte" \
  /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
```
Expected: 出力なし (`frontend/wailsjs/` は自動生成なので除外を確認)

- [ ] **Step 3: dev で手動確認**

Run: `make dev`

GUI で確認すること:
- songdata.db パス未設定の状態で起動 → 設定タブの songdata.db カードに「未設定」バッジ
- 既存 `OwnedOnly=true` の公開表にブラウザでアクセス → 0 件で返る
- songdata.db パスを設定 → 保存後にカード状態が「アタッチ済」+ 楽曲数表示
- 同じ公開表にアクセス → 所持譜面のみ返る
- パスを別の値に変更 → 自動で再アタッチ、ピックも更新

**確認できないなら明示的に「UI テストできず」と報告すること** (CLAUDE.md ルール)。

- [ ] **Step 4: ピック決定論性の手動確認 (regression)**

`daily` モードの公開表を `data.json` で取得 → アプリを再起動 → 同じ日付内なら同じ並びが返ること。Plan 4 の bug fix が壊れていないことを確認。

- [ ] **Step 5: main に push**

Run: `git push origin main`
Expected: 成功 (Windows ビルドが反映される)

- [ ] **Step 6: メモリ削減の任意確認**

`testdata/songdata.db` に大量譜面が入っているなら `runtime.ReadMemStats` で BeforeAlloc/AfterAlloc を比較する簡易ベンチを書いてもよい (任意、必須ではない)。

---

## 自己レビュー (プラン作成者用)

- [x] スペック §3.1 EnrichedChart → Task 1
- [x] スペック §3.1 SongdataAttachStatus → Task 2
- [x] スペック §3.2 ChartQuery / LoadCharts シグネチャ変更 → Task 2 + Task 4
- [x] スペック §3.3 SongdataAttacher → Task 3
- [x] スペック §3.4 SourceTableRepoSQL.LoadCharts (attached/bare 分岐) → Task 4
- [x] スペック §3.5 PickUseCase 改修 → Task 4
- [x] スペック §3.6 Bootstrap / ConfigUseCase rewire → Task 5
- [x] スペック §3.7 ステータス UI → Task 6 (handler) + Task 9 (frontend)
- [x] スペック §3.8 削除リスト → Task 7 + Task 8
- [x] スペック §5 テスト戦略 → 各 Task 内に TDD ステップ
- [x] スペック §8 受け入れ基準 → Task 10 で確認
