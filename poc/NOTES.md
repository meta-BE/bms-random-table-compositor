# POC で学んだこと（Phase 1 へ引き継ぐ）

このメモは Phase 1（MVP本実装）で再現すべき事実・回避すべき落とし穴を残す目的のもの。
POC自体はクリーンに作り直すが、ここに書かれた知見は MVP に流用する。

## 1. Wails アプリのライフサイクル

- `OnStartup(ctx)` のタイミング: ウィンドウ表示直前に呼ばれる。`a.ctx = ctx` で保持し、後続の `runtime.*` 呼び出しに使う。
- `OnShutdown(ctx)` のタイミング: ウィンドウ完全終了時。ここで HTTP サーバの `Shutdown` を呼ぶことで、終了時にポートが解放される。
- `OnBeforeClose` は POC では未使用（Phase 1 でトレイ常駐の際、ウィンドウクローズをトレイ格納に変える目的で使う）。

## 2. HTTP サーバの起動位置

- `App` が `*Server` を保持し、Bind メソッド `SaveAndStart(port)` から起動する形が動作した。
- `net.Listen` を**同期で呼んでから** `srv.Serve(ln)` を goroutine に投げることで、ポート確保失敗を呼び出し元（フロントエンド）に同期的に返せる。
- 既存サーバが動いていたら `Stop` してから新サーバを起動するパターンで衝突なく入れ替えられた。

## 3. Bind メソッドの作法

- 戻り値の Go 型は JSON 化されてフロントへ届く。`type Status struct {...}` のような小さい構造体を返すのが扱いやすい。
- error を返すと、フロント側は `try { await Bind() } catch (e) { ... }` で文字列として受け取れる。
- Bind 対象は `[]any` のスライスに渡す。POCは `app` 一つだけ。
- `Bind: []interface{}` でも動くが、Go 1.18+ 慣例に合わせて `[]any` を使う（LSP診断 `S6014` 等）。
- フロント側 import: `import { GetConfig, ... } from '../wailsjs/go/main/App'`（wails build / wails dev 時に自動生成、`.gitignore` で除外）。

## 4. 永続化先 (`os.Executable()` の挙動 / 実測値)

設計方針: 設定ファイルは「実行ファイル隣」に保存するポータブル運用。

- **macOS `wails dev` 時**: `os.Executable()` の戻り値は `__debug_bin*` 系の一時パス。実測では `poc-config.json` がカレントディレクトリ (`poc/poc-config.json`) に作成された。`configPath()` の `os.Executable()` エラーフォールバック (cwd) ではなく、一時バイナリ隣に作られる挙動。
- **macOS `wails build` 後の `.app`**: 実測パス `poc/build/bin/bms-rtc-poc.app/Contents/MacOS/poc-config.json`（実行ファイル `Contents/MacOS/bms-rtc-poc` の隣）。
- **Windows ビルド (.exe)**: `bms-rtc-poc.exe` の隣。実機動作確認済み（POC 完了基準）。

注: macOS `.app` バンドル内へ書き込みが行われるため、署名や Gatekeeper の観点で本実装では別の保存先（`UserConfigDir` など）を検討してもよい。**ただし今回の設計では「実行ファイル隣」をポータブル運用として採用済み**なので、Phase 1 でも同方針で進める。

## 5. Windows ビルド (GitHub Actions) の注意点

### 採用したセットアップ

- `actions/checkout@v4` + `actions/setup-go@v5`（Go 1.24） + `actions/setup-node@v4`（Node 20） + `wails install ...@v2.11.0` の組み合わせで動作。
- `defaults.run.working-directory: poc` を job レベルに設定。`npm install` は step レベルで `working-directory: poc/frontend` 上書きが必要。
- `wails build` の前に `frontend/` で `npm install` が必要（CI では node_modules が空なので明示）。
- `actions/upload-artifact@v4` で `name: poc-windows-amd64`、`if-no-files-found: error`、`retention-days: 14`。
- ローカルから `gh workflow run poc-build-windows.yml --ref feat/initial-design` で発火。
- `gh run download <run-id> --name poc-windows-amd64 --dir tmp/poc-windows` で Artifact 取得。`bms-rtc-poc.exe` 単独、約 11MB。

### 詰まった点と対処

#### 5.1 `gh workflow run` の default-branch 制約

GitHub API の仕様で、`workflow_dispatch` をAPI/`gh CLI` から発火するには、ワークフローファイルが**デフォルトブランチ (main) に存在する必要がある**。ファイルが feature ブランチにしかないと `HTTP 404: workflow not found on the default branch` が返る。

**対処**: feature ブランチ開発中は、`workflow_dispatch` 用 yml を **main に cherry-pick** して反映してから、`gh workflow run --ref <feature-branch>` で feature ブランチの内容で実行する。

#### 5.2 Wails Windows ビルドの exe 名

`wails init -n bms-rtc-poc -d poc` でアプリ名を `bms-rtc-poc` 指定したが、Windows ビルドの出力 exe 名は **プロジェクトディレクトリ名 `poc` を使う** (`Built '...\poc\build\bin\poc.exe'`)。macOS の `.app` ファイル名 (`bms-rtc-poc.app`) とは異なる解決ロジック。

**対処**: ワークフローに `Rename-Item` ステップを追加して `poc.exe` → `bms-rtc-poc.exe` に改名してから `upload-artifact`。

```yaml
- name: Rename exe
  shell: pwsh
  run: Rename-Item -Path build/bin/poc.exe -NewName bms-rtc-poc.exe
```

Phase 1 では `wails.json` か `info.json` で `outputfilename` を指定する手段の検証を推奨。

#### 5.3 `setup-go` のキャッシュ警告

`Restore cache failed: Dependencies file is not found in D:\a\.... Supported file pattern: go.sum` という警告が出る。`go.sum` がリポジトリルートではなく `poc/go.sum` にあるため、`setup-go` のデフォルトキャッシュ復元が機能しない。**実害なし**（毎回ダウンロードするだけ）。

Phase 1 では:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: '1.24'
    cache-dependency-path: poc/go.sum
```

で解決可能。

#### 5.4 Node.js 20 deprecation

GHA のアノテーションで「Node.js 20 actions are deprecated」警告。2026-06-02 から Node.js 24 がデフォルト、2026-09-16 で完全削除予定。Phase 1 で actions のバージョン更新を検討（v4 actions が Node.js 24 対応バージョンに上がっているか確認）。

#### 5.5 WebView2 Runtime

Windows 実機動作確認では特に問題なし。Phase 1 のリリース時は WebView2 Runtime インストール状況のドキュメント記載を検討（Windows 11 はデフォルト同梱、Windows 10 でも近年は同梱）。

## 6. 停止時ハングの対処

POC 開発中に発覚: `http.Server.Shutdown(ctx)` はクライアント側の keep-alive アイドル接続が自然に閉じるか ctx タイムアウトまで待つ。ブラウザでアクセスした後に「停止」を押すと数秒ハングした。

**対処** (採用):

1. `http.Server.IdleTimeout: 2 * time.Second` を設定 → keep-alive アイドル接続を 2 秒で切断
2. `Shutdown(ctx)` がエラーを返した場合は `srv.Close()` で強制終了するフォールバック
3. `App` 側の Shutdown ctx タイムアウトを 5 秒 → 1 秒に短縮（強制Close前提で速度優先）
4. `Shutdown` 呼び出し前に mutex を解放してデッドロック回避

Phase 1 でも同じパターンを HTTP サーバの停止挙動に適用する。

## 7. 「開く」リンクの作法

`<a href="..." target="_blank">` は Wails の WebView 内ではクリックしても外部ブラウザを開かない（既知の挙動）。

**対処**: Bind メソッド `OpenURL(url string)` を追加し、`runtime.BrowserOpenURL(a.ctx, url)` を呼ぶ。フロント側はリンク風 button + `on:click={onOpen}` で対応。

```go
import "github.com/wailsapp/wails/v2/pkg/runtime"

func (a *App) OpenURL(url string) {
    runtime.BrowserOpenURL(a.ctx, url)
}
```

Phase 1 でも公開表 HTML ビュー内のリンク等で同じ対処が必要になる可能性あり（ただしHTTPサーバ経由のページはWebView外なので不要）。GUI 設定画面内のリンクには必要。

## 8. LSP/IDE の表示警告

`poc/frontend/src/main.ts` 行2 で `Cannot find module './App.svelte'` の TypeScript 警告が出ることがある（`vite-env.d.ts` に `/// <reference types="svelte" />` が存在していてもキャッシュ起因で発生）。

**実害**: なし。`npm run build` も `wails build` も成功する。LSP / TS Server のキャッシュリフレッシュ（IDE 再起動、`tsserver` 再起動）で消える。Phase 1 で TS 設定を見直す際は `tsconfig.json` の `include` を確認。

## 9. Phase 1 へ引き継ぐ判断

- **システムトレイ常駐**: POC ではスコープ外で未検証。Phase 1 最初のタスクとして「systray ライブラリ単独検証」を独立タスクで実施する。候補は `getlantern/systray`。検証内容: `OnBeforeClose` で `runtime.WindowHide()` → トレイ格納、トレイメニューから設定再表示・終了、HTTPサーバ稼働状態に応じたアイコン色切替。
- **シングルインスタンスロック**: ロックファイル (`./.lock`) + 既存窓への前面化IPC（名前付きパイプ or watchファイル）の実装は Phase 1。
- **設定永続化先**: 「実行ファイル隣」で macOS / Windows ともに動作確認済み。MVP本体でも同方針を採用してOK（設計ドキュメント Q13 で確定済み）。
- **ポート確保失敗のUX**: 同期エラー → フロント catch で表示するパターンが分かりやすかった。MVP の設定UIでも同じ流れで実装する。
- **Windows ビルドCI**: `workflow_dispatch` + `actions/upload-artifact@v4` + cherry-pick による main 反映で十分。MVP段階でも同形式で運用、タグpushや自動リリースは将来扱い。
- **HTTPサーバ停止の高速化**: `IdleTimeout` + `Shutdown→Close` フォールバックパターンは MVP でも採用。

## 10. その他注意・落とし穴

- `wails build` 時に `frontend/wailsjs/` ディレクトリが自動生成される。`.gitignore` で除外する必要あり（wails init 生成の `poc/.gitignore` 自体は `frontend/dist`, `node_modules`, `build/bin` のみ除外で `wailsjs` は対象外なので、リポジトリルートの `.gitignore` で `poc/frontend/wailsjs/` を明示的に除外している）。
- `wails init` 生成テンプレの `App.svelte` は `Greet` 関数に依存しているため、`app.go` を上書きする際は `App.svelte` も同時に書き換える必要がある（フロントエンドビルドが失敗する）。
- Bind メソッド名を変更したら `wails build` で `wailsjs/` を再生成する必要がある。フロントエンドだけ `npm run build` してもバインディングは更新されない。
