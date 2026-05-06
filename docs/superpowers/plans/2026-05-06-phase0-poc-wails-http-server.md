# Phase 0 POC: Wails GUI + ローカルHTTPサーバ 実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wails v2デスクトップアプリ内に、Svelte GUIから入力したポート番号を永続化し、Goのバックエンドで `net/http` サーバを goroutine 起動して固定JSONを返す最小構成を作り、macOS と GitHub Actions ビルドの Windows exe で動作させる。

**Architecture:** `poc/` 配下に独立Goモジュールとして Wails アプリを `wails init` で生成。`Config` ロード/保存（`./poc-config.json`）と `Server` 起動/停止（`net/http`）を Go 側に分離。`App` 型の Wails Bind メソッドが両者を仲介。フロントエンドは Svelte + TS で最小UI（ポート入力・状態表示・起動/停止ボタン）。Windows ビルドは `.github/workflows/poc-build-windows.yml`（`workflow_dispatch`）で生成、`gh run download` で Artifact 取得。

**Tech Stack:** Go 1.24 / Wails v2.11.0 / Svelte + TypeScript + Vite / GitHub Actions

**設計ドキュメント:** `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` のセクション 14「Phase 0: POC」

**完了条件:**

- macOS 上で `wails dev` 経由の起動・操作確認
- macOS 上で `wails build` 成果物（.app）の起動・操作確認
- Windows exe を GitHub Actions（`workflow_dispatch`）で生成し、`gh run download` で取得した Artifact を Windows 機/VM で起動確認
- ポート番号を画面で変更→保存→起動→アクセスのサイクル動作
- ポート確保失敗時に GUI にエラー表示
- `poc/NOTES.md` に Phase 1 へ持ち越す知見を記録

**スコープ外（Phase 1 で扱う）:** システムトレイ常駐、シングルインスタンスロック、SQLite、ソース表取り込み、ピック、所持判定、自動テスト

---

## ファイル構造

```
bms-random-table-compositor/
├── poc/                                    # Phase 0 で新規。完了後も残す（参照用）
│   ├── go.mod / go.sum                     # wails init で生成（独立モジュール）
│   ├── wails.json
│   ├── main.go                             # wails.Run + Bind
│   ├── app.go                              # App 構造体（Wails Bindターゲット）
│   ├── config.go                           # Config 型 + Load/Save
│   ├── server.go                           # Server 型 + Start/Stop
│   ├── frontend/
│   │   ├── package.json
│   │   ├── index.html
│   │   ├── vite.config.ts
│   │   ├── tsconfig.json
│   │   ├── svelte.config.js
│   │   └── src/
│   │       ├── App.svelte                  # ポート入力 + ボタン + ステータス
│   │       ├── main.ts
│   │       └── style.css
│   ├── build/                              # wails build 成果物（gitignore）
│   ├── NOTES.md                            # Phase 1 へ引き継ぐ知見
│   └── poc-config.json                     # 永続化先（gitignore、初回起動時生成）
├── .github/
│   └── workflows/
│       └── poc-build-windows.yml           # workflow_dispatch でWindows exeをArtifact化
└── .gitignore                              # poc/build/, poc/frontend/node_modules/, poc/poc-config.json 等を追加
```

各ファイルの責務:

| ファイル | 責務 |
|---|---|
| `poc/main.go` | Wails アプリのエントリポイント、`App` のBind |
| `poc/app.go` | Wailsから呼ばれるメソッド群（GetConfig / SaveAndStart / Stop / Status）。`Config` と `Server` を保持 |
| `poc/config.go` | `Config` 型と JSON ロード/保存。永続化先パスは `os.Executable` の隣 |
| `poc/server.go` | `Server` 型。`Start(port)` で goroutine に net/http を起動、`Stop(ctx)` でグレースフル停止 |
| `poc/frontend/src/App.svelte` | UI。Wailsバインディング呼び出し |
| `.github/workflows/poc-build-windows.yml` | Windows exe ビルド+Artifact化。`workflow_dispatch` のみ |

---

## Task 1: POCディレクトリのwails initスキャフォールド

**Files:**

- Create (via wails CLI): `poc/main.go`, `poc/app.go`, `poc/go.mod`, `poc/wails.json`, `poc/frontend/...` ほか
- Modify: `.gitignore`（リポジトリルート）

- [ ] **Step 1: Wails CLI v2.11.0 がインストールされていることを確認**

Run:
```bash
wails version
```

Expected: `v2.11.0` が出力される。インストールされていなければ:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
```

- [ ] **Step 2: POC をスキャフォールド**

Run（リポジトリルートから）:
```bash
wails init -n bms-rtc-poc -t svelte-ts -d poc
```

Expected: `poc/` 配下に Wails の svelte-ts テンプレが生成される（`go.mod`, `main.go`, `app.go`, `frontend/` 等）。

- [ ] **Step 3: `.gitignore` を更新（リポジトリルート）**

リポジトリルートの `.gitignore` を以下の内容で作成（既存があれば追記）:

```gitignore
# Wails build artifacts
poc/build/bin/
poc/frontend/dist/
poc/frontend/node_modules/
poc/frontend/wailsjs/

# POC runtime artifacts
poc/poc-config.json

# OS / Editor
.DS_Store
*.swp
.idea/
.vscode/
```

- [ ] **Step 4: 初回ビルドが通ることを確認**

Run:
```bash
cd poc && wails build && cd ..
```

Expected: ビルドが成功し、`poc/build/bin/bms-rtc-poc.app`（macOS）が生成される。

- [ ] **Step 5: コミット**

```bash
git add .gitignore poc/
git commit -m "chore(poc): wails initでPOCスキャフォールドを生成"
```

---

## Task 2: Config型とJSON永続化（poc/config.go）

**Files:**

- Create: `poc/config.go`

- [ ] **Step 1: `poc/config.go` を作成**

```go
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Port int `json:"port"`
}

const defaultPort = 50000

// configPath は実行ファイル隣の poc-config.json を返す。
// wails dev 時など実行ファイルが取れない/一時的な場合はカレントディレクトリにフォールバックする。
func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "poc-config.json"
	}
	return filepath.Join(filepath.Dir(exe), "poc-config.json")
}

// LoadConfig は config を読み込む。ファイルが無ければデフォルト値を返す。
func LoadConfig() (Config, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Port: defaultPort}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	return c, nil
}

// SaveConfig は config を JSON でディスクに保存する。
func SaveConfig(c Config) error {
	path := configPath()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 2: ビルドが通ることを確認**

Run:
```bash
cd poc && go build ./... && cd ..
```

Expected: コンパイルエラーなく成功。

- [ ] **Step 3: コミット**

```bash
git add poc/config.go
git commit -m "feat(poc): Config型とJSON永続化(poc-config.json)を追加"
```

---

## Task 3: HTTPサーバ起動・停止（poc/server.go）

**Files:**

- Create: `poc/server.go`

- [ ] **Step 1: `poc/server.go` を作成**

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server はPOC用ローカルHTTPサーバ。Start/Stopでライフサイクルを制御する。
type Server struct {
	mu     sync.Mutex
	server *http.Server
	port   int
}

func NewServer() *Server {
	return &Server{}
}

// Start は指定ポートでHTTPサーバをgoroutineで起動する。
// ポート確保失敗時はerrorを返す（Listenが同期的なため、起動失敗を呼び出し元で捕捉できる）。
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return errors.New("server already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hello": "world",
			"port":  port,
		})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ポート %d の確保に失敗: %w", port, err)
	}

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.port = port

	go func() {
		_ = s.server.Serve(ln)
	}()
	return nil
}

// Stop はサーバをグレースフル停止する。未起動なら何もしない。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}
	srv := s.server
	s.server = nil
	s.port = 0
	return srv.Shutdown(ctx)
}

// Running は現在サーバが起動中かを返す。
func (s *Server) Running() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server != nil, s.port
}
```

- [ ] **Step 2: ビルドが通ることを確認**

Run:
```bash
cd poc && go build ./... && cd ..
```

Expected: コンパイルエラーなく成功。

- [ ] **Step 3: コミット**

```bash
git add poc/server.go
git commit -m "feat(poc): HTTPサーバの起動・停止ロジックを追加"
```

---

## Task 4: AppにWails Bindメソッドを実装

**Files:**

- Modify: `poc/app.go`

- [ ] **Step 1: `poc/app.go` を以下の内容で**置き換える**（wails init生成のApp構造体を流用しつつ、機能を追加）**

```go
package main

import (
	"context"
	"time"
)

// App は Wails のメインアプリオブジェクト。フロントエンドからBind経由で呼ばれる。
type App struct {
	ctx    context.Context
	server *Server
}

// NewApp は新しい App インスタンスを作る。
func NewApp() *App {
	return &App{server: NewServer()}
}

// startup は Wails の OnStartup で呼ばれる。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown は Wails の OnShutdown で呼ばれる。サーバを停止する。
func (a *App) shutdown(ctx context.Context) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = a.server.Stop(c)
}

// === フロントエンドにBindされるメソッド ===

// GetConfig は現在の設定を返す。
func (a *App) GetConfig() (Config, error) {
	return LoadConfig()
}

// Status は現在のサーバ状態を返す。
type Status struct {
	Running bool `json:"running"`
	Port    int  `json:"port"`
}

func (a *App) GetStatus() Status {
	running, port := a.server.Running()
	return Status{Running: running, Port: port}
}

// SaveAndStart は新しいポートを保存し、現在のサーバを停止してから新ポートで再起動する。
// エラー（保存失敗 or ポート確保失敗）はそのままフロントに返す。
func (a *App) SaveAndStart(port int) error {
	if port < 1 || port > 65535 {
		return errPortRange{}
	}
	if err := SaveConfig(Config{Port: port}); err != nil {
		return err
	}
	c, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	if err := a.server.Stop(c); err != nil {
		return err
	}
	return a.server.Start(port)
}

// Stop はサーバを停止する。
func (a *App) Stop() error {
	c, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	return a.server.Stop(c)
}

type errPortRange struct{}

func (errPortRange) Error() string { return "ポート番号は 1〜65535 の範囲で指定してください" }
```

- [ ] **Step 2: `poc/main.go` を以下の内容で置き換え（OnShutdownを追加、Bindは既にApp一つだけでよい）**

```go
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "bms-rtc-poc",
		Width:  640,
		Height: 360,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 3: ビルドが通ることを確認**

Run:
```bash
cd poc && wails build && cd ..
```

Expected: ビルドが成功する（`frontend/dist` が空でも `embed` が許す。失敗するなら一度 `cd poc/frontend && npm install && npm run build` を挟む）。

- [ ] **Step 4: コミット**

```bash
git add poc/app.go poc/main.go
git commit -m "feat(poc): App構造体にWails Bindメソッド(GetConfig/SaveAndStart/Stop/GetStatus)を追加"
```

---

## Task 5: Svelte UI（ポート入力フォーム + 状態表示）

**Files:**

- Modify: `poc/frontend/src/App.svelte`
- Modify: `poc/frontend/src/style.css`（あれば）

- [ ] **Step 1: `poc/frontend/src/App.svelte` を以下の内容で**置き換え****

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { GetConfig, SaveAndStart, Stop, GetStatus } from '../wailsjs/go/main/App';

  let port: number = 50000;
  let running: boolean = false;
  let runningPort: number = 0;
  let message: string = '';
  let error: string = '';
  let busy: boolean = false;

  async function refreshStatus() {
    const s = await GetStatus();
    running = s.running;
    runningPort = s.port;
  }

  async function load() {
    try {
      const c = await GetConfig();
      port = c.port || 50000;
      await refreshStatus();
    } catch (e: any) {
      error = String(e);
    }
  }

  async function onSaveAndStart() {
    error = '';
    message = '';
    busy = true;
    try {
      await SaveAndStart(port);
      message = `ポート ${port} で起動しました`;
      await refreshStatus();
    } catch (e: any) {
      error = String(e);
    } finally {
      busy = false;
    }
  }

  async function onStop() {
    error = '';
    message = '';
    busy = true;
    try {
      await Stop();
      message = '停止しました';
      await refreshStatus();
    } catch (e: any) {
      error = String(e);
    } finally {
      busy = false;
    }
  }

  onMount(load);
</script>

<main>
  <h1>BMS RTC POC</h1>

  <section class="status">
    {#if running}
      <span class="dot ok"></span>
      <span>起動中（ポート {runningPort}）</span>
      <a href={`http://localhost:${runningPort}/`} target="_blank" rel="noreferrer">開く</a>
    {:else}
      <span class="dot off"></span>
      <span>停止中</span>
    {/if}
  </section>

  <section class="form">
    <label>
      ポート番号:
      <input type="number" min="1" max="65535" bind:value={port} disabled={busy} />
    </label>
    <div class="actions">
      <button on:click={onSaveAndStart} disabled={busy}>保存して起動</button>
      <button on:click={onStop} disabled={busy || !running}>停止</button>
    </div>
  </section>

  {#if message}
    <p class="message ok">{message}</p>
  {/if}
  {#if error}
    <p class="message err">{error}</p>
  {/if}
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    padding: 24px;
    color: #1b2636;
  }
  h1 { margin: 0 0 16px 0; font-size: 18px; }
  .status { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; }
  .dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
  .dot.ok { background: #2e7d32; }
  .dot.off { background: #aaa; }
  .form label { display: block; margin-bottom: 12px; }
  .form input { margin-left: 8px; padding: 4px 8px; width: 120px; }
  .actions { display: flex; gap: 8px; }
  button { padding: 6px 12px; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: 0.6; }
  .message.ok { color: #2e7d32; }
  .message.err { color: #b71c1c; }
</style>
```

- [ ] **Step 2: 不要なテンプレ標準アセット（GreetやLogoなど）を削除**

`poc/frontend/src/main.ts` がデフォルトテンプレで生成されている場合、内容は次に書き換える（不要なロゴimport等を除去）:

```ts
import './style.css'
import App from './App.svelte'

const app = new App({
  target: document.getElementById('app')!,
})

export default app
```

`poc/frontend/src/style.css` は最小化（または空）:

```css
:root {
  color-scheme: light dark;
}
body {
  margin: 0;
  background: #f5f6f8;
}
#app {
  min-height: 100vh;
}
```

`poc/frontend/src/assets/` ディレクトリにテンプレ生成のロゴ画像等があれば削除しても問題ない（参照していないので）。

- [ ] **Step 3: フロントエンドのビルドが通ることを確認**

Run:
```bash
cd poc/frontend && npm install && npm run build && cd ../..
```

Expected: `poc/frontend/dist/` に index.html等が生成される。型エラーや未解決importがあれば修正する。

- [ ] **Step 4: Wails全体ビルドが通ることを確認**

Run:
```bash
cd poc && wails build && cd ..
```

Expected: `poc/build/bin/bms-rtc-poc.app` が生成される。

- [ ] **Step 5: コミット**

```bash
git add poc/frontend/
git commit -m "feat(poc): Svelte UIでポート入力フォーム・起動/停止ボタン・ステータス表示を実装"
```

---

## Task 6: macOSローカルでの動作確認

このタスクはコード変更を伴わない手動確認タスク。確認結果をTask 9のNOTES.md書き出し時にまとめる。

- [ ] **Step 1: `wails dev` で起動確認**

Run:
```bash
cd poc && wails dev
```

Expected:
- アプリウィンドウが開く
- 「停止中」と表示される
- ポート番号入力フォームに 50000 が入っている

UI動作チェックリスト:

- [ ] ポート番号 50000 で「保存して起動」を押す → 「起動中（ポート 50000）」表示、「ポート 50000 で起動しました」のメッセージ
- [ ] 別ターミナルで `curl http://localhost:50000/` → `{"hello":"world","port":50000}` が返る
- [ ] 「停止」ボタン → 「停止中」表示
- [ ] ポートを 50000 に設定して「保存して起動」、その後 同じポート 50000 でもう一度「保存して起動」 → 一旦停止してから再起動成功
- [ ] 既に他プロセスが使っているポート（例: 既存のwails dev用ポート、22 などroot占有ポート）を入れて「保存して起動」 → 赤字で「ポート XXX の確保に失敗」エラーが出る
- [ ] アプリウィンドウを閉じる → サーバが止まることを `curl` で確認（接続拒否）

すべてOKなら、`wails dev` を Ctrl+C で終了し、`cd ..`。

- [ ] **Step 2: `wails build` 成果物（.app）で動作確認**

Run:
```bash
cd poc && wails build && open ./build/bin/bms-rtc-poc.app && cd ..
```

Expected: アプリが開く。Step 1 のチェックリストを再度実施し、すべて通ることを確認。

- [ ] **Step 3: 永続化の確認**

Run:
```bash
cat poc/build/bin/bms-rtc-poc.app/Contents/MacOS/poc-config.json 2>/dev/null || \
  find ~/Library -name 'poc-config.json' 2>/dev/null || \
  find . -name 'poc-config.json' 2>/dev/null
```

Expected: `poc-config.json` がいずれかに作成されており、最後に保存したポート番号が記録されている。場所がどこかも NOTES.md に記録する（macOSの `os.Executable()` の挙動確認）。

- [ ] **Step 4: 確認結果をメモ**

NOTES.md にまだ書かないが、以下の事実を記憶しておく（Task 9でまとめる）:
- `wails dev` 時の `os.Executable()` の戻り値の挙動
- `wails build` 後の `.app` 内での `os.Executable()` の挙動
- `poc-config.json` の実際の保存場所
- 起動済みサーバ→ポート変更→再起動の挙動
- ウィンドウクローズ時のサーバ挙動

- [ ] **Step 5: コミット（特にコード変更がなければスキップ可）**

このタスクで挙動修正が必要になった場合のみコミット。例:
```bash
git add poc/
git commit -m "fix(poc): macOSローカル動作確認で見つかった不具合を修正"
```

修正不要なら次のタスクへ。

---

## Task 7: GitHub ActionsワークフローでWindows exe を生成

**Files:**

- Create: `.github/workflows/poc-build-windows.yml`

- [ ] **Step 1: `.github/workflows/poc-build-windows.yml` を作成**

```yaml
name: POC Build Windows

on:
  workflow_dispatch:

permissions:
  contents: read

jobs:
  build:
    runs-on: windows-latest
    defaults:
      run:
        working-directory: poc
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Wails CLI
        shell: pwsh
        run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0

      - name: Frontend install
        shell: pwsh
        working-directory: poc/frontend
        run: npm install

      - name: Wails build (windows/amd64)
        shell: pwsh
        run: wails build -platform windows/amd64

      - name: Upload exe artifact
        uses: actions/upload-artifact@v4
        with:
          name: poc-windows-amd64
          path: poc/build/bin/bms-rtc-poc.exe
          if-no-files-found: error
          retention-days: 14
```

- [ ] **Step 2: コミット**

```bash
git add .github/workflows/poc-build-windows.yml
git commit -m "ci(poc): Windows exe を workflow_dispatch で生成するActionsを追加"
```

- [ ] **Step 3: ブランチをリモートにプッシュ**

```bash
git push -u origin feat/initial-design
```

Expected: リモートにブランチが作られる。

注: ワークフロー定義はリモートに存在する必要があり、`workflow_dispatch` はデフォルトブランチ以外のブランチで定義されているとUI/ APIから一覧に出ない場合がある。`gh workflow run` は `--ref <branch>` で対象ブランチを指定可能なのでブランチのままで動く（次タスクで使う）。

---

## Task 8: Windows ビルドをgh CLIで発火し動作確認

このタスクは手動確認＋ Windows実機/VMでの動作確認を含む。

- [ ] **Step 1: ワークフローを発火**

Run:
```bash
gh workflow run poc-build-windows.yml --ref feat/initial-design
```

Expected: 「Workflow triggered」のような出力。

- [ ] **Step 2: 実行を監視**

Run:
```bash
sleep 3
gh run list --workflow poc-build-windows.yml --branch feat/initial-design --limit 1
```

直近のrun IDを取得し、

```bash
gh run watch <run-id>
```

Expected: 数分後に成功（`✓ Completed`）。失敗した場合は `gh run view <run-id> --log-failed` でログを取得し、ワークフローを修正してTask 7のStep 2-3を再実行。

- [ ] **Step 3: Artifact を取得**

Run:
```bash
mkdir -p tmp/poc-windows
gh run download <run-id> --name poc-windows-amd64 --dir tmp/poc-windows
ls tmp/poc-windows/
```

Expected: `bms-rtc-poc.exe` が存在する。

- [ ] **Step 4: Windows実機/VMで動作確認**

- `tmp/poc-windows/bms-rtc-poc.exe` を Windows 機/VM に転送（共有フォルダ、scp、または OneDrive など）
- 起動 → アプリウィンドウが開く
- Task 6 Step 1 のUI動作チェックリストを Windows 上でも同様に実施
  - ポート 50000 で「保存して起動」 → 別の Windows ターミナル（PowerShell）で `Invoke-WebRequest http://localhost:50000/` または ブラウザで http://localhost:50000/ にアクセス → JSONが返る
  - ポート衝突時のエラー表示
  - 停止ボタン → 接続不可
  - ウィンドウクローズ → サーバ停止
- `poc-config.json` の保存場所も確認（`bms-rtc-poc.exe` と同じディレクトリにできているか）

- [ ] **Step 5: 結果をメモ（Windows ビルドの挙動・注意点を Task 9 で NOTES.md にまとめる）**

特に以下を控えておく:
- `os.Executable()` の戻り値（Windows での実際のパス）
- WebView2 Runtime の依存（ない環境ではどう振る舞うか）
- ポート衝突エラーのメッセージ表記が macOS と一致するか

- [ ] **Step 6: コミット（Windows 検証で修正が出た場合のみ）**

修正があれば:
```bash
git add poc/ .github/
git commit -m "fix(poc): Windows実機動作確認で見つかった不具合を修正"
git push
```

修正がなければ次のタスクへ。

---

## Task 9: NOTES.md（Phase 1 へ持ち越す知見）作成 + 仕上げ

**Files:**

- Create: `poc/NOTES.md`

- [ ] **Step 1: `poc/NOTES.md` を作成**

実際にPOCで得た知見を反映する。下記はテンプレートかつ最低限カバーすべき項目:

```markdown
# POC で学んだこと（Phase 1 へ引き継ぐ）

このメモは Phase 1（MVP本実装）で再現すべき事実・回避すべき落とし穴を残す目的のもの。
POC自体はクリーンに作り直すが、ここに書かれた知見は MVP に流用する。

## 1. Wails アプリのライフサイクル

- `OnStartup(ctx)` のタイミング: ウィンドウ表示直前に呼ばれる。`a.ctx = ctx` で保持し、後続の `runtime.*` 呼び出しに使う。
- `OnShutdown(ctx)` のタイミング: ウィンドウ完全終了時。ここで HTTP サーバの `Shutdown` を呼ぶことで、終了時にポートが解放される。
- `OnBeforeClose` は今回未使用（Phase 1 でトレイ常駐の際に使う）。

## 2. HTTP サーバの起動位置

- `App` が `*Server` を保持し、Bind メソッド `SaveAndStart(port)` から起動する形が動作した。
- `net.Listen` を**同期で呼んでから** `srv.Serve(ln)` を goroutine に投げることで、ポート確保失敗を呼び出し元（フロントエンド）に同期的に返せる。
- 既存サーバが動いていたら `Shutdown(5sタイムアウト)` してから新サーバを起動するパターンで衝突なく入れ替えられた。

## 3. Bind メソッドの作法

- 戻り値の Go 型は JSON 化されてフロントへ届く。`type Status struct {...}` のような小さい構造体を返すのが扱いやすい。
- error を返すと、フロント側は `try { await Bind() } catch (e) { ... }` で文字列として受け取れる。
- Bind 対象は `interface{}` のスライスに渡す。POCは `app` 一つだけ。

## 4. 永続化先 (`os.Executable()` の挙動)

- `wails dev` 時: 実行ファイルパスは `<somewhere>/__debug_bin*` のような一時パスになり、`./poc-config.json` は **そのディレクトリに作られる**（または cwd フォールバック）。
  - 実測: <ここに実際のパスを書く>
- macOS `wails build` 後の `.app` 内: `Contents/MacOS/bms-rtc-poc` の隣に `poc-config.json` が作られる。
  - `.app` バンドル内に書き込みが行われるため、署名やGatekeeperの観点で本実装では別の保存先（`UserConfigDir` など）を検討してもよい。**ただし今回の設計では「実行ファイル隣」をポータブル運用として採用済み**なので、Phase 1 でも同方針で進める。
- Windows: `bms-rtc-poc.exe` の隣に `poc-config.json` が作成された。
  - 実測: <ここに実際のパスを書く>

## 5. Windows ビルド (GitHub Actions) の注意点

- `actions/setup-go@v5` (Go 1.24) + `actions/setup-node@v4` (Node 20) + `wails install ...@v2.11.0` の組み合わせで動いた。
- `working-directory: poc` を job レベル / step レベルで設定する必要があった。
- `wails build` の前に `frontend/` で `npm install` が必要だった（CI では node_modules が空なので明示）。
- Artifact 名 `poc-windows-amd64`、サイズ: <実測>。ダウンロード後そのまま実行可能。
- WebView2 Runtime: <インストール状況の確認結果>

## 6. Phase 1 へ引き継ぐ判断

- システムトレイ: POCではスコープ外として確認していない。Phase 1 最初のタスクとして「systray ライブラリ単独検証」を独立タスクで実施する。候補は `getlantern/systray`。
- シングルインスタンス: ロックファイル + 名前付きパイプの実装は Phase 1 で。
- 設定永続化先: 「実行ファイル隣」で macOS / Windows ともに動作確認済み。MVP本体でも同方針を採用してOK。
- ポート確保失敗のUX: 同期エラー → フロント catch で表示するパターンが分かりやすかった。MVP の設定UIでも同じ流れで実装する。
- Windows ビルドCI: `workflow_dispatch` + `actions/upload-artifact@v4` で十分。MVP段階でも同形式で運用、タグpushや自動リリースは将来扱い。

## 7. 注意・落とし穴

- <POC実装中に詰まった点を箇条書きで残す。例: ビルドフロー、依存バージョン、設計ドキュメントとの差異、など>
```

実装中に得た事実で `<...>` プレースホルダ部分を埋める。Task 6 / Task 8 の手動確認で把握した情報をここに反映。

- [ ] **Step 2: NOTES.md の最終確認**

NOTES.md にすべての項目（Wailsライフサイクル / HTTPサーバ / Bind作法 / 永続化先 / Windowsビルド / Phase 1 引き継ぎ判断 / 落とし穴）が埋まっていることを確認。プレースホルダ `<...>` が残っていないこと。

- [ ] **Step 3: コミット**

```bash
git add poc/NOTES.md
git commit -m "docs(poc): Phase 1 へ引き継ぐ知見を NOTES.md に記録"
```

- [ ] **Step 4: ブランチをリモートにプッシュ**

```bash
git push
```

Expected: リモートが最新コミットまで更新される。

---

## 最終チェックリスト

POC完了の自己診断（すべてチェックが入ること）:

- [ ] `poc/` 配下にWailsアプリが独立Goモジュールで存在する
- [ ] `wails dev` で起動 → ポート保存→起動→`curl` 応答確認 OK（macOS）
- [ ] `wails build` 成果物 `.app` で同じサイクル OK（macOS）
- [ ] ポート衝突時にGUIにエラー表示
- [ ] ウィンドウクローズでサーバ停止
- [ ] `poc-config.json` が永続化されている
- [ ] `.github/workflows/poc-build-windows.yml` がリモートに存在
- [ ] `gh workflow run` で発火 → Artifact `poc-windows-amd64` が成功で生成
- [ ] Artifact を Windows 機/VM で起動し、ポート保存→起動→ブラウザでJSON応答確認 OK
- [ ] `poc/NOTES.md` が埋まっており、プレースホルダなし
- [ ] すべての変更が `feat/initial-design` ブランチにコミット・push 済み

完了後、ユーザー（meta-BE）に Phase 0 完了を報告し、Phase 1 のプラン作成セッションへ移行する。
