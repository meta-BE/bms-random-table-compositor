# Phase 1 / Plan 1: 基盤（プロジェクト雛形 + DB + 設定 + トレイ + ロック + ログ）実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** リポジトリルートに `bms-random-table-compositor` 本体の Wails アプリを展開し、DBマイグレーション、`ConfigStore`、ロギング、シングルインスタンスロック、システムトレイ常駐、GUI 最小設定画面まで揃え、「常駐するだけのアプリ」を macOS と Windows で動作させる。

**Architecture:** bms-elsa の `internal/{domain,port,usecase,adapter,app}` クリーンアーキテクチャ風レイアウトを踏襲。`compositor.db` (modernc/sqlite) に config + 後続Plan用空テーブルを冪等マイグレーションで構築。設定の Get/Set は `ConfigStore` ポート＋adapter＋Wails Bind ハンドラで分離。常駐は `getlantern/systray` を採用、`OnBeforeClose` で `runtime.WindowHide` してトレイ格納。シングルインスタンスは `os.Executable()` 隣の `.lock` ファイルへのファイルロック。

**Tech Stack:** Go 1.24 / Wails v2.11.0 / Svelte + TypeScript + Vite / `modernc.org/sqlite` / `getlantern/systray` / `gopkg.in/natefinch/lumberjack.v2` / `oklog/ulid/v2` / `stretchr/testify`

**設計ドキュメント:** `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` の Phase 1（章 1〜13、16）。本Plan は Phase 1 の **基盤レイヤー** に絞る。

**Phase 1 全体の Plan 分割:** Plan 1（本ファイル）→ Plan 2（ソース表取り込み） → Plan 3（公開表+ピック+HTTPサーバ） → Plan 4（GUI仕上げ+E2E）

**完了条件:**

- リポジトリルートに本体 Wails アプリ（main.go / app.go / go.mod / wails.json / frontend/）が展開され、`go build ./...` と `wails build` がパス
- `compositor.db` が初回起動時に生成され、`config / source_table / source_table_chart / published_table` の4テーブルが存在
- 「ポート番号」「songdata.db パス」を画面で入力→保存→再起動後も値が保持される
- `./logs/YYYY-MM-DD.log` にアプリ起動ログが出力される
- 二重起動を防ぐ（既存インスタンスがあれば、新インスタンスは即終了 + 既存窓を前面化）
- ウィンドウクローズでトレイに格納され、トレイメニューから「設定再表示」「終了」可能
- 既存 `.github/workflows/poc-build-windows.yml` とは別に `.github/workflows/build-windows.yml` を追加し、本体exe `bms-random-table-compositor.exe` を `workflow_dispatch` で生成 → Windows 実機/VM で起動確認
- 既存の `poc/` ディレクトリ・POC アセット・docs は維持

**スコープ外（Plan 2 以降）:** ソース表取り込み (Plan 2) / 公開表 / ピックエンジン / 所持判定 / HTTPサーバ・3エンドポイント / HTMLビュー / ダッシュボード / ソース表バックグラウンド更新

**ブランチ運用:** main 上で直接コミット（ユーザー判断、`workflow_dispatch` がデフォルトブランチ参照のためfeatureブランチ運用が煩雑）。

---

## ファイル構造（Plan 1 終了時点）

```
bms-random-table-compositor/
├── main.go                                    # 新規 (wails init 派生)
├── app.go                                     # 新規 (wails init 派生 + 本Plan拡張)
├── go.mod / go.sum                            # 新規
├── wails.json                                 # 新規
├── Makefile                                   # 新規
├── README.md                                  # 既存（最小内容、本Plan で更新）
├── CLAUDE.md                                  # 新規
├── .gitignore                                 # 既存（Plan 1 で本体向けエントリ追記）
├── .github/workflows/
│   ├── poc-build-windows.yml                  # 既存（POC 用、保持）
│   └── build-windows.yml                      # 新規（本体用）
├── docs/                                      # 既存（保持）
├── poc/                                       # 既存（保持）
├── internal/
│   ├── domain/
│   │   └── config_value.go                    # ConfigKey 型 (Plan 1 はこれだけ)
│   ├── port/
│   │   └── config_store.go                    # ConfigStore インタフェース
│   ├── usecase/
│   │   └── config_usecase.go                  # ConfigUseCase（Get/Set + 既知キー定数）
│   ├── adapter/
│   │   ├── persistence/
│   │   │   ├── db.go                          # OpenDB ヘルパー
│   │   │   ├── migrations.go                  # RunMigrations
│   │   │   ├── migrations_test.go
│   │   │   ├── config_store.go                # SQL 実装
│   │   │   └── config_store_test.go
│   │   ├── logger/
│   │   │   ├── logger.go                      # slog + lumberjack
│   │   │   └── logger_test.go
│   │   ├── singleinstance/
│   │   │   ├── lock.go                        # ファイルロック (Linux/macOS:flock, Windows:LockFileEx via syscall)
│   │   │   └── lock_test.go
│   │   ├── tray/
│   │   │   └── tray.go                        # getlantern/systray 起動・色切替
│   │   └── paths/
│   │       └── paths.go                       # 実行ファイル隣のパス解決ヘルパー
│   └── app/
│       └── handler/
│           └── config_handler.go              # Wails Bind ターゲット (GetPort/SetPort 等)
├── frontend/
│   ├── package.json
│   ├── svelte.config.js
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── tsconfig.node.json
│   ├── index.html
│   └── src/
│       ├── App.svelte                         # Plan 1: ポート番号 + songdata.db パスのフォーム
│       ├── lib/
│       │   ├── api.ts                         # Wails Bind 呼び出しラッパ
│       │   └── tabs/
│       │       └── ServerTab.svelte           # 設定タブ（Plan 1）
│       ├── main.ts
│       ├── style.css
│       └── vite-env.d.ts
└── compositor.db                              # 実行時生成（gitignore済）
```

各ファイルの責務:

| ファイル | 責務 |
|---|---|
| `main.go` | Wails のエントリ。`wails.Run` 呼び出し、Bind 配列、OnStartup/OnBeforeClose/OnShutdown フック |
| `app.go` | `App` 構造体（`ConfigHandler`, `tray`, `db`, `lock`）、ライフサイクル |
| `internal/adapter/paths/paths.go` | 実行ファイル隣のパス算出（`compositor.db`, `logs/`, `.lock`） |
| `internal/adapter/persistence/db.go` | `OpenDB(path)` で `*sql.DB` を返す |
| `internal/adapter/persistence/migrations.go` | `RunMigrations(db)` で4テーブル + 1インデックスを冪等作成 |
| `internal/port/config_store.go` | `ConfigStore` インタフェース定義 |
| `internal/adapter/persistence/config_store.go` | SQL 実装 |
| `internal/usecase/config_usecase.go` | キー定数 + Get/Set ファサード（バリデーション含む） |
| `internal/app/handler/config_handler.go` | Wails Bind: `GetServerConfig`, `SetServerPort`, `SetSongdataDBPath` |
| `internal/adapter/logger/logger.go` | slog ハンドラ + lumberjack 出力 |
| `internal/adapter/singleinstance/lock.go` | ファイルロック取得・解放 |
| `internal/adapter/tray/tray.go` | systray 起動 + メニュー + アイコン色切替（緑/灰/赤） |
| `frontend/src/lib/api.ts` | Bind ラッパ |
| `frontend/src/App.svelte` | タブ切替の親（Plan 1 はサーバ設定タブのみ） |

---

## 前提条件と注意

- 開発機 macOS / Apple Silicon / Wails CLI v2.11.0 / Node 20+ / Go 1.24+ がインストール済み（Phase 0 で確認済み）
- 作業ブランチは **main**（ユーザー指定。`workflow_dispatch` のデフォルトブランチ要件を回避するため）
- リポジトリルートに既存の `docs/`, `poc/`, `testdata/`, `.gitignore` があることに注意。`wails init` を直接ルートに走らせると失敗する可能性が高いため、**一時ディレクトリで生成 → ファイルを選択的にコピー** する方式を採る（Task 1 で詳述）
- POC で得た知見は `poc/NOTES.md` 参照
- POC の `poc-build-windows.yml` は保持。本体用は `build-windows.yml` として **新規作成**

---

## Task 1: リポジトリルートに本体 Wails アプリ雛形を展開

**Files:**

- Create (via wails CLI through 一時ディレクトリ): `main.go`, `app.go`, `go.mod`, `go.sum`, `wails.json`, `frontend/...`, `build/...`
- Modify: `.gitignore`（リポジトリルート、本体向けエントリ追記）

- [ ] **Step 1: 既存リポジトリ状態の確認**

Run:
```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git status
ls
```

Expected: `docs/`, `poc/`, `testdata/`, `.gitignore`, `README.md` が見える。`main.go` 等は存在しない。

- [ ] **Step 2: 一時ディレクトリで wails init を実行**

Run:
```bash
mkdir -p /tmp/bms-rtc-init
cd /tmp/bms-rtc-init
wails init -n bms-random-table-compositor -t svelte-ts -d .
ls
```

Expected: `/tmp/bms-rtc-init/` に `main.go`, `app.go`, `go.mod`, `wails.json`, `frontend/`, `build/` などが生成される。

- [ ] **Step 3: 必要ファイルをリポジトリルートにコピー**

Run:
```bash
cd /tmp/bms-rtc-init
cp main.go app.go go.mod go.sum wails.json /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/
cp -R frontend /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/
cp -R build /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/
cp .gitignore /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/.gitignore.wails-template
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
```

Expected: ルート直下に `main.go`, `app.go`, `go.mod`, `go.sum`, `wails.json`, `frontend/`, `build/` がコピーされる。`.gitignore.wails-template` は次のステップで内容を merge するため一時的に保持。

- [ ] **Step 4: ルートの `.gitignore` を更新（既存内容を維持しつつ本体向けエントリ追加）**

`.gitignore` を以下の内容に**置換**（既存の POC 関連エントリも維持）:

```gitignore
# Wails build artifacts (POC)
poc/build/bin/
poc/frontend/dist/
poc/frontend/node_modules/
poc/frontend/wailsjs/

# POC runtime artifacts
poc/poc-config.json

# Wails build artifacts (main app)
/build/bin/
/frontend/dist/
/frontend/node_modules/
/frontend/wailsjs/

# Main app runtime artifacts
/compositor.db
/compositor.db-shm
/compositor.db-wal
/.lock
/logs/

# OS / Editor
.DS_Store
*.swp
.idea/
.vscode/

# Temp
tmp/
```

その後、一時ファイルを削除:
```bash
rm .gitignore.wails-template
```

- [ ] **Step 5: `go.mod` のモジュール名を修正**

`wails init` の生成する `go.mod` は `module changeme` または `module bms-random-table-compositor` 等になっている。リポジトリの GitHub パスに合わせて修正する。

`go.mod` の冒頭行を以下に変更:

```go
module github.com/meta-BE/bms-random-table-compositor
```

その後 `go mod tidy` を実行:

```bash
go mod tidy
```

Expected: モジュールパスが更新され、`go.sum` も再生成される。

- [ ] **Step 6: 不要な wails テンプレ assets の整理**

`frontend/src/assets/images/logo-universal.png` 等のテンプレロゴは Plan 4 でデザイン整備時に決定するので、本Plan ではいったん保持してOK。`frontend/src/App.svelte` は wails init 生成のまま（次の Task 13/14 で書き換える）。

- [ ] **Step 7: ビルド成功確認**

Run:
```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
go build ./...
wails build
ls build/bin/
```

Expected: `bms-random-table-compositor.app` が生成される。`go build` も成功（追加の internal/* がない時点では main パッケージのみ）。

- [ ] **Step 8: コミット**

```bash
git add main.go app.go go.mod go.sum wails.json frontend/ build/ .gitignore
git commit -m "$(cat <<'EOF'
chore: 本体 Wails アプリ雛形をリポジトリルートに展開

wails init -n bms-random-table-compositor -t svelte-ts のテンプレを
ルート直下にコピー。既存の poc/、docs/ は維持。
.gitignore に本体ビルド成果物 (compositor.db/.lock/logs/) と
POC 既存エントリの両方を含める。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

Expected: コミット成功。`git show --stat HEAD` で `frontend/` 配下のテンプレファイル、`main.go`, `app.go`, `go.mod`, `wails.json`, `build/`、`.gitignore` の変更が含まれる。

---

## Task 2: Makefile と CLAUDE.md を整備

**Files:**

- Create: `Makefile`
- Create: `CLAUDE.md`

- [ ] **Step 1: `Makefile` を作成**

```makefile
.PHONY: dev build build-windows test lint vet fmt-check clean

dev:
	wails dev

build:
	wails build

build-windows:
	wails build -platform windows/amd64

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "次のファイルが gofmt されていません:"; \
		gofmt -l .; \
		exit 1; \
	fi

lint: vet fmt-check

clean:
	rm -rf build/bin/* frontend/dist/ frontend/wailsjs/
```

- [ ] **Step 2: `CLAUDE.md` を作成**

bms-elsa 流儀のプロジェクト固有指示:

```markdown
# BMS Random Table Compositor

## プロジェクト概要

既存BMS難易度表をローカルで再ホストし、ランダム選曲・所持限定・合成等の編集を加えて beatoraja に提供するWindows向けデスクトップアプリ。
詳細は `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` 参照。

## ビルド
- コンパイル確認には `go build ./...` を使う（`go build .` はバイナリ出力するため不可）

## マイグレーション
- スキーマ変更は `internal/adapter/persistence/migrations.go` に冪等な `CREATE IF NOT EXISTS` / `ALTER TABLE` として追加
- 既存DBが壊れないよう `pragma_table_info` チェックで保護
- 必ずユニットテストを追加 (`internal/adapter/persistence/migrations_test.go`)

## フロントエンド
- 設定画面のUI規約は `docs/style-guide.md` に従う（後続 Plan で整備）

## マニュアル
- ユーザー向けは `docs/manual.md`（後続 Plan で整備）。機能追加・変更時は更新

## POC
- `poc/` 配下は Phase 0 POC の参照用コードベース。**本体実装はリポジトリルート直下**
- POC で得た知見は `poc/NOTES.md` を参照
```

- [ ] **Step 3: `make test` でテストが空通り確認**

Run:
```bash
make test
```

Expected: `?   	github.com/meta-BE/bms-random-table-compositor	[no test files]` のような出力。失敗なし。

- [ ] **Step 4: コミット**

```bash
git add Makefile CLAUDE.md
git commit -m "$(cat <<'EOF'
chore: Makefile と CLAUDE.md を追加

bms-elsa流儀でプロジェクト固有のビルド・テスト・マイグレーション
ガイドラインを記載。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: 内部パッケージディレクトリ構造を作成

**Files:**

- Create: `internal/domain/doc.go`
- Create: `internal/port/doc.go`
- Create: `internal/usecase/doc.go`
- Create: `internal/adapter/doc.go`
- Create: `internal/app/doc.go`

- [ ] **Step 1: 各パッケージに `doc.go` を作成**

`internal/domain/doc.go`:
```go
// Package domain は本アプリのコアデータ型（SourceTable, PublishedTable, SourceChart 等）
// と純粋関数を提供する。外部 I/O やフレームワーク依存を持たない。
package domain
```

`internal/port/doc.go`:
```go
// Package port は usecase 層が依存する外部リソースのインタフェース定義を提供する。
// 実装は internal/adapter 配下にある。
package port
```

`internal/usecase/doc.go`:
```go
// Package usecase はアプリケーションのビジネスロジック（ピック、所持判定、ソース表更新等）を提供する。
// port を介して外部リソースに依存する。
package usecase
```

`internal/adapter/doc.go`:
```go
// Package adapter は port インタフェースの実装と、外部 I/O を担うアダプタ群（永続化、HTTP、systray 等）を提供する。
package adapter
```

`internal/app/doc.go`:
```go
// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app
```

- [ ] **Step 2: ビルド確認**

Run:
```bash
go build ./...
```

Expected: コンパイルエラーなし。

- [ ] **Step 3: コミット**

```bash
git add internal/
git commit -m "$(cat <<'EOF'
chore: internal パッケージのディレクトリ構造を作成

domain / port / usecase / adapter / app の各パッケージに doc.go を配置。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: パス解決ヘルパー（実行ファイル隣のパス算出）

**Files:**

- Create: `internal/adapter/paths/paths.go`
- Create: `internal/adapter/paths/paths_test.go`

- [ ] **Step 1: 失敗するテストを書く（TDD）**

`internal/adapter/paths/paths_test.go`:
```go
package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableDir_ReturnsAbsolutePath(t *testing.T) {
	dir, err := ExecutableDir()
	if err != nil {
		t.Fatalf("ExecutableDir returned error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if strings.TrimSpace(dir) == "" {
		t.Error("expected non-empty directory path")
	}
}

func TestDBPath_IsExecutableDirCompositorDB(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath returned error: %v", err)
	}
	want := filepath.Join(exe, "compositor.db")
	if got != want {
		t.Errorf("DBPath() = %q, want %q", got, want)
	}
}

func TestLogDir_IsExecutableDirLogs(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := LogDir()
	if err != nil {
		t.Fatalf("LogDir returned error: %v", err)
	}
	want := filepath.Join(exe, "logs")
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}

func TestLockPath_IsExecutableDirDotLock(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath returned error: %v", err)
	}
	want := filepath.Join(exe, ".lock")
	if got != want {
		t.Errorf("LockPath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run:
```bash
go test ./internal/adapter/paths/...
```

Expected: コンパイルエラー（`paths.go` 未作成）。

- [ ] **Step 3: 実装を書く**

`internal/adapter/paths/paths.go`:
```go
// Package paths は実行ファイル隣の各種パス（DB、ログディレクトリ、ロックファイル）を算出するヘルパーを提供する。
package paths

import (
	"os"
	"path/filepath"
)

const (
	dbFilename   = "compositor.db"
	logDirname   = "logs"
	lockFilename = ".lock"
)

// ExecutableDir は実行ファイルが置かれているディレクトリの絶対パスを返す。
// シンボリックリンクは解決済み。
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// シンボリックリンク解決に失敗しても、実行ファイル自体のパスから取得を試みる
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// DBPath は compositor.db の絶対パスを返す。
func DBPath() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFilename), nil
}

// LogDir はログディレクトリの絶対パスを返す。
func LogDir() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logDirname), nil
}

// LockPath はシングルインスタンスロックファイルの絶対パスを返す。
func LockPath() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, lockFilename), nil
}
```

- [ ] **Step 4: テストパス確認**

Run:
```bash
go test ./internal/adapter/paths/...
```

Expected: `ok  	github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths`

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/paths/
git commit -m "$(cat <<'EOF'
feat: 実行ファイル隣のパスを返す paths ヘルパーを追加

DB / ログ / ロックファイルのパスを実行ファイル隣に統一して
ポータブル運用する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: SQLite DB接続ヘルパー

**Files:**

- Create: `internal/adapter/persistence/db.go`
- Create: `internal/adapter/persistence/db_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/persistence/db_test.go`:
```go
package persistence

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenDB_CreatesFileAndPings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
}

func TestOpenDB_EnablesForeignKeys(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys query failed: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: コンパイルエラー (`OpenDB` 未定義)。

- [ ] **Step 3: 実装を書く**

`internal/adapter/persistence/db.go`:
```go
// Package persistence は SQLite を用いた永続化層の実装を提供する。
package persistence

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDB は指定パスの SQLite ファイルを開き、外部キー制約を有効化した *sql.DB を返す。
// ファイルが存在しなければ新規作成される。
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", path, err)
	}

	// SQLite の外部キー制約はデフォルトOFF。明示的に有効化する。
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}

	return db, nil
}
```

- [ ] **Step 4: 依存追加（go.mod 更新）**

Run:
```bash
go get modernc.org/sqlite@v1.46.1
go mod tidy
```

- [ ] **Step 5: テストパス確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: `ok  	github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence`

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/persistence/db.go internal/adapter/persistence/db_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat: SQLite DB 接続ヘルパー OpenDB を追加

外部キー制約を有効化した *sql.DB を返す薄いヘルパー。
modernc.org/sqlite v1.46.1 を採用。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: マイグレーション基盤（4テーブル + インデックス）

**Files:**

- Create: `internal/adapter/persistence/migrations.go`
- Create: `internal/adapter/persistence/migrations_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/persistence/migrations_test.go`:
```go
package persistence

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMigrations_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db))

	for _, table := range []string{"config", "source_table", "source_table_chart", "published_table"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		require.NoError(t, err, "table %s not found", table)
		require.Equal(t, table, name)
	}
}

func TestRunMigrations_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db))
	require.NoError(t, RunMigrations(db), "second migration should succeed")
	require.NoError(t, RunMigrations(db), "third migration should succeed")
}

func TestRunMigrations_CreatesIndexes(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	for _, idx := range []string{"idx_stc_md5", "idx_stc_source_level"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&name)
		require.NoError(t, err, "index %s not found", idx)
	}
}

func TestRunMigrations_SetsSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, RunMigrations(db))

	var v string
	err = db.QueryRow(`SELECT value FROM config WHERE key='schema_version'`).Scan(&v)
	require.NoError(t, err)
	require.Equal(t, "1", v)
}

```

- [ ] **Step 2: testify 依存追加**

Run:
```bash
go get github.com/stretchr/testify@latest
go mod tidy
```

- [ ] **Step 3: テストが失敗することを確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: コンパイルエラー (`RunMigrations` 未定義)。

- [ ] **Step 4: 実装を書く**

`internal/adapter/persistence/migrations.go`:
```go
package persistence

import (
	"database/sql"
	"fmt"
)

// schemaVersion は現在のスキーマバージョン。スキーマ変更時にインクリメント。
const schemaVersion = "1"

// RunMigrations は compositor.db のスキーマを冪等に作成する。
// CREATE IF NOT EXISTS と pragma_table_info チェックを使い、
// 既存DBが壊れないようにする。
func RunMigrations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS config (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
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
		`CREATE TABLE IF NOT EXISTS published_table (
			id                 TEXT PRIMARY KEY,
			slug               TEXT NOT NULL UNIQUE,
			display_name       TEXT NOT NULL,
			symbol             TEXT NOT NULL DEFAULT '',
			source_table_id    TEXT NOT NULL REFERENCES source_table(id) ON DELETE CASCADE,
			owned_only         INTEGER NOT NULL DEFAULT 0,
			pick_per_level     INTEGER NOT NULL DEFAULT 0,
			pick_refresh_mode  TEXT NOT NULL DEFAULT 'manual'
			                   CHECK(pick_refresh_mode IN ('per_request', 'daily', 'manual')),
			prefer_old_play    INTEGER NOT NULL DEFAULT 0,
			sort_order         INTEGER NOT NULL DEFAULT 0,
			created_at         TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration exec: %w", err)
		}
	}

	// schema_version を config に書き込む（INSERT OR REPLACE で冪等）
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO config(key, value) VALUES('schema_version', ?)`,
		schemaVersion,
	); err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}

	return nil
}
```

- [ ] **Step 5: テストパス確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: `ok  	github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence`

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/persistence/migrations.go internal/adapter/persistence/migrations_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat: SQLite マイグレーション基盤を追加

config / source_table / source_table_chart / published_table の4テーブルと
2インデックスを冪等に作成する RunMigrations を実装。
schema_version='1' を config テーブルに記録。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: ConfigStore の port インタフェースと SQL 実装

**Files:**

- Create: `internal/port/config_store.go`
- Create: `internal/adapter/persistence/config_store.go`
- Create: `internal/adapter/persistence/config_store_test.go`

- [ ] **Step 1: port インタフェース定義**

`internal/port/config_store.go`:
```go
package port

import "context"

// ConfigStore は config テーブルへの key-value ストアアクセスを提供する。
type ConfigStore interface {
	// Get は指定キーの値を返す。存在しない場合 found=false を返す。
	Get(ctx context.Context, key string) (value string, found bool, err error)
	// Set は指定キーに値を保存する。既存キーは上書きされる。
	Set(ctx context.Context, key string, value string) error
}
```

- [ ] **Step 2: 失敗するテストを書く**

`internal/adapter/persistence/config_store_test.go`:
```go
package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupConfigStore(t *testing.T) (*ConfigStoreSQL, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, RunMigrations(db))
	return NewConfigStoreSQL(db), func() { db.Close() }
}

func TestConfigStoreSQL_Get_NotFound(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()

	_, found, err := store.Get(context.Background(), "missing_key")
	require.NoError(t, err)
	require.False(t, found)
}

func TestConfigStoreSQL_SetThenGet_RoundTrip(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()

	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))

	v, found, err := store.Get(context.Background(), "server_port")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "50000", v)
}

func TestConfigStoreSQL_Set_Overwrites(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "server_port", "50000"))
	require.NoError(t, store.Set(ctx, "server_port", "51234"))

	v, _, err := store.Get(ctx, "server_port")
	require.NoError(t, err)
	require.Equal(t, "51234", v)
}
```

- [ ] **Step 3: テスト失敗確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: コンパイルエラー (`ConfigStoreSQL` 未定義)。

- [ ] **Step 4: 実装を書く**

`internal/adapter/persistence/config_store.go`:
```go
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ConfigStoreSQL は config テーブルを使った port.ConfigStore の実装。
type ConfigStoreSQL struct {
	db *sql.DB
}

// NewConfigStoreSQL は新しい ConfigStoreSQL を作る。
func NewConfigStoreSQL(db *sql.DB) *ConfigStoreSQL {
	return &ConfigStoreSQL{db: db}
}

// Get は指定キーの値を返す。
func (s *ConfigStoreSQL) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("config get %q: %w", key, err)
	}
	return v, true, nil
}

// Set は指定キーに値を保存する。既存キーは上書き。
func (s *ConfigStoreSQL) Set(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("config set %q: %w", key, err)
	}
	return nil
}
```

- [ ] **Step 5: テストパス確認**

Run:
```bash
go test ./internal/adapter/persistence/...
```

Expected: 全テスト pass。

- [ ] **Step 6: コミット**

```bash
git add internal/port/config_store.go internal/adapter/persistence/config_store.go internal/adapter/persistence/config_store_test.go
git commit -m "$(cat <<'EOF'
feat: ConfigStore port と SQL 実装を追加

key-value ストアとして config テーブルへの Get/Set を提供。
ON CONFLICT DO UPDATE で冪等な上書きを実現。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: ConfigUseCase（既知キー定数とバリデーション付きファサード）

**Files:**

- Create: `internal/usecase/config_usecase.go`
- Create: `internal/usecase/config_usecase_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/config_usecase_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakeConfigStore は port.ConfigStore のテスト用実装。
type fakeConfigStore struct {
	data map[string]string
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{data: map[string]string{}}
}

func (f *fakeConfigStore) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeConfigStore) Set(_ context.Context, key, value string) error {
	f.data[key] = value
	return nil
}

func TestConfigUseCase_GetServerPort_DefaultsTo50000(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	port, err := uc.GetServerPort(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50000, port)
}

func TestConfigUseCase_SetThenGetServerPort(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	require.NoError(t, uc.SetServerPort(context.Background(), 51234))
	port, err := uc.GetServerPort(context.Background())
	require.NoError(t, err)
	require.Equal(t, 51234, port)
}

func TestConfigUseCase_SetServerPort_RejectsOutOfRange(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())

	require.Error(t, uc.SetServerPort(context.Background(), 0))
	require.Error(t, uc.SetServerPort(context.Background(), 65536))
	require.Error(t, uc.SetServerPort(context.Background(), -1))
}

func TestConfigUseCase_GetSongdataDBPath_DefaultsToEmpty(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	p, err := uc.GetSongdataDBPath(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", p)
}

func TestConfigUseCase_SetThenGetSongdataDBPath(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	require.NoError(t, uc.SetSongdataDBPath(context.Background(), "/tmp/songdata.db"))
	p, err := uc.GetSongdataDBPath(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/tmp/songdata.db", p)
}
```

- [ ] **Step 2: テスト失敗確認**

Run:
```bash
go test ./internal/usecase/...
```

Expected: コンパイルエラー。

- [ ] **Step 3: 実装を書く**

`internal/usecase/config_usecase.go`:
```go
package usecase

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// 既知の config キー
const (
	keyServerPort      = "server_port"
	keySongdataDBPath  = "songdata_db_path"
	defaultServerPort  = 50000
)

// ConfigUseCase は config の Get/Set を型安全にラップする。
type ConfigUseCase struct {
	store port.ConfigStore
}

// NewConfigUseCase は新しい ConfigUseCase を作る。
func NewConfigUseCase(store port.ConfigStore) *ConfigUseCase {
	return &ConfigUseCase{store: store}
}

// GetServerPort は HTTP サーバのポート番号を返す。未設定時は defaultServerPort。
func (u *ConfigUseCase) GetServerPort(ctx context.Context) (int, error) {
	v, found, err := u.store.Get(ctx, keyServerPort)
	if err != nil {
		return 0, err
	}
	if !found || v == "" {
		return defaultServerPort, nil
	}
	port, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse server_port %q: %w", v, err)
	}
	return port, nil
}

// SetServerPort は HTTP サーバのポート番号を保存する。範囲は 1〜65535。
func (u *ConfigUseCase) SetServerPort(ctx context.Context, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("ポート番号は 1〜65535 の範囲で指定してください (got %d)", port)
	}
	return u.store.Set(ctx, keyServerPort, strconv.Itoa(port))
}

// GetSongdataDBPath は beatoraja の songdata.db のパスを返す。未設定時は空文字列。
func (u *ConfigUseCase) GetSongdataDBPath(ctx context.Context) (string, error) {
	v, _, err := u.store.Get(ctx, keySongdataDBPath)
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSongdataDBPath は songdata.db のパスを保存する（バリデーションは行わない）。
func (u *ConfigUseCase) SetSongdataDBPath(ctx context.Context, path string) error {
	return u.store.Set(ctx, keySongdataDBPath, path)
}
```

- [ ] **Step 4: テストパス確認**

Run:
```bash
go test ./internal/usecase/...
```

Expected: 全テスト pass。

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/config_usecase.go internal/usecase/config_usecase_test.go
git commit -m "$(cat <<'EOF'
feat: ConfigUseCase で既知キーを型安全にラップ

server_port (1〜65535 バリデーション + デフォルト50000) と
songdata_db_path の Get/Set を提供。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Logger（slog + lumberjack 日次ローテ）

**Files:**

- Create: `internal/adapter/logger/logger.go`
- Create: `internal/adapter/logger/logger_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/logger/logger_test.go`:
```go
package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	logger, closeFn, err := New(Options{
		LogDir:     dir,
		MaxSizeMB:  10,
		MaxBackups: 7,
		MaxAgeDays: 7,
	})
	require.NoError(t, err)

	logger.Info("hello", "key", "value")

	// close で flush される
	require.NoError(t, closeFn())

	matches, err := filepath.Glob(filepath.Join(dir, "*.log"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no .log files in %s", dir)

	contents, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.True(t, strings.Contains(string(contents), "hello"), "log content: %s", contents)
	require.True(t, strings.Contains(string(contents), "key=value"), "log content: %s", contents)
}

func TestNew_CreatesLogDirIfMissing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nested", "logs")
	_, closeFn, err := New(Options{
		LogDir:     missing,
		MaxSizeMB:  10,
		MaxBackups: 1,
		MaxAgeDays: 1,
	})
	require.NoError(t, err)
	defer closeFn()

	info, err := os.Stat(missing)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
```

- [ ] **Step 2: 依存追加**

Run:
```bash
go get gopkg.in/natefinch/lumberjack.v2@latest
go mod tidy
```

- [ ] **Step 3: テスト失敗確認**

Run:
```bash
go test ./internal/adapter/logger/...
```

Expected: コンパイルエラー (`New`, `Options` 未定義)。

- [ ] **Step 4: 実装を書く**

`internal/adapter/logger/logger.go`:
```go
// Package logger は slog + lumberjack による日次ローテーションログを提供する。
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Options は Logger の設定を表す。
type Options struct {
	// LogDir はログファイルを格納するディレクトリ。なければ作成する。
	LogDir string
	// MaxSizeMB は1ファイルの最大サイズ。超過時に新ファイルへローテ。
	MaxSizeMB int
	// MaxBackups はローテ後に保持する旧ファイル数。
	MaxBackups int
	// MaxAgeDays は旧ファイルを保持する最大日数。
	MaxAgeDays int
}

// CloseFunc は Logger 関連リソースを開放するクロージャ。
type CloseFunc func() error

// New は Options に基づき *slog.Logger を返す。
// ログ出力は LogDir/<YYYY-MM-DD>.log に書かれ、stderr にもミラーされる。
func New(opts Options) (*slog.Logger, CloseFunc, error) {
	if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
		return nil, noopClose, fmt.Errorf("mkdir log dir: %w", err)
	}
	filename := filepath.Join(opts.LogDir, time.Now().Format("2006-01-02")+".log")
	rotator := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   false,
	}

	writer := io.MultiWriter(rotator, os.Stderr)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	return logger, rotator.Close, nil
}

func noopClose() error { return nil }
```

- [ ] **Step 5: テストパス確認**

Run:
```bash
go test ./internal/adapter/logger/...
```

Expected: 全テスト pass。

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/logger/ go.mod go.sum
git commit -m "$(cat <<'EOF'
feat: slog + lumberjack による日次ローテログを追加

実行ファイル隣の logs/YYYY-MM-DD.log にテキスト形式で出力。
stderr にもミラーする。サイズ・期間・世代でローテ可能。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: シングルインスタンスロック

**Files:**

- Create: `internal/adapter/singleinstance/lock.go`
- Create: `internal/adapter/singleinstance/lock_unix.go`
- Create: `internal/adapter/singleinstance/lock_windows.go`
- Create: `internal/adapter/singleinstance/lock_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/singleinstance/lock_test.go`:
```go
package singleinstance

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAcquire_Succeeds_WhenLockFree(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	lock, err := Acquire(lockPath)
	require.NoError(t, err)
	defer lock.Release()
	require.NotNil(t, lock)
}

func TestAcquire_Fails_WhenAlreadyHeld(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	first, err := Acquire(lockPath)
	require.NoError(t, err)
	defer first.Release()

	_, err = Acquire(lockPath)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAlreadyRunning)
}

func TestRelease_AllowsReacquire(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	first, err := Acquire(lockPath)
	require.NoError(t, err)
	require.NoError(t, first.Release())

	second, err := Acquire(lockPath)
	require.NoError(t, err)
	require.NoError(t, second.Release())
}
```

- [ ] **Step 2: テスト失敗確認**

Run:
```bash
go test ./internal/adapter/singleinstance/...
```

Expected: コンパイルエラー。

- [ ] **Step 3: 共通インタフェース部分を書く**

`internal/adapter/singleinstance/lock.go`:
```go
// Package singleinstance はファイルロックによるシングルインスタンス保証を提供する。
//
// 動作: 起動時に Acquire(path) を呼び、ロックファイルへ排他ロックを取る。
// 既にロックされていれば ErrAlreadyRunning を返す。
package singleinstance

import "errors"

// ErrAlreadyRunning は別プロセスが既にロックを保持している場合に返される。
var ErrAlreadyRunning = errors.New("別のインスタンスが既に実行中です")

// Lock はロック取得状態を表すハンドル。Release で解放する。
type Lock interface {
	Release() error
}
```

- [ ] **Step 4: Unix 実装を書く（macOS / Linux）**

`internal/adapter/singleinstance/lock_unix.go`:
```go
//go:build !windows

package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixLock struct {
	file *os.File
}

func (l *unixLock) Release() error {
	if l.file == nil {
		return nil
	}
	defer func() {
		_ = l.file.Close()
		l.file = nil
	}()
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("flock unlock: %w", err)
	}
	return nil
}

// Acquire は指定パスのファイルへ排他ロックを取得する。
// 既に他プロセスがロックしていれば ErrAlreadyRunning を返す。
func Acquire(path string) (Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("flock: %w", err)
	}

	return &unixLock{file: file}, nil
}
```

- [ ] **Step 5: Windows 実装を書く**

`internal/adapter/singleinstance/lock_windows.go`:
```go
//go:build windows

package singleinstance

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type windowsLock struct {
	handle windows.Handle
}

func (l *windowsLock) Release() error {
	if l.handle == 0 {
		return nil
	}
	defer func() {
		_ = windows.CloseHandle(l.handle)
		l.handle = 0
	}()
	// LockFileEx で取得したロックは CloseHandle で解放される
	return nil
}

// Acquire は指定パスのファイルへ排他ロックを取得する。
func Acquire(path string) (Lock, error) {
	// 親ディレクトリは存在する想定（実行ファイル隣）。なければ作成。
	if dir := pathDir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	utf16Path, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("utf16 path: %w", err)
	}

	handle, err := windows.CreateFile(
		utf16Path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ, // 他プロセスから読みは許可、書きは拒否
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("create lock file: %w", err)
	}

	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, overlapped,
	); err != nil {
		_ = windows.CloseHandle(handle)
		// FAIL_IMMEDIATELY 指定時、既にロックされていれば ERROR_LOCK_VIOLATION
		return nil, ErrAlreadyRunning
	}

	return &windowsLock{handle: handle}, nil
}

func pathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return ""
}
```

- [ ] **Step 6: 依存追加**

Run:
```bash
go get golang.org/x/sys/windows@latest
go mod tidy
```

注: `golang.org/x/sys` は Wails が間接依存しているはずだが、明示する。

- [ ] **Step 7: macOS でテストパス確認（Unix 実装のみテストされる）**

Run:
```bash
go test ./internal/adapter/singleinstance/...
```

Expected: 全テスト pass。

注: Windows 実装のテストは Windows GHA 上の `make test` で確認される。

- [ ] **Step 8: コミット**

```bash
git add internal/adapter/singleinstance/ go.mod go.sum
git commit -m "$(cat <<'EOF'
feat: シングルインスタンスロックを追加

OS別実装 (Unix:flock / Windows:LockFileEx) でロックファイルへ
排他ロックを取り、二重起動を防ぐ。Acquire/Release のシンプルAPI。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: ConfigHandler（Wails Bind ターゲット）

**Files:**

- Create: `internal/app/handler/config_handler.go`
- Create: `internal/app/handler/config_handler_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/handler/config_handler_test.go`:
```go
package handler_test

import (
	"context"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	data map[string]string
}

func (f *fakeStore) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := f.data[k]
	return v, ok, nil
}
func (f *fakeStore) Set(_ context.Context, k, v string) error {
	f.data[k] = v
	return nil
}

func newHandler() *handler.ConfigHandler {
	uc := usecase.NewConfigUseCase(&fakeStore{data: map[string]string{}})
	return handler.NewConfigHandler(uc)
}

func TestConfigHandler_GetServerConfig_DefaultValues(t *testing.T) {
	h := newHandler()
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, 50000, cfg.Port)
	require.Equal(t, "", cfg.SongdataDBPath)
}

func TestConfigHandler_SetServerPort_Persists(t *testing.T) {
	h := newHandler()
	require.NoError(t, h.SetServerPort(51234))
	cfg, _ := h.GetServerConfig()
	require.Equal(t, 51234, cfg.Port)
}

func TestConfigHandler_SetServerPort_RejectsOutOfRange(t *testing.T) {
	h := newHandler()
	require.Error(t, h.SetServerPort(0))
	require.Error(t, h.SetServerPort(70000))
}

func TestConfigHandler_SetSongdataDBPath_Persists(t *testing.T) {
	h := newHandler()
	require.NoError(t, h.SetSongdataDBPath("/tmp/songdata.db"))
	cfg, _ := h.GetServerConfig()
	require.Equal(t, "/tmp/songdata.db", cfg.SongdataDBPath)
}
```

- [ ] **Step 2: テスト失敗確認**

Run:
```bash
go test ./internal/app/handler/...
```

Expected: コンパイルエラー。

- [ ] **Step 3: 実装を書く**

`internal/app/handler/config_handler.go`:
```go
package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// ServerConfig は GetServerConfig が返す JSON 構造体。
type ServerConfig struct {
	Port           int    `json:"port"`
	SongdataDBPath string `json:"songdataDbPath"`
}

// ConfigHandler は Wails Bind 経由でフロントエンドから呼ばれる設定ハンドラ。
type ConfigHandler struct {
	uc  *usecase.ConfigUseCase
	ctx context.Context
}

// NewConfigHandler は ConfigHandler を作る。
func NewConfigHandler(uc *usecase.ConfigUseCase) *ConfigHandler {
	return &ConfigHandler{uc: uc, ctx: context.Background()}
}

// SetContext は Wails の OnStartup で受け取る context を保存する。
func (h *ConfigHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// GetServerConfig は現在の設定値（ポート / songdata.db パス）を返す。
func (h *ConfigHandler) GetServerConfig() (ServerConfig, error) {
	port, err := h.uc.GetServerPort(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	dbPath, err := h.uc.GetSongdataDBPath(h.ctx)
	if err != nil {
		return ServerConfig{}, err
	}
	return ServerConfig{Port: port, SongdataDBPath: dbPath}, nil
}

// SetServerPort はサーバポート番号を保存する。範囲外はエラー。
func (h *ConfigHandler) SetServerPort(port int) error {
	return h.uc.SetServerPort(h.ctx, port)
}

// SetSongdataDBPath は beatoraja の songdata.db パスを保存する。
func (h *ConfigHandler) SetSongdataDBPath(path string) error {
	return h.uc.SetSongdataDBPath(h.ctx, path)
}
```

- [ ] **Step 4: テストパス確認**

Run:
```bash
go test ./...
```

Expected: 全テスト pass。

- [ ] **Step 5: コミット**

```bash
git add internal/app/handler/
git commit -m "$(cat <<'EOF'
feat: ConfigHandler で Wails Bind 用の設定 API を提供

GetServerConfig / SetServerPort / SetSongdataDBPath をエクスポート。
ConfigUseCase をラップして Wails 経由で呼び出せる形にする。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: システムトレイ常駐

**Files:**

- Create: `internal/adapter/tray/tray.go`
- Create: `internal/adapter/tray/icons.go`

注: Plan 1 段階ではアイコンビット列を仮実装で持つ。Plan 4 で実アイコンに差し替え。

- [ ] **Step 1: 依存追加**

Run:
```bash
go get github.com/getlantern/systray@latest
go mod tidy
```

- [ ] **Step 2: 仮アイコンを生成（簡易：単色のPNGバイト列）**

`internal/adapter/tray/icons.go`:
```go
package tray

// 16x16 単色 PNG のハードコード。本実装は Plan 4 で実アイコンに差し替え。
// 生成手順: macOS なら以下の Python ワンライナー等で作る:
//   python -c "import struct; ..." (省略)
// 一時的にライブラリ提供のサンプルを参考に Plan 1 では空(=色なし)で構わない。
//
// systray.SetIcon(nil) はパニックするため、常に何らかのバイト列が必要。
// 簡易な Stub として「空PNG」（透明1x1 PNG）を埋め込む。

// stubIconBytes は 1x1 透明 PNG。systray が要求する有効PNG/ICOバイト列を満たす。
var stubIconBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// IconFor は与えられた状態に対応する Stub アイコンを返す。
// Plan 4 で状態別の実アイコンに差し替える。
func IconFor(state State) []byte {
	// Plan 1 では全状態で同じ stub を返す（差し替えポイントを明示）
	return stubIconBytes
}
```

- [ ] **Step 3: トレイ実装を書く**

`internal/adapter/tray/tray.go`:
```go
// Package tray は getlantern/systray を使ったシステムトレイ常駐機能を提供する。
package tray

import (
	"sync"

	"github.com/getlantern/systray"
)

// State はトレイアイコン色の切替対象となる状態。
type State int

const (
	// StateIdle はサーバ未起動 (Plan 1 の初期状態)。
	StateIdle State = iota
	// StateRunning はサーバが正常稼働中 (Plan 3 以降で使用)。
	StateRunning
	// StateError はサーバ起動失敗等のエラー状態。
	StateError
)

// Callbacks は GUI 側へイベントを通知するコールバック群。
type Callbacks struct {
	OnShowSettings func()
	OnQuit         func()
}

// Tray はシステムトレイを管理する。Run で起動、SetState で状態変更、Quit で停止。
type Tray struct {
	cb        Callbacks
	mu        sync.Mutex
	state     State
	mShow     *systray.MenuItem
	mQuit     *systray.MenuItem
	tooltip   string
}

// New は Tray を作る。Run で実際に起動する。
func New(cb Callbacks) *Tray {
	return &Tray{
		cb:      cb,
		state:   StateIdle,
		tooltip: "BMS Random Table Compositor",
	}
}

// Run は systray のメインループを開始する。**呼び出しスレッドはメインスレッドで実行する必要がある**。
// onReady はトレイがUIに登録された後で呼ばれる。
func (t *Tray) Run(onReady func()) {
	systray.Run(func() {
		systray.SetTooltip(t.tooltip)
		systray.SetIcon(IconFor(t.state))

		t.mShow = systray.AddMenuItem("設定を開く", "メインウィンドウを表示")
		systray.AddSeparator()
		t.mQuit = systray.AddMenuItem("終了", "アプリケーションを終了")

		go t.handleClicks()

		if onReady != nil {
			onReady()
		}
	}, func() {
		// onExit
	})
}

func (t *Tray) handleClicks() {
	for {
		select {
		case <-t.mShow.ClickedCh:
			if t.cb.OnShowSettings != nil {
				t.cb.OnShowSettings()
			}
		case <-t.mQuit.ClickedCh:
			if t.cb.OnQuit != nil {
				t.cb.OnQuit()
			}
			systray.Quit()
			return
		}
	}
}

// SetState はトレイアイコンの状態を切り替える。
func (t *Tray) SetState(s State) {
	t.mu.Lock()
	t.state = s
	t.mu.Unlock()
	systray.SetIcon(IconFor(s))
}

// Quit はトレイメインループを停止する。
func (t *Tray) Quit() {
	systray.Quit()
}
```

- [ ] **Step 4: ビルド確認**

Run:
```bash
go build ./...
```

Expected: コンパイルエラーなし。

注: systray は OS 依存リソースを使うため、ユニットテストは作らない（`go test ./internal/adapter/tray/` を実行しても、`go test` は動かない）。実 GUI 動作は Task 13 以降の統合で確認。

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/tray/ go.mod go.sum
git commit -m "$(cat <<'EOF'
feat: getlantern/systray でトレイ常駐機能を追加

Tray.Run でメインループを起動、SetState でアイコン状態切替。
メニューは「設定を開く」「終了」の2項目。アイコンは Plan 1 段階で
1x1 透明 PNG の stub。Plan 4 で実アイコンに差し替え予定。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Wails ライフサイクル統合（main.go / app.go の書き換え）

**Files:**

- Modify: `main.go`
- Modify: `app.go`
- Create: `internal/app/bootstrap.go`

このタスクは GUI 起動の統合点で、各コンポーネントを配線する。

- [ ] **Step 1: `internal/app/bootstrap.go` を作成（依存配線の集約）**

```go
// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/singleinstance"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// Services はアプリ全体で共有する依存を保持する。
type Services struct {
	DB             *sql.DB
	Logger         *slog.Logger
	LoggerClose    logger.CloseFunc
	Lock           singleinstance.Lock
	ConfigHandler  *handler.ConfigHandler
	Tray           *tray.Tray
}

// Bootstrap は Services を構築する（DB接続・マイグレーション・ロック取得・ロガー初期化）。
// 失敗時は途中で取得済みのリソースを開放してエラーを返す。
func Bootstrap() (*Services, error) {
	// 1. ログディレクトリとロガー
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

	// 2. シングルインスタンスロック
	lockPath, err := paths.LockPath()
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("lock path: %w", err)
	}
	lock, err := singleinstance.Acquire(lockPath)
	if err != nil {
		_ = closeLog()
		return nil, err // ErrAlreadyRunning も含む
	}

	// 3. DB と マイグレーション
	dbPath, err := paths.DBPath()
	if err != nil {
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("db path: %w", err)
	}
	db, err := persistence.OpenDB(dbPath)
	if err != nil {
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := persistence.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	// 4. ハンドラ配線
	configStore := persistence.NewConfigStoreSQL(db)
	configUC := usecase.NewConfigUseCase(configStore)
	configHandler := handler.NewConfigHandler(configUC)

	lg.Info("bootstrap complete", "db", dbPath, "logDir", logDir)

	return &Services{
		DB:            db,
		Logger:        lg,
		LoggerClose:   closeLog,
		Lock:          lock,
		ConfigHandler: configHandler,
	}, nil
}

// Close は Services が保持する全リソースを開放する。
func (s *Services) Close() {
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.Lock != nil {
		_ = s.Lock.Release()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
```

- [ ] **Step 2: `app.go` を書き換える**

既存の `app.go`（wails initテンプレ）を以下に**完全置き換え**:

```go
package main

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	"github.com/meta-BE/bms-random-table-compositor/internal/app"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App は Wails のメインアプリオブジェクト。
type App struct {
	ctx      context.Context
	services *app.Services
	tray     *tray.Tray
}

// NewApp は services を保持した App を作る。
// services は Bootstrap で構築済みのものを渡す。
func NewApp(services *app.Services) *App {
	return &App{services: services}
}

// startup は OnStartup で呼ばれる。ctx 保持と ConfigHandler への ctx 配布。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.services.ConfigHandler.SetContext(ctx)
	a.services.Logger.Info("wails startup")
}

// onBeforeClose はウィンドウクローズ前に呼ばれる。
// true を返すとクローズが取り消される。本実装ではトレイ格納のため
// 自前で WindowHide してから true を返す。
func (a *App) onBeforeClose(ctx context.Context) bool {
	wailsruntime.WindowHide(ctx)
	return true
}

// shutdown はアプリ完全終了時に呼ばれる。
func (a *App) shutdown(ctx context.Context) {
	a.services.Logger.Info("wails shutdown")
}

// SetTray はトレイインスタンスを保持する（main から渡される）。
func (a *App) SetTray(t *tray.Tray) {
	a.tray = t
}

// ShowWindow はトレイメニューから呼ばれ、ウィンドウを再表示する。
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
	}
}

// Quit はトレイメニュー「終了」から呼ばれる。Wails ウィンドウを終了させる。
func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}
```

- [ ] **Step 3: `main.go` を書き換える**

```go
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/singleinstance"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
	appinternal "github.com/meta-BE/bms-random-table-compositor/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	services, err := appinternal.Bootstrap()
	if err != nil {
		if err == singleinstance.ErrAlreadyRunning {
			fmt.Fprintln(os.Stderr, "別のインスタンスが既に実行中です。設定を開きたい場合はトレイメニューから操作してください。")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "起動エラー: %v\n", err)
		os.Exit(1)
	}
	defer services.Close()

	myApp := NewApp(services)

	tr := tray.New(tray.Callbacks{
		OnShowSettings: myApp.ShowWindow,
		OnQuit:         myApp.Quit,
	})
	myApp.SetTray(tr)

	// systray.Run はブロッキングで、メインスレッドを占有する。
	// goroutine で起動し、ready 後に wails.Run に進む方式は systray の
	// メインスレッド要件と衝突する可能性がある。Plan 1 では POC で得た
	// 「Wails と systray の同居」方針に従い、systray を別 goroutine で起動する。
	// systray ライブラリは Run を main goroutine から呼ぶ前提だが、
	// Wails が main goroutine を使うので、systray を goroutine から起動する。
	// ※ Phase 1 で実機検証して問題があれば Plan 1 内で再設計する。
	go tr.Run(nil)

	if err := wails.Run(&options.App{
		Title:  "BMS Random Table Compositor",
		Width:  900,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     myApp.startup,
		OnBeforeClose: myApp.onBeforeClose,
		OnShutdown:    myApp.shutdown,
		Bind: []any{
			myApp,
			services.ConfigHandler,
		},
	}); err != nil {
		services.Logger.Error("wails run failed", "err", err)
		fmt.Fprintf(os.Stderr, "Wails Error: %v\n", err)
		os.Exit(1)
	}

	tr.Quit()
}
```

- [ ] **Step 4: ビルドが通ることを確認**

Run:
```bash
go build ./...
wails build
```

Expected: 成功。

- [ ] **Step 5: コミット**

```bash
git add main.go app.go internal/app/bootstrap.go
git commit -m "$(cat <<'EOF'
feat: Wails ライフサイクルに DB/ロック/ログ/トレイを統合

Bootstrap で全依存を構築し、main.go から OnStartup/OnBeforeClose/
OnShutdown フックと Bind を配線。ウィンドウクローズはトレイ格納に
変換、トレイ「終了」で Wails Quit を呼ぶ。
ErrAlreadyRunning 時は標準エラーへメッセージを出して終了する。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: フロントエンド設定タブ（最小実装）

**Files:**

- Create: `frontend/src/lib/api.ts`
- Create: `frontend/src/lib/tabs/ServerTab.svelte`
- Modify: `frontend/src/App.svelte`
- Modify: `frontend/src/main.ts`（必要に応じて）
- Modify: `frontend/src/style.css`

- [ ] **Step 1: api.ts ラッパを作成**

`frontend/src/lib/api.ts`:
```ts
// Wails Bind のラッパ。生成型の細かい変動を吸収するために薄く包む。
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
} from '../../wailsjs/go/handler/ConfigHandler';

export type ServerConfig = {
  port: number;
  songdataDbPath: string;
};

export const api = {
  getServerConfig(): Promise<ServerConfig> {
    return GetServerConfig() as Promise<ServerConfig>;
  },
  setServerPort(port: number): Promise<void> {
    return SetServerPort(port);
  },
  setSongdataDBPath(path: string): Promise<void> {
    return SetSongdataDBPath(path);
  },
};
```

- [ ] **Step 2: ServerTab.svelte を作成**

`frontend/src/lib/tabs/ServerTab.svelte`:
```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type ServerConfig } from '../api';

  let port: number = 50000;
  let songdataDbPath: string = '';
  let originalPort: number = 50000;
  let originalPath: string = '';
  let saving: boolean = false;
  let message: string = '';
  let error: string = '';

  async function load() {
    try {
      const cfg: ServerConfig = await api.getServerConfig();
      port = cfg.port;
      songdataDbPath = cfg.songdataDbPath;
      originalPort = cfg.port;
      originalPath = cfg.songdataDbPath;
    } catch (e: any) {
      error = `読み込みエラー: ${String(e)}`;
    }
  }

  async function save() {
    saving = true;
    error = '';
    message = '';
    try {
      if (port !== originalPort) {
        await api.setServerPort(port);
        originalPort = port;
      }
      if (songdataDbPath !== originalPath) {
        await api.setSongdataDBPath(songdataDbPath);
        originalPath = songdataDbPath;
      }
      message = '保存しました';
    } catch (e: any) {
      error = String(e);
    } finally {
      saving = false;
    }
  }

  $: dirty = port !== originalPort || songdataDbPath !== originalPath;

  onMount(load);
</script>

<section class="tab">
  <h2>サーバ設定</h2>

  <label class="row">
    <span class="label">HTTPサーバ ポート番号</span>
    <input type="number" min="1" max="65535" bind:value={port} disabled={saving} />
  </label>

  <label class="row">
    <span class="label">beatoraja の songdata.db パス</span>
    <input type="text" bind:value={songdataDbPath} placeholder="/path/to/songdata.db" disabled={saving} />
  </label>

  <div class="actions">
    <button on:click={save} disabled={saving || !dirty}>保存</button>
  </div>

  {#if message}<p class="message ok">{message}</p>{/if}
  {#if error}<p class="message err">{error}</p>{/if}
</section>

<style>
  .tab { padding: 16px; }
  h2 { margin-top: 0; font-size: 16px; }
  .row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
  .label { font-size: 13px; color: #555; }
  input { padding: 6px 8px; font-size: 14px; }
  .actions { margin-top: 12px; }
  button { padding: 6px 14px; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: 0.6; }
  .message.ok { color: #2e7d32; }
  .message.err { color: #b71c1c; }
</style>
```

- [ ] **Step 3: App.svelte を書き換える（タブ親）**

```svelte
<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
</script>

<main>
  <header>
    <h1>BMS Random Table Compositor</h1>
  </header>
  <ServerTab />
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    color: #1b2636;
    min-height: 100vh;
  }
  header {
    padding: 16px;
    border-bottom: 1px solid #e0e0e0;
  }
  header h1 { margin: 0; font-size: 18px; }
</style>
```

- [ ] **Step 4: main.ts と style.css は最小（既存テンプレのままで OK）**

確認のみ。`main.ts` が `App.svelte` を mount してれば変更不要。`style.css` は既存テンプレで OK。

- [ ] **Step 5: フロントエンド + Wails ビルド成功確認**

Run:
```bash
cd frontend && npm install && npm run build && cd ..
wails build
```

Expected: 成功。`build/bin/bms-random-table-compositor.app` が生成される。

- [ ] **Step 6: コミット**

```bash
git add frontend/
git commit -m "$(cat <<'EOF'
feat: 設定タブ (ポート + songdata.db パス) の最小UI を実装

ServerTab.svelte に2フィールドの保存フォーム。Wails Bind ラッパは
frontend/src/lib/api.ts に集約。Plan 4 でデザイン整備予定。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: macOS 動作確認

このタスクはコード変更を伴わない手動確認タスク。

- [ ] **Step 1: `wails dev` で起動確認**

Run:
```bash
wails dev
```

確認:
- [ ] ウィンドウが開き、設定タブのフォームが表示される
- [ ] ポート番号と songdata.db パスを入力 → 保存 → 「保存しました」メッセージ
- [ ] アプリを終了 → 再度 `wails dev` → 値が保持されている
- [ ] `compositor.db` が `os.Executable()` の戻り値の隣（一時バイナリ隣）に作成される
- [ ] `logs/YYYY-MM-DD.log` にログが書き込まれる
- [ ] ウィンドウを閉じる → トレイに格納される（Dockのアイコン状況を確認）
- [ ] トレイメニューから「設定を開く」 → ウィンドウ再表示
- [ ] 別ターミナルでもう一度 `wails dev` 起動 → 「別のインスタンスが既に実行中です」と出て即終了
- [ ] トレイメニュー「終了」 → アプリ終了

注: `wails dev` 中は `os.Executable()` が一時バイナリを返すため、`compositor.db` も一時パスに作られる。これは想定挙動（POC でも同様）。

- [ ] **Step 2: `wails build` 成果物で同様に確認**

Run:
```bash
wails build
open ./build/bin/bms-random-table-compositor.app
```

確認:
- [ ] Step 1 のチェックリストを再実施
- [ ] `compositor.db` が `bms-random-table-compositor.app/Contents/MacOS/` 配下に作成される
- [ ] `logs/` も同じ場所に作成される
- [ ] `.lock` ファイルも同じ場所に作成される

- [ ] **Step 3: 不具合があれば修正コミットする**

修正が必要な場合のみコミット:
```bash
git add ...
git commit -m "fix: macOS 動作確認で見つかった不具合を修正"
```

なければ次のタスクへ。

---

## Task 16: Windows ビルドワークフローを追加

**Files:**

- Create: `.github/workflows/build-windows.yml`

- [ ] **Step 1: `.github/workflows/build-windows.yml` を作成**

```yaml
name: Build Windows

on:
  workflow_dispatch:

permissions:
  contents: read

jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache-dependency-path: go.sum

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Wails CLI
        shell: pwsh
        run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0

      - name: Frontend install
        shell: pwsh
        working-directory: frontend
        run: npm install

      - name: Wails build (windows/amd64)
        shell: pwsh
        run: wails build -platform windows/amd64

      # Wails の Windows ビルドはディレクトリ名を exe 名に使う場合があるため、
      # 一律でリネームしてから artifact 化する（POC の知見）。
      - name: Rename exe (idempotent)
        shell: pwsh
        run: |
          if (Test-Path build/bin/bms-random-table-compositor.exe) {
            Write-Host "exe already named correctly"
          } elseif (Test-Path build/bin/bms-random-table-compositor-windows-amd64.exe) {
            Rename-Item -Path build/bin/bms-random-table-compositor-windows-amd64.exe -NewName bms-random-table-compositor.exe
          } else {
            $alt = Get-ChildItem -Path build/bin/*.exe | Select-Object -First 1
            if ($alt) {
              Rename-Item -Path $alt.FullName -NewName bms-random-table-compositor.exe
            } else {
              Write-Error "No exe found in build/bin"
              exit 1
            }
          }

      - name: Upload exe artifact
        uses: actions/upload-artifact@v4
        with:
          name: bms-random-table-compositor-windows-amd64
          path: build/bin/bms-random-table-compositor.exe
          if-no-files-found: error
          retention-days: 14
```

- [ ] **Step 2: コミット**

```bash
git add .github/workflows/build-windows.yml
git commit -m "$(cat <<'EOF'
ci: 本体 Windows exe を workflow_dispatch で生成する Actions を追加

POC の poc-build-windows.yml は別物として保持。本体は go.sum を
リポジトリルートから読み、cache-dependency-path で setup-go の
キャッシュ復元も有効化。Rename ステップは exe 名揺れに頑健な
冪等実装。

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: push**

```bash
git push origin main
```

注: ユーザー指定で main ブランチで開発しているため、cherry-pick は不要。直接 main から `gh workflow run` できる。

---

## Task 17: Windows ビルドを gh CLI で発火・動作確認

このタスクは Windows 実機/VM での手動確認を含む。

- [ ] **Step 1: ワークフロー発火**

Run:
```bash
gh workflow run build-windows.yml --ref main
sleep 3
gh run list --workflow build-windows.yml --branch main --limit 1 --json databaseId,status
```

実行 ID をメモする（以下 `<run-id>`）。

- [ ] **Step 2: 実行を監視**

Run:
```bash
gh run watch <run-id> --exit-status
```

Expected: 全ステップ ✓ で完了。失敗時は `gh run view <run-id> --log-failed` で原因確認、ワークフロー修正、Task 16 から再実行。

- [ ] **Step 3: Artifact 取得**

Run:
```bash
mkdir -p tmp/main-windows
gh run download <run-id> --name bms-random-table-compositor-windows-amd64 --dir tmp/main-windows
ls tmp/main-windows/
```

Expected: `bms-random-table-compositor.exe` が存在する。

- [ ] **Step 4: Windows 実機/VM で動作確認**

`tmp/main-windows/bms-random-table-compositor.exe` を Windows 機/VM に転送し起動。

確認:
- [ ] アプリウィンドウが開く
- [ ] 設定タブが表示される
- [ ] ポート番号と songdata.db パスを保存できる
- [ ] アプリを再起動 → 値が保持されている
- [ ] `bms-random-table-compositor.exe` の隣に `compositor.db`, `logs/`, `.lock` が作られる
- [ ] ウィンドウクローズ → タスクトレイに格納される
- [ ] トレイメニュー「設定を開く」 → ウィンドウ再表示
- [ ] 二重起動 → 即終了
- [ ] トレイメニュー「終了」 → アプリ終了

- [ ] **Step 5: 結果レポートと次のステップ**

Phase 1 / Plan 1 の完了基準（Plan 冒頭参照）をすべて満たしているか最終確認:

- [ ] リポジトリルートに本体 Wails アプリ展開完了
- [ ] `compositor.db` の4テーブル生成
- [ ] ポート番号 / songdata.db パス保存・再起動後保持
- [ ] `logs/YYYY-MM-DD.log` 出力
- [ ] 二重起動防止
- [ ] トレイ常駐 + メニュー操作
- [ ] Windows exe ビルド + 動作確認

すべて OK なら Plan 1 完了。Plan 2（ソース表取り込み）の `writing-plans` セッションへ。

不具合があれば修正コミットを Plan 1 内で完結させる。

---

## 最終チェックリスト

Plan 1 完了の自己診断:

- [ ] `make test` が全 pass
- [ ] `make build` が成功（macOS .app 生成）
- [ ] GitHub Actions の `build-windows.yml` が成功（exe Artifact 生成）
- [ ] Windows 実機/VM で起動確認 OK
- [ ] `compositor.db` が4テーブルで初期化される
- [ ] 設定保存サイクル動作
- [ ] トレイ常駐動作
- [ ] 二重起動防止動作
- [ ] ログ出力動作
- [ ] すべての変更が main ブランチに push 済み

完了後、Plan 2（ソース表取り込み）の writing-plans セッションへ。Plan 1 の `bootstrap.go` に SourceTableRepo / Fetcher の依存を追加する形で拡張する想定。
