# Phase 1 / Plan 4: GUI 仕上げ + ダッシュボード + Plan 2.5 統合 + ドキュメント整備 実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Phase 1 MVP の仕上げとして、Tailwind v4 + daisyUI v5 (theme: `emerald`) を導入して既存 3 タブを書き直し、ダッシュボードタブを新設して最近のリクエスト・ソース表更新履歴・現在のピック結果を可視化、Plan 2.5 (`songdata.db` 参照ボタン + 確認ダイアログ + 行コンテキストメニュー) を統合、トレイアイコンを 3 状態色切替化、HTML ビューの所持判定スタブを `OwnedMD5Cache` で正しい色分けにリファクタ、最小ドキュメント (`docs/manual.md` / `docs/test-plan.md`) を整備する。

**Architecture:** フロントは bms-elsa の Tailwind v4 + daisyUI v5 構成 (`vite.config.ts` に `@tailwindcss/vite` プラグイン、`style.css` に `@import "tailwindcss"` + `@plugin "daisyui" { themes: emerald --default; }` のみ) を踏襲し、各タブは daisyUI コンポーネント (btn / input / table / badge / alert / loading / modal / dropdown) で書き直す。ダッシュボードはバックエンドに `internal/usecase/dashboard_usecase.go` (汎用 `RingBuffer[T]` + `RequestLogEntry` / `FetchLogEntry`) を新設、HTTP リクエストは `internal/adapter/httpserver/middleware.go` でラップして集計、ソース表更新は既存 `SourceTableUseCase.refreshOne` 完了時にコールバック通知、ピック結果は既存 `PickResultStore` の `OnChange` フックを追加。フロントは `onMount` で `DashboardHandler.Snapshot()` を fetch し、その後 Wails event (`dashboard:request_logged` / `dashboard:fetch_logged` / `dashboard:pick_changed`) で増分更新するハイブリッド方式。HTML ビューの所持判定は `httpserver.Deps` に `Owned *usecase.OwnedMD5Cache` を追加し、`buildHTMLPageData` のスタブ `ownedSet := map[string]struct{}{}` を `deps.Owned.Get(ctx)` 呼び出しに置換。トレイアイコンは `internal/adapter/tray/icons/{idle,running,error}.png` (16x16 純色 PNG、ビルド時 `go:embed`) を `IconFor(State)` から返し、`app.go` から `serverUC.OnStatusChange` で `tray.SetState` を呼ぶ配線を追加。

**Tech Stack:** Go 1.24 / Wails v2.11.0 / `modernc.org/sqlite` (既存) / 標準 `net/http` / Svelte + TypeScript / Tailwind CSS v4.2.1 (新規導入、Vite プラグイン形式) / daisyUI v5.5.19 (theme: `emerald`) / `fyne.io/systray` (既存)

**設計ドキュメント:** ブレストメモ (本セッション) で固めた決定事項を本 Plan 内に直接埋め込む (spec はスキップ方針)。spec 本体への参照は `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` の §9 (ロギング・ダッシュボード方針) / §11 (プロジェクト構造) / §16 (主要設計判断) を補助的に参照。

**Phase 1 全体の Plan 分割:** Plan 1 (基盤＝完了) → Plan 2 (ソース表取り込み＝完了) → Plan 3 (公開表 + ピック + HTTPサーバ＝完了) → **Plan 4 (本ファイル)**

**完了条件:**

- `frontend/src/style.css` に `@import "tailwindcss"` + `@plugin "daisyui" { themes: emerald --default; }` が入り、4 タブ全てが daisyUI コンポーネントで動作 (`make dev` でホットリロード可、`make build` で macOS 成果物が生成)
- 4 タブ全てに「データ 0 件のプレースホルダ」「ローディングスピナー」「エラー時の `alert-error` / `badge-error`」が入っている
- ダッシュボードタブで以下が表示・自動更新される:
  - 最近のリクエスト 100 件 (新しい順、JST、メソッド / パス / ステータス / 経過 ms)
  - ソース表更新履歴 (起動後の取得分のみ、JST、表示名 / 結果 / エラー)
  - 現在のピック結果サマリ (公開表ごとに card で slug / 生成時刻 / レベル別曲数)
- `songdata.db` パスを「参照」ボタンから OS ファイル選択ダイアログで指定できる
- ソース表 / 公開表の削除が ConfirmDialog で確認後に実行される
- ソース表 / 公開表の行を右クリックでコンテキストメニュー (編集 / 削除 / 再取得 or 再ピック / ブラウザで開く) が出る
- Windows トレイアイコンが Idle (グレー) / Running (緑) / Error (赤) の 3 色で切り替わる
- HTML ビューで `OwnedOnly=false` 公開表でも実所持で色分けされる (`OwnedMD5Cache` 経由)
- `docs/manual.md` (初回起動 → 設定 → ソース表 → 公開表 → beatoraja 接続 + トラブルシューティング数項目、IPv4 ポート占有問題含む) と `docs/test-plan.md` (手動 E2E チェックリスト) がコミットされている
- Plan 1-3 の動作 (ServerTab 起動/停止 / SourceTablesTab 追加/更新/削除 / PublishedTablesTab CRUD / 3 HTTPエンドポイント / SingleInstance / トレイ常駐) が無回帰
- `go build ./...` / `go test ./...` 全 pass、`make build` で macOS 成果物生成、`gh workflow run build-windows.yml` で Windows exe 生成 + 実機 E2E 確認 (test-plan.md チェックリスト)

**スコープ外 (v2 以降):**

- ピックアルゴリズム B / C、最終プレイ日時優先、コースデータ
- ETag 304 の本格運用、ソース表のスケジュール自動更新
- IPv4 ループバック自接続テスト (マニュアル記載のみ)
- 複数ソース表合成 (spec §15)
- スクリーンショット / 動画ドキュメント
- アクセシビリティ (aria, tabindex 等)
- トースト通知

**ブランチ運用:** Plan 1-3 と同様 main 上で直接コミット。完了時は `git push origin main` で remote 反映 (Windows ビルドの `workflow_dispatch` がデフォルトブランチ参照のため)。

---

## ファイル構造 (Plan 4 終了時点で追加・変更されるもの)

新規作成:

```
internal/
├── usecase/
│   ├── ring_buffer.go                                # 汎用 RingBuffer[T]
│   ├── ring_buffer_test.go
│   ├── dashboard_usecase.go                          # DashboardUseCase + RequestLogEntry + FetchLogEntry
│   └── dashboard_usecase_test.go
├── domain/
│   └── dashboard.go                                  # RequestLogEntry / FetchLogEntry / DashboardSnapshot 構造体
├── adapter/
│   ├── httpserver/
│   │   ├── middleware.go                             # logging middleware
│   │   └── middleware_test.go
│   └── tray/
│       └── icons/                                    # 純色 PNG 素材ディレクトリ
│           ├── idle.png                              # 16x16 純色グレー
│           ├── running.png                           # 16x16 純色緑
│           ├── error.png                             # 16x16 純色赤
│           └── gen.go                                # `//go:build ignore` の素材生成スクリプト
└── app/handler/
    ├── dashboard_handler.go
    └── dashboard_handler_test.go

frontend/src/lib/
├── components/
│   ├── ConfirmDialog.svelte                          # daisyUI <dialog class="modal">
│   ├── confirm.ts                                    # Promise<boolean> ヘルパ
│   └── ContextMenu.svelte                            # daisyUI dropdown 流用
└── tabs/
    └── DashboardTab.svelte                           # 新規 4 番目のタブ

docs/
├── manual.md                                         # ユーザー向け最小マニュアル
└── test-plan.md                                      # 手動 E2E チェックリスト
```

変更:

```
frontend/
├── package.json                                      # tailwindcss / daisyui / @tailwindcss/vite 追加
├── vite.config.ts                                    # @tailwindcss/vite プラグイン追加
├── src/
│   ├── style.css                                     # 全置換 (Tailwind + daisyUI 設定)
│   ├── App.svelte                                    # daisyUI tabs-boxed に書き直し + Dashboard タブ追加
│   └── lib/
│       ├── api.ts                                    # dashboard / pickSongdataDB API + 新 events 追加
│       └── tabs/
│           ├── ServerTab.svelte                      # daisyUI 化 + 参照ボタン
│           ├── SourceTablesTab.svelte                # daisyUI 化 + ConfirmDialog + ContextMenu
│           └── PublishedTablesTab.svelte             # daisyUI 化 + ConfirmDialog + ContextMenu

internal/
├── app/
│   ├── bootstrap.go                                  # DashboardUseCase + DashboardHandler 配線追加
│   └── handler/
│       └── config_handler.go                         # PickSongdataDB メソッド追加
├── adapter/
│   ├── httpserver/
│   │   ├── server.go                                 # Deps に Owned 追加
│   │   ├── router.go                                 # middleware 適用
│   │   ├── handler_html.go                           # buildHTMLPageData の OwnedMD5Cache 経由化
│   │   └── handler_html_test.go                      # OwnedMD5Cache 経由テスト
│   └── tray/
│       └── icons.go                                  # IconFor の状態別差し替え
├── usecase/
│   ├── pick_result_store.go                          # OnChange リスナー追加
│   ├── pick_result_store_test.go                     # リスナーテスト
│   ├── source_table_usecase.go                       # refreshOne 完了時の hook 追加
│   └── source_table_usecase_test.go                  # hook テスト

main.go                                              # Bind に DashboardHandler 追加
app.go                                               # OnStartup で tray SetState 配線 + dashboard event emit
```

---

## Task 1: Tailwind v4 + daisyUI v5 導入

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/vite.config.ts`
- Modify: `frontend/src/style.css`

bms-elsa が採用済みの Tailwind v4 + daisyUI v5 を最小コストで導入する。Tailwind v4 は config-less で、Vite プラグイン + style.css の `@import` / `@plugin` だけで完結する。

- [ ] **Step 1: 依存追加 (バージョン明示)**

`go get` ではなく `npm install` だが、Plan 2 lessons #1「依存追加時はバージョン明示」を踏襲し、bms-elsa と同じバージョンに固定する。

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/frontend
npm install -D tailwindcss@4.2.1 @tailwindcss/vite@4.2.1 daisyui@5.5.19
```

期待される `frontend/package.json` の `devDependencies` に以下が追加される:

```json
{
  "devDependencies": {
    "@tailwindcss/vite": "^4.2.1",
    "daisyui": "^5.5.19",
    "tailwindcss": "^4.2.1"
  }
}
```

- [ ] **Step 2: vite.config.ts にプラグイン追加**

現在の `frontend/vite.config.ts` を確認し、`@tailwindcss/vite` を追加する:

```ts
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
});
```

- [ ] **Step 3: style.css 全置換**

`frontend/src/style.css` を以下で全置換 (既存 11 行を捨てる):

```css
@import "tailwindcss";
@plugin "daisyui" {
  themes: emerald --default;
}

#app {
  height: 100vh;
}
```

- [ ] **Step 4: 開発サーバで確認**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
make dev
```

確認:
- ビルドエラーなし
- 既存 4 タブ (ServerTab / SourceTablesTab / PublishedTablesTab) が表示される (見た目はまだ素 CSS 寄り、後続タスクで daisyUI 化)
- ブラウザの DevTools で `<html>` / `<body>` の computed styles に Tailwind の reset が当たっている
- `daisyUI` の theme `emerald` が適用されている (確認方法: DevTools コンソールで `getComputedStyle(document.documentElement).getPropertyValue('--color-primary')` が emerald カラーを返す)

確認できたら `Ctrl-C` で `make dev` を停止。

- [ ] **Step 5: コミット**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git add frontend/package.json frontend/package-lock.json frontend/vite.config.ts frontend/src/style.css
git commit -m "chore(frontend): Tailwind v4 + daisyUI v5 (emerald) を導入"
```

---

## Task 2: ConfirmDialog 共通コンポーネント

**Files:**
- Create: `frontend/src/lib/components/ConfirmDialog.svelte`
- Create: `frontend/src/lib/components/confirm.ts`

daisyUI `<dialog class="modal">` を Svelte コンポーネント化し、`confirm({title, message}): Promise<boolean>` ヘルパで Promise ベースに使えるようにする。Plan 2 lessons #2 で `window.confirm` が Wails WebView で動かないことが分かっており、本コンポーネントが置換となる。

- [ ] **Step 1: confirm.ts のヘルパを書く**

`frontend/src/lib/components/confirm.ts`:

```ts
import { mount, unmount } from 'svelte';
import ConfirmDialog from './ConfirmDialog.svelte';

export type ConfirmOptions = {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    const target = document.createElement('div');
    document.body.appendChild(target);
    const component = mount(ConfirmDialog, {
      target,
      props: {
        ...opts,
        onResult: (ok: boolean) => {
          unmount(component);
          target.remove();
          resolve(ok);
        },
      },
    });
  });
}
```

注: Svelte 5 と Svelte 4 では mount/unmount API が異なる。プロジェクトの Svelte バージョンを確認し、4 系の場合は以下に書き直す:

```ts
// Svelte 4 の場合
import ConfirmDialog from './ConfirmDialog.svelte';

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    const target = document.createElement('div');
    document.body.appendChild(target);
    const component = new ConfirmDialog({
      target,
      props: {
        ...opts,
        onResult: (ok: boolean) => {
          component.$destroy();
          target.remove();
          resolve(ok);
        },
      },
    });
  });
}
```

確認:

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/frontend
grep -E '"svelte":' package.json
```

`"svelte": "^4.x.x"` なら Svelte 4 系のコードを採用。

- [ ] **Step 2: ConfirmDialog.svelte 本体を書く**

`frontend/src/lib/components/ConfirmDialog.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';

  export let title: string;
  export let message: string;
  export let confirmLabel: string = 'OK';
  export let cancelLabel: string = 'キャンセル';
  export let danger: boolean = false;
  export let onResult: (ok: boolean) => void;

  let dialog: HTMLDialogElement;

  onMount(() => {
    dialog.showModal();
  });

  function handleConfirm() {
    dialog.close();
    onResult(true);
  }

  function handleCancel() {
    dialog.close();
    onResult(false);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      handleCancel();
    }
  }
</script>

<dialog bind:this={dialog} class="modal" on:keydown={handleKeydown}>
  <div class="modal-box">
    <h3 class="font-bold text-lg">{title}</h3>
    <p class="py-4 whitespace-pre-line">{message}</p>
    <div class="modal-action">
      <button class="btn" on:click={handleCancel}>{cancelLabel}</button>
      <button
        class="btn"
        class:btn-error={danger}
        class:btn-primary={!danger}
        on:click={handleConfirm}>{confirmLabel}</button>
    </div>
  </div>
</dialog>
```

- [ ] **Step 3: スモークテスト用の一時利用を ServerTab に仮入れ**

`frontend/src/lib/tabs/ServerTab.svelte` の `<script>` セクションに一時的に追加 (後続タスクで本格利用するので、ここでは動作確認のみ):

```ts
import { confirm } from '../components/confirm';

async function smokeTest() {
  const ok = await confirm({
    title: '動作確認',
    message: 'ConfirmDialog のスモークテストです。OK で true、キャンセルで false。',
  });
  console.log('confirm result:', ok);
}
```

ServerTab のテンプレに一時的にボタンを追加:

```svelte
<button on:click={smokeTest}>ConfirmDialog test</button>
```

`make dev` で起動 → ServerTab を開いてボタン押下 → ダイアログが出る、OK / キャンセルで `console.log` が想定通り出ることをブラウザコンソールで確認。

- [ ] **Step 4: スモークテスト用の一時コードを除去**

確認後、`smokeTest` 関数と一時ボタンを削除する。`import { confirm }` も削除しておく (後続タスクで本格利用時に再 import)。

- [ ] **Step 5: コミット**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git add frontend/src/lib/components/ConfirmDialog.svelte frontend/src/lib/components/confirm.ts
git commit -m "feat(frontend): ConfirmDialog 共通コンポーネント (daisyUI modal) を追加"
```

---

## Task 3: ContextMenu 共通コンポーネント

**Files:**
- Create: `frontend/src/lib/components/ContextMenu.svelte`

行右クリックで出すメニュー。daisyUI `dropdown` の見た目を流用しつつ、`oncontextmenu` で右クリック位置に `position: fixed` で出す。

- [ ] **Step 1: ContextMenu.svelte 本体**

`frontend/src/lib/components/ContextMenu.svelte`:

```svelte
<script lang="ts">
  import { onDestroy } from 'svelte';

  export type MenuItem = {
    label: string;
    onClick: () => void;
    danger?: boolean;
    disabled?: boolean;
  };

  let visible = false;
  let x = 0;
  let y = 0;
  let items: MenuItem[] = [];

  export function open(event: MouseEvent, menuItems: MenuItem[]) {
    event.preventDefault();
    items = menuItems;
    x = event.clientX;
    y = event.clientY;
    visible = true;
  }

  function close() {
    visible = false;
  }

  function handleItem(item: MenuItem) {
    if (item.disabled) return;
    close();
    item.onClick();
  }

  function handleWindowClick(_e: MouseEvent) {
    close();
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }

  $: if (visible) {
    window.addEventListener('click', handleWindowClick);
    window.addEventListener('keydown', handleKeydown);
  } else {
    window.removeEventListener('click', handleWindowClick);
    window.removeEventListener('keydown', handleKeydown);
  }

  onDestroy(() => {
    window.removeEventListener('click', handleWindowClick);
    window.removeEventListener('keydown', handleKeydown);
  });
</script>

{#if visible}
  <ul
    class="menu bg-base-200 rounded-box shadow-lg z-50 fixed text-sm"
    style="left: {x}px; top: {y}px; min-width: 160px;"
  >
    {#each items as item}
      <li class:disabled={item.disabled}>
        <button
          type="button"
          class:text-error={item.danger}
          on:click|stopPropagation={() => handleItem(item)}
          disabled={item.disabled}
        >
          {item.label}
        </button>
      </li>
    {/each}
  </ul>
{/if}
```

- [ ] **Step 2: スモークテスト**

ServerTab.svelte に一時的に追加:

```svelte
<script lang="ts">
  import ContextMenu from '../components/ContextMenu.svelte';
  let menu: ContextMenu;

  function onRowContext(e: MouseEvent) {
    menu.open(e, [
      { label: '編集', onClick: () => console.log('edit') },
      { label: '削除', danger: true, onClick: () => console.log('delete') },
    ]);
  }
</script>

<ContextMenu bind:this={menu} />
<div on:contextmenu={onRowContext} style="padding: 20px; border: 1px solid #ccc;">
  右クリックしてください
</div>
```

`make dev` で起動 → ServerTab で右クリック → メニュー表示 → アイテムクリック / Esc / 外側クリックで閉じる、を確認。

- [ ] **Step 3: スモークテスト除去**

確認後、`ContextMenu` の import / インスタンス / テストボックスを ServerTab から削除。

- [ ] **Step 4: コミット**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git add frontend/src/lib/components/ContextMenu.svelte
git commit -m "feat(frontend): ContextMenu 共通コンポーネント (daisyUI menu) を追加"
```

---

## Task 4: ConfigHandler.PickSongdataDB の Bind 追加

**Files:**
- Modify: `internal/app/handler/config_handler.go`
- Modify: `main.go` (Bind 配列の確認のみ、ConfigHandler は既に Bind 済み)
- Test: `internal/app/handler/config_handler_test.go`

`runtime.OpenFileDialog` を呼ぶ Bind ハンドラを追加。Wails の `OpenDialogOptions` を使ってファイル選択ダイアログを開き、選択された絶対パスを返す。空文字 (キャンセル時) はそのまま返してフロント側で扱う。

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/handler/config_handler_test.go` の末尾に追加。`PickSongdataDB` は Wails ランタイム依存だが、context が nil でない時のみ動作する設計の確認テストを書く:

```go
func TestConfigHandler_PickSongdataDB_NoContext(t *testing.T) {
	t.Parallel()
	uc := buildTestConfigUseCase(t)
	h := handler.NewConfigHandler(uc)
	// SetContext 前は ctx が context.Background() 固定。runtime API は呼ばない契約とする。
	got, err := h.PickSongdataDB()
	assert.NoError(t, err)
	assert.Equal(t, "", got)
}
```

`buildTestConfigUseCase` は既存の test helper (もし存在しなければ既存 `config_handler_test.go` のセットアップを参考に、`persistence.NewConfigStoreSQL` を `:memory:` DB で作って `usecase.NewConfigUseCase` する流れをコピー)。

- [ ] **Step 2: テスト失敗を確認**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
go test ./internal/app/handler/... -run TestConfigHandler_PickSongdataDB_NoContext -v
```

期待: `undefined: handler.ConfigHandler.PickSongdataDB` でコンパイルエラー。

- [ ] **Step 3: PickSongdataDB を実装**

`internal/app/handler/config_handler.go` に追加:

```go
import (
	"context"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// PickSongdataDB はユーザーに songdata.db のパスを OS のファイル選択ダイアログで
// 選ばせ、選ばれた絶対パスを返す。キャンセル時は空文字を返す。
// SetContext 前 (Wails OnStartup 前) に呼ばれた場合はランタイム API を呼ばずに
// 空文字を返す (テスト用のセーフガード)。
func (h *ConfigHandler) PickSongdataDB() (string, error) {
	if h.ctx == nil || h.ctx == context.Background() {
		return "", nil
	}
	return wailsruntime.OpenFileDialog(h.ctx, wailsruntime.OpenDialogOptions{
		Title: "songdata.db を選択",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "SQLite データベース (*.db)", Pattern: "*.db"},
			{DisplayName: "すべてのファイル (*.*)", Pattern: "*"},
		},
	})
}
```

注: 既存の `SetContext` は `h.ctx = ctx` を直接代入していて初期値は `context.Background()`。「未セット」の判別は `ctx == context.Background()` で行う (`ctx == nil` はゼロ値だが `NewConfigHandler` 内で `context.Background()` を入れているのでチェックの両方を入れる安全側)。

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/app/handler/... -run TestConfigHandler_PickSongdataDB_NoContext -v
```

期待: PASS。

- [ ] **Step 5: wails generate module で TS bindings 再生成**

Plan 3 lessons #5 通り、Bind ハンドラのメソッド追加直後に再生成する:

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
wails generate module
```

期待: `frontend/wailsjs/go/handler/ConfigHandler.{d.ts,js}` の `PickSongdataDB` が生成される。生成物確認:

```bash
grep PickSongdataDB frontend/wailsjs/go/handler/ConfigHandler.d.ts
```

- [ ] **Step 6: api.ts にラッパ追加**

`frontend/src/lib/api.ts` の `import` 部に `PickSongdataDB` を追加し、`api` オブジェクトに以下を追加:

```ts
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
  PickSongdataDB,                // ← 追加
} from '../../wailsjs/go/handler/ConfigHandler';

// ... (既存)

export const api = {
  // ... (既存)
  pickSongdataDB(): Promise<string> {
    return PickSongdataDB() as Promise<string>;
  },
  // ...
};
```

- [ ] **Step 7: コミット**

```bash
git add internal/app/handler/config_handler.go internal/app/handler/config_handler_test.go frontend/src/lib/api.ts
git commit -m "feat(config): songdata.db 参照ボタン用に PickSongdataDB を追加"
```

注: `frontend/wailsjs/` は `.gitignore` 対象 (Plan 3 lessons #5)。生成物自体はコミットしない。

---

## Task 5: ServerTab を daisyUI 化 + 参照ボタン + 状態整備

**Files:**
- Modify: `frontend/src/lib/tabs/ServerTab.svelte` (全面書き直し)

既存 ServerTab は素 CSS。Tailwind+daisyUI に書き直し、`songdata.db` パス入力欄に「参照」ボタンを追加、ローディング / エラー状態を整える。

- [ ] **Step 1: 既存 ServerTab.svelte の構造把握**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
cat frontend/src/lib/tabs/ServerTab.svelte
```

主要セクション:
- ポート番号入力
- songdata.db パス入力
- 「保存」ボタン
- サーバステータス (state / port / startedAt / lastError)
- 「起動 / 停止 / 再起動」ボタン
- 所持キャッシュ状態 + 「再読み込み」ボタン

- [ ] **Step 2: ServerTab.svelte 書き直し**

`frontend/src/lib/tabs/ServerTab.svelte` を以下で全置換 (既存ロジックを維持しつつ Tailwind+daisyUI 化、参照ボタン追加、ローディング状態追加):

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type ServerConfig, type ServerStatusDTO, type OwnedCacheStatusDTO } from '../api';

  let cfg: ServerConfig = { port: 50000, songdataDbPath: '' };
  let savedCfg: ServerConfig = { port: 50000, songdataDbPath: '' };
  let status: ServerStatusDTO = { state: 'stopped', port: 0, startedAt: '', lastError: '' };
  let owned: OwnedCacheStatusDTO = { loaded: false, count: 0, loadedAt: '', loadedPath: '', lastError: '' };

  let configLoading = true;
  let saving = false;
  let savingError = '';
  let serverActing = false;
  let ownedLoading = true;
  let ownedReloading = false;

  let unsubServer: (() => void) | null = null;

  onMount(async () => {
    try {
      cfg = await api.getServerConfig();
      savedCfg = { ...cfg };
    } catch (e) {
      savingError = `設定の取得に失敗: ${(e as Error).message}`;
    } finally {
      configLoading = false;
    }
    try {
      status = await api.getServerStatus();
    } catch {
      // status は既定値で続行
    }
    try {
      owned = await api.getOwnedCacheStatus();
    } catch {
      // ignore
    } finally {
      ownedLoading = false;
    }
    unsubServer = api.onServerStatusChanged((s) => {
      status = s;
    });
  });

  onDestroy(() => {
    if (unsubServer) unsubServer();
  });

  $: dirty = cfg.port !== savedCfg.port || cfg.songdataDbPath !== savedCfg.songdataDbPath;

  async function pickPath() {
    try {
      const picked = await api.pickSongdataDB();
      if (picked) {
        cfg.songdataDbPath = picked;
      }
    } catch (e) {
      savingError = `ファイル選択に失敗: ${(e as Error).message}`;
    }
  }

  async function save() {
    saving = true;
    savingError = '';
    try {
      if (cfg.port !== savedCfg.port) {
        await api.setServerPort(cfg.port);
      }
      if (cfg.songdataDbPath !== savedCfg.songdataDbPath) {
        await api.setSongdataDBPath(cfg.songdataDbPath);
      }
      savedCfg = { ...cfg };
      // songdata.db パス変更で所持キャッシュが invalidate されるため再取得
      owned = await api.getOwnedCacheStatus();
    } catch (e) {
      savingError = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  async function startSrv() {
    serverActing = true;
    try {
      await api.startServer();
    } catch (e) {
      // status は server_status:changed イベントで反映されるためここでは握る
      console.warn('start failed', e);
    } finally {
      serverActing = false;
    }
  }
  async function stopSrv() {
    serverActing = true;
    try { await api.stopServer(); } catch (e) { console.warn(e); } finally { serverActing = false; }
  }
  async function restartSrv() {
    serverActing = true;
    try { await api.restartServer(); } catch (e) { console.warn(e); } finally { serverActing = false; }
  }

  async function reloadOwned() {
    ownedReloading = true;
    try {
      await api.reloadOwnedCache();
      owned = await api.getOwnedCacheStatus();
    } catch (e) {
      console.warn('reload owned failed', e);
    } finally {
      ownedReloading = false;
    }
  }

  function formatJST(iso: string): string {
    if (!iso) return '-';
    try {
      return new Date(iso).toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo', hour12: false });
    } catch {
      return iso;
    }
  }
</script>

<section class="p-4 space-y-6">
  <!-- 設定 -->
  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      <h2 class="card-title text-base">設定</h2>
      {#if configLoading}
        <div class="flex items-center gap-2 text-sm">
          <span class="loading loading-spinner loading-sm"></span>
          <span>読み込み中…</span>
        </div>
      {:else}
        <label class="form-control">
          <div class="label"><span class="label-text">ポート番号</span></div>
          <input
            class="input input-bordered input-sm w-40"
            type="number"
            min="1"
            max="65535"
            bind:value={cfg.port}
          />
        </label>

        <label class="form-control">
          <div class="label"><span class="label-text">songdata.db のパス</span></div>
          <div class="join w-full">
            <input class="input input-bordered input-sm join-item flex-1" type="text" bind:value={cfg.songdataDbPath} />
            <button class="btn btn-sm join-item" type="button" on:click={pickPath}>参照…</button>
          </div>
        </label>

        <div class="card-actions justify-end mt-2">
          <button class="btn btn-primary btn-sm" disabled={!dirty || saving} on:click={save}>
            {#if saving}<span class="loading loading-spinner loading-xs"></span>{/if}
            保存
          </button>
        </div>

        {#if savingError}
          <div class="alert alert-error mt-2 text-sm">{savingError}</div>
        {/if}
      {/if}
    </div>
  </div>

  <!-- サーバ -->
  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      <h2 class="card-title text-base">HTTP サーバ</h2>
      <div class="flex items-center gap-2 text-sm">
        <span>状態:</span>
        {#if status.state === 'running'}
          <span class="badge badge-success">稼働中</span>
          <span class="text-xs opacity-70">port {status.port} / 起動 {formatJST(status.startedAt)}</span>
        {:else if status.state === 'error'}
          <span class="badge badge-error">エラー</span>
        {:else}
          <span class="badge">停止中</span>
        {/if}
      </div>
      {#if status.state === 'error' && status.lastError}
        <div class="alert alert-error text-sm mt-2 whitespace-pre-line">{status.lastError}</div>
      {/if}
      <div class="card-actions justify-end mt-2">
        <button class="btn btn-sm" disabled={serverActing || status.state === 'running'} on:click={startSrv}>起動</button>
        <button class="btn btn-sm" disabled={serverActing || status.state !== 'running'} on:click={stopSrv}>停止</button>
        <button class="btn btn-sm" disabled={serverActing} on:click={restartSrv}>再起動</button>
      </div>
    </div>
  </div>

  <!-- 所持キャッシュ -->
  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      <h2 class="card-title text-base">所持キャッシュ</h2>
      {#if ownedLoading}
        <div class="flex items-center gap-2 text-sm">
          <span class="loading loading-spinner loading-sm"></span>
          <span>読み込み中…</span>
        </div>
      {:else}
        <div class="text-sm space-y-1">
          <div>状態: {owned.loaded ? `読み込み済み (${owned.count} 件)` : '未読み込み'}</div>
          <div class="text-xs opacity-70">パス: {owned.loadedPath || '(未設定)'} </div>
          <div class="text-xs opacity-70">最終読み込み: {formatJST(owned.loadedAt)}</div>
          {#if owned.lastError}
            <div class="alert alert-warning text-xs">{owned.lastError}</div>
          {/if}
        </div>
        <div class="card-actions justify-end mt-2">
          <button class="btn btn-sm" disabled={ownedReloading} on:click={reloadOwned}>
            {#if ownedReloading}<span class="loading loading-spinner loading-xs"></span>{/if}
            再読み込み
          </button>
        </div>
      {/if}
    </div>
  </div>
</section>
```

- [ ] **Step 3: 動作確認**

```bash
make dev
```

確認:
- ポート / songdata.db 入力 → 保存 → 反映
- 「参照…」ボタンで OS ファイル選択ダイアログが開く
- 起動 / 停止 / 再起動が動く
- 所持キャッシュの「再読み込み」が動く
- ローディング中にスピナーが出る
- 状態が `error` のとき alert-error が出る

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/tabs/ServerTab.svelte
git commit -m "feat(frontend): ServerTab を daisyUI 化、songdata.db 参照ボタンと状態整備を追加"
```

---

## Task 6: SourceTablesTab を daisyUI 化 + ConfirmDialog + ContextMenu + 状態整備

**Files:**
- Modify: `frontend/src/lib/tabs/SourceTablesTab.svelte` (全面書き直し)

既存 SourceTablesTab は素 CSS で、削除ボタンが Plan 2 lessons #2 の `window.confirm` 不可問題で「確認なし即削除」になっている。`ConfirmDialog` で確認付きに戻し、行右クリックで `ContextMenu` を出す。

- [ ] **Step 1: 既存 SourceTablesTab の機能把握**

```bash
cat frontend/src/lib/tabs/SourceTablesTab.svelte
```

主要機能: 一覧表示 / 追加フォーム / 個別更新 / 一括更新 / 表示名編集 / 削除。

- [ ] **Step 2: SourceTablesTab.svelte 書き直し**

`frontend/src/lib/tabs/SourceTablesTab.svelte` を以下で全置換:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type SourceTableDTO } from '../api';
  import { confirm } from '../components/confirm';
  import ContextMenu, { type MenuItem } from '../components/ContextMenu.svelte';

  let rows: SourceTableDTO[] = [];
  let loading = true;
  let listError = '';

  let newUrl = '';
  let adding = false;
  let addError = '';

  let refreshingAll = false;
  let refreshingId: string | null = null;

  let menu: ContextMenu;
  let unsubRefreshAll: (() => void) | null = null;

  onMount(async () => {
    await reload();
    unsubRefreshAll = api.onSourceTableRefreshAllDone(async () => {
      await reload();
    });
  });

  onDestroy(() => {
    if (unsubRefreshAll) unsubRefreshAll();
  });

  async function reload() {
    loading = true;
    listError = '';
    try {
      rows = await api.listSourceTables();
    } catch (e) {
      listError = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function add() {
    if (!newUrl.trim()) {
      addError = 'URL を入力してください';
      return;
    }
    adding = true;
    addError = '';
    try {
      await api.addSourceTable({ url: newUrl.trim() });
      newUrl = '';
      await reload();
      // 追加直後にバックグラウンド更新を発火
      void api.refreshAllSourceTables();
    } catch (e) {
      addError = (e as Error).message;
    } finally {
      adding = false;
    }
  }

  async function refreshAll() {
    refreshingAll = true;
    try {
      await api.refreshAllSourceTables();
      await reload();
    } finally {
      refreshingAll = false;
    }
  }

  async function refreshOne(id: string) {
    refreshingId = id;
    try {
      await api.refreshSourceTable(id);
      await reload();
    } finally {
      refreshingId = null;
    }
  }

  async function rename(row: SourceTableDTO, newName: string) {
    try {
      await api.updateSourceTableDisplayName(row.id, newName);
      row.displayName = newName;
    } catch (e) {
      console.warn('rename failed', e);
    }
  }

  function handleNameInput(row: SourceTableDTO, e: Event) {
    const t = e.currentTarget as HTMLInputElement;
    rename(row, t.value);
  }

  async function remove(row: SourceTableDTO) {
    const ok = await confirm({
      title: 'ソース表を削除',
      message: `「${row.displayName || row.name || row.inputUrl}」を削除します。\n紐付く公開表も削除されます。続行しますか？`,
      confirmLabel: '削除',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.deleteSourceTable(row.id);
      await reload();
    } catch (e) {
      console.warn('delete failed', e);
    }
  }

  function onRowContextMenu(e: MouseEvent, row: SourceTableDTO) {
    const items: MenuItem[] = [
      { label: '再取得', onClick: () => void refreshOne(row.id), disabled: refreshingId === row.id },
      { label: '削除', danger: true, onClick: () => void remove(row) },
    ];
    menu.open(e, items);
  }

  function statusBadge(s: SourceTableDTO): { cls: string; label: string } {
    if (s.lastFetchStatus === 'ok') return { cls: 'badge-success', label: 'OK' };
    if (s.lastFetchStatus === 'error') return { cls: 'badge-error', label: 'エラー' };
    return { cls: 'badge-ghost', label: '未取得' };
  }

  function formatJST(iso: string): string {
    if (!iso) return '-';
    try {
      return new Date(iso).toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo', hour12: false });
    } catch {
      return iso;
    }
  }
</script>

<section class="p-4 space-y-4">
  <!-- 追加 -->
  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      <h2 class="card-title text-base">ソース表を追加</h2>
      <div class="join">
        <input
          class="input input-bordered input-sm join-item flex-1"
          type="text"
          placeholder="HTML or header.json の URL"
          bind:value={newUrl}
          on:keydown={(e) => e.key === 'Enter' && add()}
        />
        <button class="btn btn-primary btn-sm join-item" disabled={adding} on:click={add}>
          {#if adding}<span class="loading loading-spinner loading-xs"></span>{/if}
          追加
        </button>
      </div>
      {#if addError}<div class="alert alert-error text-sm">{addError}</div>{/if}
    </div>
  </div>

  <!-- 一覧 -->
  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      <div class="flex items-center justify-between">
        <h2 class="card-title text-base">登録済みソース表</h2>
        <button class="btn btn-sm" disabled={refreshingAll} on:click={refreshAll}>
          {#if refreshingAll}<span class="loading loading-spinner loading-xs"></span>{/if}
          一括再取得
        </button>
      </div>

      {#if loading}
        <div class="flex items-center gap-2 text-sm py-4">
          <span class="loading loading-spinner loading-sm"></span>
          <span>読み込み中…</span>
        </div>
      {:else if listError}
        <div class="alert alert-error text-sm">{listError}</div>
      {:else if rows.length === 0}
        <div class="text-sm opacity-70 py-4">ソース表が登録されていません。上の入力欄から URL を追加してください。</div>
      {:else}
        <div class="overflow-x-auto">
          <table class="table table-sm table-zebra">
            <thead>
              <tr>
                <th>表示名 / Name</th>
                <th>URL</th>
                <th>状態</th>
                <th>最終取得</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row (row.id)}
                {@const sb = statusBadge(row)}
                <tr on:contextmenu={(e) => onRowContextMenu(e, row)}>
                  <td>
                    <input
                      class="input input-bordered input-xs w-full"
                      type="text"
                      value={row.displayName}
                      placeholder={row.name || '(未取得)'}
                      on:change={(e) => handleNameInput(row, e)}
                    />
                    <div class="text-xs opacity-60 mt-1">{row.symbol}</div>
                  </td>
                  <td class="text-xs break-all max-w-xs">{row.inputUrl}</td>
                  <td>
                    <span class="badge {sb.cls}">{sb.label}</span>
                    {#if row.lastFetchError}
                      <div class="text-xs text-error mt-1 whitespace-pre-line">{row.lastFetchError}</div>
                    {/if}
                  </td>
                  <td class="text-xs">{formatJST(row.lastFetchedAt)}</td>
                  <td class="whitespace-nowrap">
                    <button
                      class="btn btn-xs"
                      disabled={refreshingId === row.id}
                      on:click={() => refreshOne(row.id)}
                    >
                      {#if refreshingId === row.id}<span class="loading loading-spinner loading-xs"></span>{/if}
                      再取得
                    </button>
                    <button class="btn btn-xs btn-error btn-outline" on:click={() => remove(row)}>削除</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</section>

<ContextMenu bind:this={menu} />
```

- [ ] **Step 3: 動作確認**

```bash
make dev
```

確認:
- 一覧表示 / 追加 / 編集 / 一括/個別再取得が動く
- 削除で ConfirmDialog が出る → 「キャンセル」で削除されない / 「削除」で削除される
- 行右クリックでコンテキストメニュー (再取得 / 削除) が出る
- 0 件状態でプレースホルダ
- ローディング中にスピナー
- `last_fetch_status='error'` の行に `badge-error`、エラー本文も表示

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/tabs/SourceTablesTab.svelte
git commit -m "feat(frontend): SourceTablesTab を daisyUI 化、ConfirmDialog/ContextMenu と状態整備を追加"
```

---

## Task 7: PublishedTablesTab を daisyUI 化 + ConfirmDialog + ContextMenu + 状態整備

**Files:**
- Modify: `frontend/src/lib/tabs/PublishedTablesTab.svelte` (全面書き直し)

既存と同じく素 CSS。Tailwind+daisyUI 化、削除に ConfirmDialog、行右クリックでコンテキストメニュー (編集 / 削除 / 再ピック (manual のみ) / ブラウザで開く)。

- [ ] **Step 1: 既存 PublishedTablesTab の機能把握**

```bash
cat frontend/src/lib/tabs/PublishedTablesTab.svelte
```

主要機能: 一覧 / 作成フォーム / 編集モーダル or インライン / 削除 / slug 検証 / slug 自動提案 / 「開く」 / 「再ピック」 (manual mode)。

- [ ] **Step 2: PublishedTablesTab.svelte 書き直し**

このタスクは規模が大きい。既存 312 行を Tailwind+daisyUI で書き直すが、既存ロジック (slug 検証 / 自動提案 / 編集 / 作成) を維持する。以下のテンプレを基に既存 .svelte の `<script>` から関数群をコピーして使う:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PublishedTableDTO, type SourceTableDTO, type RefreshMode, type ServerConfig } from '../api';
  import { confirm } from '../components/confirm';
  import ContextMenu, { type MenuItem } from '../components/ContextMenu.svelte';

  type FormMode = 'create' | { kind: 'edit'; id: string };

  let rows: PublishedTableDTO[] = [];
  let sources: SourceTableDTO[] = [];
  let loading = true;
  let listError = '';

  let formMode: FormMode = 'create';
  let formOpen = false;
  let form = {
    slug: '',
    displayName: '',
    symbol: '',
    sourceTableId: '',
    ownedOnly: false,
    pickPerLevel: 0,
    refreshMode: 'per_request' as RefreshMode,
    sortOrder: 0,
  };
  let formError = '';
  let saving = false;
  let slugStatus: 'idle' | 'ok' | 'invalid_format' | 'reserved' | 'duplicate' = 'idle';
  let slugDirty = false;

  let serverPort = 50000;

  let menu: ContextMenu;

  onMount(async () => {
    await Promise.all([reload(), loadSources(), loadServerCfg()]);
  });

  async function reload() {
    loading = true;
    listError = '';
    try {
      rows = await api.listPublishedTables();
    } catch (e) {
      listError = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function loadSources() {
    try { sources = await api.listSourceTables(); } catch (e) { console.warn(e); }
  }

  async function loadServerCfg() {
    try {
      const cfg: ServerConfig = await api.getServerConfig();
      serverPort = cfg.port;
    } catch (e) { console.warn(e); }
  }

  function openCreate() {
    formMode = 'create';
    form = {
      slug: '', displayName: '', symbol: '', sourceTableId: sources[0]?.id ?? '',
      ownedOnly: false, pickPerLevel: 0, refreshMode: 'per_request', sortOrder: 0,
    };
    slugStatus = 'idle';
    slugDirty = false;
    formError = '';
    formOpen = true;
  }

  function openEdit(row: PublishedTableDTO) {
    formMode = { kind: 'edit', id: row.id };
    form = { ...row };
    slugStatus = 'idle';
    slugDirty = true;  // 既存 slug は valid とみなす
    formError = '';
    formOpen = true;
  }

  function closeForm() {
    formOpen = false;
  }

  async function suggestSlug() {
    if (!form.sourceTableId) return;
    try {
      const s = await api.suggestSlugFromSource(form.sourceTableId);
      form.slug = s;
      slugDirty = true;
      await checkSlug();
    } catch (e) { console.warn(e); }
  }

  async function checkSlug() {
    if (!form.slug) { slugStatus = 'idle'; return; }
    try {
      const excludeId = formMode === 'create' ? '' : formMode.id;
      const v = await api.validateSlug(form.slug, excludeId);
      if (v.ok) { slugStatus = 'ok'; }
      else { slugStatus = (v.reason as typeof slugStatus); }
    } catch (e) {
      slugStatus = 'invalid_format';
    }
  }

  async function save() {
    if (!form.displayName.trim()) { formError = '表示名は必須です'; return; }
    if (!form.sourceTableId) { formError = 'ソース表を選択してください'; return; }
    if (slugStatus !== 'ok') { formError = 'slug が不正です'; return; }
    saving = true;
    formError = '';
    try {
      if (formMode === 'create') {
        await api.createPublishedTable({
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          sourceTableId: form.sourceTableId,
          ownedOnly: form.ownedOnly,
          pickPerLevel: form.pickPerLevel,
          refreshMode: form.refreshMode,
        });
      } else {
        await api.updatePublishedTable({
          id: formMode.id,
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          sourceTableId: form.sourceTableId,
          ownedOnly: form.ownedOnly,
          pickPerLevel: form.pickPerLevel,
          refreshMode: form.refreshMode,
          sortOrder: form.sortOrder,
        });
      }
      formOpen = false;
      await reload();
    } catch (e) {
      formError = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  async function remove(row: PublishedTableDTO) {
    const ok = await confirm({
      title: '公開表を削除',
      message: `公開表「${row.displayName}」(slug: ${row.slug}) を削除します。続行しますか？`,
      confirmLabel: '削除',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.deletePublishedTable(row.id);
      await reload();
    } catch (e) { console.warn(e); }
  }

  async function openInBrowser(row: PublishedTableDTO) {
    try { await api.openPublishedTableURL(row.slug, serverPort); } catch (e) { console.warn(e); }
  }

  async function manualRefresh(row: PublishedTableDTO) {
    try { await api.manualRefreshPick(row.id); } catch (e) { console.warn(e); }
  }

  function onRowContextMenu(e: MouseEvent, row: PublishedTableDTO) {
    const items: MenuItem[] = [
      { label: '編集', onClick: () => openEdit(row) },
      { label: 'ブラウザで開く', onClick: () => void openInBrowser(row) },
      { label: '再ピック', disabled: row.refreshMode !== 'manual', onClick: () => void manualRefresh(row) },
      { label: '削除', danger: true, onClick: () => void remove(row) },
    ];
    menu.open(e, items);
  }

  function modeLabel(m: RefreshMode): string {
    return m === 'per_request' ? 'リクエスト毎' : m === 'daily' ? '日次' : '手動';
  }
</script>

<section class="p-4 space-y-4">
  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold">公開表</h2>
    <button class="btn btn-primary btn-sm" on:click={openCreate} disabled={sources.length === 0}>新規作成</button>
  </div>

  {#if sources.length === 0}
    <div class="alert alert-warning text-sm">公開表を作成するには、まず「ソース表」タブでソース表を 1 つ以上登録してください。</div>
  {/if}

  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      {#if loading}
        <div class="flex items-center gap-2 text-sm py-4">
          <span class="loading loading-spinner loading-sm"></span>
          <span>読み込み中…</span>
        </div>
      {:else if listError}
        <div class="alert alert-error text-sm">{listError}</div>
      {:else if rows.length === 0}
        <div class="text-sm opacity-70 py-4">公開表がまだ作成されていません。「新規作成」から始めてください。</div>
      {:else}
        <div class="overflow-x-auto">
          <table class="table table-sm table-zebra">
            <thead>
              <tr>
                <th>表示名</th>
                <th>slug</th>
                <th>シンボル</th>
                <th>ソース表</th>
                <th>所持絞り込み</th>
                <th>件数/レベル</th>
                <th>更新</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row (row.id)}
                {@const src = sources.find((s) => s.id === row.sourceTableId)}
                <tr on:contextmenu={(e) => onRowContextMenu(e, row)}>
                  <td>{row.displayName}</td>
                  <td class="font-mono text-xs">{row.slug}</td>
                  <td>{row.symbol}</td>
                  <td class="text-xs">{src?.displayName || src?.name || '(削除済み)'}</td>
                  <td>{row.ownedOnly ? '有' : '無'}</td>
                  <td>{row.pickPerLevel === 0 ? '無制限' : row.pickPerLevel}</td>
                  <td><span class="badge badge-ghost">{modeLabel(row.refreshMode)}</span></td>
                  <td class="whitespace-nowrap">
                    <button class="btn btn-xs" on:click={() => openInBrowser(row)}>開く</button>
                    <button class="btn btn-xs" on:click={() => openEdit(row)}>編集</button>
                    <button class="btn btn-xs btn-error btn-outline" on:click={() => remove(row)}>削除</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</section>

<!-- 作成 / 編集モーダル -->
{#if formOpen}
  <dialog class="modal modal-open">
    <div class="modal-box max-w-2xl">
      <h3 class="font-bold text-base">{formMode === 'create' ? '新規公開表' : '公開表を編集'}</h3>
      <div class="space-y-2 mt-2 text-sm">
        <label class="form-control">
          <div class="label py-1"><span class="label-text">表示名</span></div>
          <input class="input input-bordered input-sm" bind:value={form.displayName} />
        </label>
        <label class="form-control">
          <div class="label py-1"><span class="label-text">slug (URL に使われる)</span></div>
          <div class="join w-full">
            <input
              class="input input-bordered input-sm join-item flex-1"
              bind:value={form.slug}
              on:input={() => { slugDirty = true; void checkSlug(); }}
            />
            <button class="btn btn-sm join-item" type="button" on:click={suggestSlug}>自動生成</button>
          </div>
          {#if slugDirty && slugStatus !== 'idle' && slugStatus !== 'ok'}
            <div class="text-xs text-error mt-1">
              {slugStatus === 'invalid_format' ? '形式が不正 (a-z, 0-9, ハイフンのみ、先頭は英数字、最大63文字)' :
               slugStatus === 'reserved' ? '予約語です' :
               slugStatus === 'duplicate' ? '既に使われています' : ''}
            </div>
          {/if}
        </label>
        <label class="form-control">
          <div class="label py-1"><span class="label-text">シンボル (例: ★, ▲)</span></div>
          <input class="input input-bordered input-sm w-32" bind:value={form.symbol} />
        </label>
        <label class="form-control">
          <div class="label py-1"><span class="label-text">ソース表</span></div>
          <select class="select select-bordered select-sm" bind:value={form.sourceTableId}>
            {#each sources as s}
              <option value={s.id}>{s.displayName || s.name || s.inputUrl}</option>
            {/each}
          </select>
        </label>
        <label class="label cursor-pointer justify-start gap-3">
          <input type="checkbox" class="checkbox checkbox-sm" bind:checked={form.ownedOnly} />
          <span class="label-text">所持譜面のみ表示</span>
        </label>
        <label class="form-control">
          <div class="label py-1"><span class="label-text">レベル毎の件数 (0=無制限)</span></div>
          <input class="input input-bordered input-sm w-32" type="number" min="0" bind:value={form.pickPerLevel} />
        </label>
        <label class="form-control">
          <div class="label py-1"><span class="label-text">更新モード</span></div>
          <select class="select select-bordered select-sm" bind:value={form.refreshMode}>
            <option value="per_request">リクエスト毎 (毎回再生成)</option>
            <option value="daily">日次 (同一日付内で固定)</option>
            <option value="manual">手動 (再ピックボタンまで固定)</option>
          </select>
        </label>
      </div>
      {#if formError}<div class="alert alert-error text-sm mt-3">{formError}</div>{/if}
      <div class="modal-action">
        <button class="btn btn-sm" on:click={closeForm}>キャンセル</button>
        <button class="btn btn-primary btn-sm" disabled={saving} on:click={save}>
          {#if saving}<span class="loading loading-spinner loading-xs"></span>{/if}
          保存
        </button>
      </div>
    </div>
  </dialog>
{/if}

<ContextMenu bind:this={menu} />
```

- [ ] **Step 3: 動作確認**

```bash
make dev
```

確認:
- 一覧 / 新規作成 / 編集 / 削除 / 開く / 再ピック (manual) が動く
- 削除で ConfirmDialog が出る
- 行右クリックでコンテキストメニュー (4 項目)、`refresh_mode != manual` の行で「再ピック」が disabled
- ソース表 0 件のとき新規作成ボタンが disabled、警告 alert が出る
- 0 件状態 / ローディング状態 / エラー状態が出る

- [ ] **Step 4: コミット**

```bash
git add frontend/src/lib/tabs/PublishedTablesTab.svelte
git commit -m "feat(frontend): PublishedTablesTab を daisyUI 化、ConfirmDialog/ContextMenu と状態整備を追加"
```

---

## Task 8: App.svelte を daisyUI tabs-boxed + Dashboard タブ追加 (空)

**Files:**
- Modify: `frontend/src/App.svelte`

タブ切替を daisyUI `tabs tabs-boxed` で書き直し、4 番目のタブ「ダッシュボード」を追加 (中身は次タスクで実装、現時点ではプレースホルダ)。

- [ ] **Step 1: App.svelte 書き直し**

`frontend/src/App.svelte` を以下で全置換:

```svelte
<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
  import SourceTablesTab from './lib/tabs/SourceTablesTab.svelte';
  import PublishedTablesTab from './lib/tabs/PublishedTablesTab.svelte';
  import DashboardTab from './lib/tabs/DashboardTab.svelte';

  type TabKey = 'server' | 'source-tables' | 'published-tables' | 'dashboard';
  let active: TabKey = 'server';
</script>

<main class="min-h-screen bg-base-200 text-base-content">
  <header class="bg-base-100 border-b border-base-300">
    <div class="px-4 py-2">
      <h1 class="text-base font-semibold">BMS Random Table Compositor</h1>
    </div>
    <nav role="tablist" class="tabs tabs-bordered px-4">
      <button
        role="tab"
        class="tab"
        class:tab-active={active === 'server'}
        on:click={() => (active = 'server')}>サーバ設定</button>
      <button
        role="tab"
        class="tab"
        class:tab-active={active === 'source-tables'}
        on:click={() => (active = 'source-tables')}>ソース表</button>
      <button
        role="tab"
        class="tab"
        class:tab-active={active === 'published-tables'}
        on:click={() => (active = 'published-tables')}>公開表</button>
      <button
        role="tab"
        class="tab"
        class:tab-active={active === 'dashboard'}
        on:click={() => (active = 'dashboard')}>ダッシュボード</button>
    </nav>
  </header>
  {#if active === 'server'}<ServerTab />
  {:else if active === 'source-tables'}<SourceTablesTab />
  {:else if active === 'published-tables'}<PublishedTablesTab />
  {:else if active === 'dashboard'}<DashboardTab />
  {/if}
</main>
```

- [ ] **Step 2: DashboardTab スタブ作成**

`frontend/src/lib/tabs/DashboardTab.svelte` (本実装は Task 16 で完了):

```svelte
<section class="p-4">
  <div class="alert alert-info text-sm">ダッシュボードは実装中です。</div>
</section>
```

- [ ] **Step 3: 動作確認**

```bash
make dev
```

確認: 4 つのタブが daisyUI tabs として並び、切り替えが動く。Dashboard タブはプレースホルダ表示。

- [ ] **Step 4: コミット**

```bash
git add frontend/src/App.svelte frontend/src/lib/tabs/DashboardTab.svelte
git commit -m "feat(frontend): App を daisyUI tabs 化、ダッシュボードタブ枠を追加"
```

---

## Task 9: PickResultStore に OnChange リスナーを追加

**Files:**
- Modify: `internal/usecase/pick_result_store.go`
- Modify: `internal/usecase/pick_result_store_test.go`

ダッシュボード「現在のピック結果」の自動更新のため、`Set` / `Delete` / `Clear` 時に通知するリスナーを追加する。

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/pick_result_store_test.go` の末尾に追加:

```go
func TestPickResultStore_OnChange_FiresOnSet(t *testing.T) {
	t.Parallel()
	s := usecase.NewPickResultStore()
	var got []string
	s.OnChange(func(publishedID string) { got = append(got, publishedID) })
	s.Set("a", domain.PickResult{PublishedTableID: "a"})
	s.Set("b", domain.PickResult{PublishedTableID: "b"})
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestPickResultStore_OnChange_FiresOnDeleteAndClear(t *testing.T) {
	t.Parallel()
	s := usecase.NewPickResultStore()
	s.Set("a", domain.PickResult{PublishedTableID: "a"})
	s.Set("b", domain.PickResult{PublishedTableID: "b"})
	var got []string
	s.OnChange(func(publishedID string) { got = append(got, publishedID) })
	s.Delete("a")
	s.Clear()
	assert.Equal(t, []string{"a", ""}, got, "delete fires id, clear fires empty string")
}
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/usecase/... -run TestPickResultStore_OnChange -v
```

期待: `undefined: store.OnChange` でコンパイルエラー。

- [ ] **Step 3: PickResultStore に OnChange を実装**

`internal/usecase/pick_result_store.go` を修正:

```go
package usecase

import (
	"sync"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

type PickResultStore struct {
	mu        sync.RWMutex
	m         map[string]domain.PickResult
	listeners []func(publishedID string)
}

func NewPickResultStore() *PickResultStore {
	return &PickResultStore{m: map[string]domain.PickResult{}}
}

// OnChange は Set / Delete / Clear 時に呼ばれる。Clear は publishedID="" で通知。
// 同期的に呼ばれるので、リスナー側は重い処理をしないか自分で goroutine 化する。
func (s *PickResultStore) OnChange(fn func(publishedID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *PickResultStore) notify(publishedID string) {
	// listeners のコピーを取ってからロック解除して呼ぶ (デッドロック回避)
	s.mu.Lock()
	listeners := append(([]func(string))(nil), s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(publishedID)
	}
}

func (s *PickResultStore) Get(publishedID string) (domain.PickResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[publishedID]
	return r, ok
}

func (s *PickResultStore) Set(publishedID string, r domain.PickResult) {
	s.mu.Lock()
	s.m[publishedID] = r
	s.mu.Unlock()
	s.notify(publishedID)
}

func (s *PickResultStore) Delete(publishedID string) {
	s.mu.Lock()
	delete(s.m, publishedID)
	s.mu.Unlock()
	s.notify(publishedID)
}

func (s *PickResultStore) Snapshot() map[string]domain.PickResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.PickResult, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}

func (s *PickResultStore) Clear() {
	s.mu.Lock()
	s.m = map[string]domain.PickResult{}
	s.mu.Unlock()
	s.notify("")
}
```

注: `notify` 内で listeners をコピーしてロック解除後に呼ぶことで、リスナーが Store のメソッドを呼んでもデッドロックしない。

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/usecase/... -run TestPickResultStore_OnChange -v
go test ./internal/usecase/... -v   # 既存テストも確認
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/pick_result_store.go internal/usecase/pick_result_store_test.go
git commit -m "feat(pick): PickResultStore に OnChange リスナーを追加"
```

---

## Task 10: SourceTableUseCase に refreshOne 完了 hook を追加

**Files:**
- Modify: `internal/usecase/source_table_usecase.go`
- Modify: `internal/usecase/source_table_usecase_test.go`

ダッシュボード「ソース表更新履歴」の自動更新のため、`refreshOne` 完了時に通知するリスナーを追加する。成否どちらでも通知。

- [ ] **Step 1: 失敗するテストを書く**

既存 `source_table_usecase_test.go` には `fakeSourceRepo` / `fakeFetcher` / `fakeIDGen` / `newSilentLogger` が既に定義済み。それらを直接使う:

`internal/usecase/source_table_usecase_test.go` の末尾に追加:

```go
import (
	// 既存 import に "github.com/stretchr/testify/assert" を追加 (require は既に入っている)
	"github.com/stretchr/testify/assert"
)

func TestSourceTableUseCase_OnRefreshComplete_FiresOnSuccess(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	uc := usecase.NewSourceTableUseCase(repo, fetcher,
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())

	const u = "https://example.com/sl/table.html"
	fetcher.results[u] = port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "SL", Symbol: "★", LevelOrder: []string{"sl0"},
			DataURL: "https://example.com/sl/data.json",
		},
		Charts: []domain.SourceChart{{Position: 0, MD5: "m1", Level: "sl0", Title: "T"}},
		ETag:   "tag-1",
	}

	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: u})
	require.NoError(t, err)

	var got []usecase.RefreshCompleteEvent
	uc.OnRefreshComplete(func(e usecase.RefreshCompleteEvent) { got = append(got, e) })

	require.NoError(t, uc.RefreshOne(context.Background(), id))
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].SourceID)
	assert.Equal(t, domain.FetchStatusOK, got[0].Status)
	assert.Empty(t, got[0].Error)
}

func TestSourceTableUseCase_OnRefreshComplete_FiresOnError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	uc := usecase.NewSourceTableUseCase(repo, fetcher,
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())

	const u = "https://example.com/sl/table.html"
	fetcher.errs[u] = errors.New("network error")

	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: u})
	require.NoError(t, err)

	var got []usecase.RefreshCompleteEvent
	uc.OnRefreshComplete(func(e usecase.RefreshCompleteEvent) { got = append(got, e) })

	_ = uc.RefreshOne(context.Background(), id) // エラーでも通知
	require.Len(t, got, 1)
	assert.Equal(t, domain.FetchStatusError, got[0].Status)
	assert.Contains(t, got[0].Error, "network error")
}
```

注: `port.FetchedTable` の `Header` フィールド型 (`domain.BMSTableHeader`) や `RefreshOne` のメソッド名は既存実装を参照して微調整する必要がある。確認:

```bash
grep -n "type FetchedTable\|type BMSTableHeader\|func.*RefreshOne\|func.*RefreshAll" \
  /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/port/*.go \
  /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/domain/*.go \
  /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/usecase/source_table_usecase.go
```

実装側のメソッド名 (`RefreshOne` か `refreshOne` か) に合わせて assert する。

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/usecase/... -run TestSourceTableUseCase_OnRefreshComplete -v
```

期待: `undefined: usecase.RefreshCompleteEvent` 等のコンパイルエラー。

- [ ] **Step 3: SourceTableUseCase に hook を追加**

`internal/usecase/source_table_usecase.go` の構造体・コンストラクタ・refreshOne (refreshOne の名前は既存実装に従う、もし `RefreshOne` ならそのまま) に追加:

```go
// RefreshCompleteEvent は refreshOne 完了時にリスナーへ渡されるイベント。
type RefreshCompleteEvent struct {
	SourceID    string
	DisplayName string
	Status      domain.FetchStatus
	Error       string
	At          time.Time
}

// OnRefreshComplete は refreshOne 完了時に呼ばれるリスナーを登録する。
// 成功時 / 失敗時の両方で呼ばれる。
func (u *SourceTableUseCase) OnRefreshComplete(fn func(RefreshCompleteEvent)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.refreshListeners = append(u.refreshListeners, fn)
}
```

`SourceTableUseCase` 構造体に `mu sync.Mutex` と `refreshListeners []func(RefreshCompleteEvent)` を追加 (既に sync.Mutex があれば流用)。

`refreshOne` (or `RefreshOne`) のロジックに、終了直前で以下を呼ぶ:

```go
// refreshOne の最後 (成否どちらの分岐でも呼ばれるよう defer 内で)
defer func() {
	u.mu.Lock()
	listeners := append(([]func(RefreshCompleteEvent))(nil), u.refreshListeners...)
	u.mu.Unlock()
	ev := RefreshCompleteEvent{
		SourceID:    id,
		DisplayName: stCurrent.DisplayName, // 取得後の値を使う
		Status:      finalStatus,           // OK / Error
		Error:       finalError,            // エラーメッセージ
		At:          time.Now(),
	}
	for _, fn := range listeners {
		fn(ev)
	}
}()
```

具体実装は既存の refreshOne の構造に合わせて書く。`finalStatus` / `finalError` は既存ロジックで `last_fetch_status` / `last_fetch_error` を保存している値と一致させる。

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/usecase/... -run TestSourceTableUseCase_OnRefreshComplete -v
go test ./internal/usecase/... -v   # 既存テストも確認
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/source_table_usecase.go internal/usecase/source_table_usecase_test.go
git commit -m "feat(source-table): refreshOne 完了 hook を追加 (ダッシュボード履歴用)"
```

---

## Task 11: 汎用 RingBuffer[T] を追加

**Files:**
- Create: `internal/usecase/ring_buffer.go`
- Create: `internal/usecase/ring_buffer_test.go`

ダッシュボードの「最近のリクエスト 100 件」「ソース表更新履歴」の格納に使う汎用ジェネリック型。`Append` / `Snapshot` (新しい順) のみ提供。

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/ring_buffer_test.go`:

```go
package usecase_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer_AppendAndSnapshot_NewestFirst(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](3)
	rb.Append(1)
	rb.Append(2)
	rb.Append(3)
	assert.Equal(t, []int{3, 2, 1}, rb.Snapshot())
}

func TestRingBuffer_DropsOldestOverCapacity(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](3)
	for i := 1; i <= 5; i++ {
		rb.Append(i)
	}
	// capacity=3 なので最新 3 件 (5,4,3) のみ
	assert.Equal(t, []int{5, 4, 3}, rb.Snapshot())
}

func TestRingBuffer_EmptyReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[string](10)
	assert.Equal(t, []string{}, rb.Snapshot())
}

func TestRingBuffer_ConcurrentAppendIsSafe(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](100)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(start int) {
			for j := 0; j < 10; j++ {
				rb.Append(start*10 + j)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	snap := rb.Snapshot()
	assert.Len(t, snap, 100)
}
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/usecase/... -run TestRingBuffer -v
```

期待: `undefined: usecase.NewRingBuffer` でコンパイルエラー。

- [ ] **Step 3: RingBuffer 実装**

`internal/usecase/ring_buffer.go`:

```go
package usecase

import "sync"

// RingBuffer は容量上限付きの単純なリングバッファ。スレッドセーフ。
// Snapshot は新しい順にコピーした slice を返す (元の格納順は古→新)。
type RingBuffer[T any] struct {
	mu   sync.RWMutex
	cap  int
	data []T
}

// NewRingBuffer は capacity 件の容量を持つリングバッファを作る。
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer[T]{cap: capacity, data: make([]T, 0, capacity)}
}

// Append は要素を追加する。容量超過時は最古の要素が捨てられる。
func (r *RingBuffer[T]) Append(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) >= r.cap {
		r.data = r.data[1:]
	}
	r.data = append(r.data, v)
}

// Snapshot は現在の格納要素を新しい順 (新→古) にコピーして返す。
// 結果スライスは呼び出し側が自由に変更してよい。
func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, len(r.data))
	for i, v := range r.data {
		out[len(r.data)-1-i] = v
	}
	return out
}
```

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/usecase/... -run TestRingBuffer -v
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/ring_buffer.go internal/usecase/ring_buffer_test.go
git commit -m "feat(usecase): 汎用 RingBuffer[T] を追加 (ダッシュボード用)"
```

---

## Task 12: domain.RequestLogEntry / FetchLogEntry / DashboardSnapshot

**Files:**
- Create: `internal/domain/dashboard.go`

ダッシュボード関連の domain 型を集約。

- [ ] **Step 1: domain 型を書く**

`internal/domain/dashboard.go`:

```go
package domain

import "time"

// RequestLogEntry はダッシュボードに表示する 1 件の HTTP リクエスト履歴。
type RequestLogEntry struct {
	At         time.Time
	Method     string
	Path       string
	Slug       string // パースできれば slug、できなければ空
	StatusCode int
	DurationMs int64
}

// FetchLogEntry はダッシュボードに表示する 1 件のソース表取得履歴。
type FetchLogEntry struct {
	At          time.Time
	SourceID    string
	DisplayName string
	Status      FetchStatus
	Error       string
}

// PickSnapshotEntry はダッシュボードに表示するピック結果サマリ 1 件。
type PickSnapshotEntry struct {
	PublishedID string
	GeneratedAt time.Time
	LevelOrder  []string
	LevelCounts map[string]int
	TotalCount  int
}

// DashboardSnapshot は DashboardUseCase.Snapshot が返す全データ。
type DashboardSnapshot struct {
	Requests []RequestLogEntry
	Fetches  []FetchLogEntry
	Picks    []PickSnapshotEntry
}
```

- [ ] **Step 2: コンパイル確認**

```bash
go build ./...
```

期待: エラーなし。

- [ ] **Step 3: コミット**

```bash
git add internal/domain/dashboard.go
git commit -m "feat(domain): ダッシュボード用 RequestLogEntry / FetchLogEntry / DashboardSnapshot を追加"
```

---

## Task 13: DashboardUseCase 実装

**Files:**
- Create: `internal/usecase/dashboard_usecase.go`
- Create: `internal/usecase/dashboard_usecase_test.go`

`RingBuffer` で 100 件ずつのリクエスト / フェッチ履歴を保持。`AppendRequest` / `AppendFetch` で追加、`Snapshot` で全データを返す。`PickResultStore.Snapshot` を流用してピックも合算する。

- [ ] **Step 1: 失敗するテストを書く**

`internal/usecase/dashboard_usecase_test.go`:

```go
package usecase_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestDashboardUseCase_AppendAndSnapshot(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)

	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	d.AppendRequest(domain.RequestLogEntry{At: now, Method: "GET", Path: "/sl-random", StatusCode: 200, DurationMs: 12, Slug: "sl-random"})
	d.AppendRequest(domain.RequestLogEntry{At: now.Add(time.Second), Method: "GET", Path: "/sl-random/data.json", StatusCode: 200, DurationMs: 4, Slug: "sl-random"})
	d.AppendFetch(domain.FetchLogEntry{At: now, SourceID: "src1", DisplayName: "Satellite", Status: domain.FetchStatusOK})

	pickStore.Set("pub1", domain.PickResult{
		PublishedTableID: "pub1",
		GeneratedAt:      now,
		LevelOrder:       []string{"sl0", "sl1"},
		Charts: []domain.SourceChart{
			{Level: "sl0"}, {Level: "sl0"}, {Level: "sl1"},
		},
	})

	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 2)
	assert.Equal(t, "/sl-random/data.json", snap.Requests[0].Path, "newest first")
	assert.Len(t, snap.Fetches, 1)
	assert.Len(t, snap.Picks, 1)
	assert.Equal(t, "pub1", snap.Picks[0].PublishedID)
	assert.Equal(t, 3, snap.Picks[0].TotalCount)
	assert.Equal(t, map[string]int{"sl0": 2, "sl1": 1}, snap.Picks[0].LevelCounts)
}

func TestDashboardUseCase_RingBufferCapacity(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	for i := 0; i < 150; i++ {
		d.AppendRequest(domain.RequestLogEntry{Method: "GET", Path: "/x", StatusCode: 200})
	}
	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 100, "capped at 100")
}
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/usecase/... -run TestDashboardUseCase -v
```

期待: `undefined: usecase.NewDashboardUseCase`。

- [ ] **Step 3: DashboardUseCase 実装**

`internal/usecase/dashboard_usecase.go`:

```go
package usecase

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

const (
	dashboardRequestsCap = 100
	dashboardFetchesCap  = 100
)

// DashboardUseCase はダッシュボード表示用の最近のリクエスト / ソース表更新履歴を
// メモリリングバッファで保持し、ピック結果は PickResultStore から取得する。
type DashboardUseCase struct {
	requests  *RingBuffer[domain.RequestLogEntry]
	fetches   *RingBuffer[domain.FetchLogEntry]
	pickStore *PickResultStore

	requestListeners []func(domain.RequestLogEntry)
	fetchListeners   []func(domain.FetchLogEntry)
	pickListeners    []func(publishedID string)
}

// NewDashboardUseCase は新しい DashboardUseCase を作る。
func NewDashboardUseCase(pickStore *PickResultStore) *DashboardUseCase {
	return &DashboardUseCase{
		requests:  NewRingBuffer[domain.RequestLogEntry](dashboardRequestsCap),
		fetches:   NewRingBuffer[domain.FetchLogEntry](dashboardFetchesCap),
		pickStore: pickStore,
	}
}

// AppendRequest はリクエスト履歴を 1 件追加する。
func (d *DashboardUseCase) AppendRequest(e domain.RequestLogEntry) {
	d.requests.Append(e)
	for _, fn := range d.requestListeners {
		fn(e)
	}
}

// AppendFetch はソース表更新履歴を 1 件追加する。
func (d *DashboardUseCase) AppendFetch(e domain.FetchLogEntry) {
	d.fetches.Append(e)
	for _, fn := range d.fetchListeners {
		fn(e)
	}
}

// NotifyPickChanged は PickResultStore の OnChange から呼ばれる。
// イベント転送のみ行う (実体は PickResultStore が持つ)。
func (d *DashboardUseCase) NotifyPickChanged(publishedID string) {
	for _, fn := range d.pickListeners {
		fn(publishedID)
	}
}

// OnRequest は AppendRequest のたびに呼ばれるリスナーを登録する。
func (d *DashboardUseCase) OnRequest(fn func(domain.RequestLogEntry)) {
	d.requestListeners = append(d.requestListeners, fn)
}

// OnFetch は AppendFetch のたびに呼ばれるリスナーを登録する。
func (d *DashboardUseCase) OnFetch(fn func(domain.FetchLogEntry)) {
	d.fetchListeners = append(d.fetchListeners, fn)
}

// OnPickChanged は NotifyPickChanged 経由で発火するリスナーを登録する。
func (d *DashboardUseCase) OnPickChanged(fn func(publishedID string)) {
	d.pickListeners = append(d.pickListeners, fn)
}

// Snapshot は現在の全データを返す。
func (d *DashboardUseCase) Snapshot() domain.DashboardSnapshot {
	picks := d.snapshotPicks()
	return domain.DashboardSnapshot{
		Requests: d.requests.Snapshot(),
		Fetches:  d.fetches.Snapshot(),
		Picks:    picks,
	}
}

func (d *DashboardUseCase) snapshotPicks() []domain.PickSnapshotEntry {
	raw := d.pickStore.Snapshot()
	out := make([]domain.PickSnapshotEntry, 0, len(raw))
	for id, r := range raw {
		counts := map[string]int{}
		for _, c := range r.Charts {
			counts[c.Level]++
		}
		out = append(out, domain.PickSnapshotEntry{
			PublishedID: id,
			GeneratedAt: r.GeneratedAt,
			LevelOrder:  r.LevelOrder,
			LevelCounts: counts,
			TotalCount:  len(r.Charts),
		})
	}
	return out
}
```

注: `domain.PickResult.LevelOrder` は Plan 3 で既に追加済み (`internal/domain/pick_result.go`)。本タスクはそれを参照するだけ。

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/usecase/... -run TestDashboardUseCase -v
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/usecase/dashboard_usecase.go internal/usecase/dashboard_usecase_test.go
git commit -m "feat(usecase): DashboardUseCase を追加 (リクエスト/フェッチ/ピック集約)"
```

---

## Task 14: HTTP middleware (リクエストログ)

**Files:**
- Create: `internal/adapter/httpserver/middleware.go`
- Create: `internal/adapter/httpserver/middleware_test.go`
- Modify: `internal/adapter/httpserver/server.go` (Deps に Dashboard 追加)
- Modify: `internal/adapter/httpserver/router.go` (middleware 適用)

リクエストごとに `DashboardUseCase.AppendRequest` を呼ぶミドルウェア。`http.ResponseWriter` をラップしてステータスコードを取得、`time.Since` で経過時間。`Path` の先頭セグメントを slug として抽出。

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/httpserver/middleware_test.go`:

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestLoggingMiddleware_AppendsRequest(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("teapot"))
	})
	wrapped := httpserver.LoggingMiddleware(d)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sl-random/header.json", nil)
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)

	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 1)
	assert.Equal(t, "/sl-random/header.json", snap.Requests[0].Path)
	assert.Equal(t, "GET", snap.Requests[0].Method)
	assert.Equal(t, http.StatusTeapot, snap.Requests[0].StatusCode)
	assert.Equal(t, "sl-random", snap.Requests[0].Slug)
}

func TestLoggingMiddleware_RootPathHasEmptySlug(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	wrapped := httpserver.LoggingMiddleware(d)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 1)
	assert.Equal(t, "", snap.Requests[0].Slug)
}

func TestLoggingMiddleware_DefaultStatus200WhenNotWritten(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	wrapped := httpserver.LoggingMiddleware(d)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/sl/data.json", nil))
	snap := d.Snapshot()
	assert.Equal(t, 200, snap.Requests[0].StatusCode)
}
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/adapter/httpserver/... -run TestLoggingMiddleware -v
```

期待: `undefined: httpserver.LoggingMiddleware`。

- [ ] **Step 3: middleware 実装**

`internal/adapter/httpserver/middleware.go`:

```go
package httpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// statusCapturingWriter はステータスコードをキャプチャする ResponseWriter ラッパ。
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// LoggingMiddleware は DashboardUseCase にリクエスト履歴を記録するミドルウェアを返す。
func LoggingMiddleware(d *usecase.DashboardUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			scw := &statusCapturingWriter{ResponseWriter: w}
			next.ServeHTTP(scw, r)
			status := scw.status
			if status == 0 {
				status = http.StatusOK
			}
			d.AppendRequest(domain.RequestLogEntry{
				At:         start,
				Method:     r.Method,
				Path:       r.URL.Path,
				Slug:       firstPathSegment(r.URL.Path),
				StatusCode: status,
				DurationMs: time.Since(start).Milliseconds(),
			})
		})
	}
}

// firstPathSegment は "/sl-random/data.json" から "sl-random" を抽出する。
// 先頭が "/" でない、または segment が無い場合は空文字を返す。
func firstPathSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
```

- [ ] **Step 4: server.go の Deps に Dashboard を追加**

`internal/adapter/httpserver/server.go` を修正:

```go
// Deps は HTTP ハンドラが依存する usecase 群。
type Deps struct {
	Pick      *usecase.PickUseCase
	Pub       *usecase.PublishedTableUseCase
	Owned     *usecase.OwnedMD5Cache       // 後続 Task 17 で使用
	Dashboard *usecase.DashboardUseCase
	Log       *slog.Logger
}
```

- [ ] **Step 5: router.go から middleware を適用**

`internal/adapter/httpserver/router.go` を修正:

```go
package httpserver

import "net/http"

// NewMux は 4 ルートを登録した http.Handler を返す。LoggingMiddleware でラップ済み。
func NewMux(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", newHTMLHandler(deps))
	mux.HandleFunc("GET /{slug}/header.json", newHeaderHandler(deps))
	mux.HandleFunc("GET /{slug}/data.json", newDataHandler(deps))
	mux.HandleFunc("POST /{slug}/_refresh", newRefreshHandler(deps))
	if deps.Dashboard != nil {
		return LoggingMiddleware(deps.Dashboard)(mux)
	}
	return mux
}
```

`server.go` 側で `mux := NewMux(deps)` の戻り値の型が `*http.ServeMux` から `http.Handler` になるので、利用側 (server.go の `Handler: mux`) を `Handler: NewMux(deps)` に変える:

```go
func New(addr string, deps Deps) *AdapterServer {
	return &AdapterServer{
		addr: addr,
		srv: &http.Server{
			Handler:           NewMux(deps),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}
```

- [ ] **Step 6: テスト pass を確認 (middleware + 既存 router/server)**

```bash
go test ./internal/adapter/httpserver/... -v
```

期待: 全 PASS (middleware の新規 3 件 + 既存テストの無回帰)。既存テストが Deps の組み立てで `Owned` / `Dashboard` を nil で渡していて落ちる場合は、テストヘルパ (`handler_test_helpers_test.go`) の Deps 組み立てを見直して nil 許容にする。

- [ ] **Step 7: コミット**

```bash
git add internal/adapter/httpserver/middleware.go internal/adapter/httpserver/middleware_test.go internal/adapter/httpserver/server.go internal/adapter/httpserver/router.go
git commit -m "feat(httpserver): LoggingMiddleware を追加し Deps に Dashboard/Owned を追加"
```

---

## Task 15: HTML ビュー所持判定リファクタ (Deps.Owned 経由)

**Files:**
- Modify: `internal/adapter/httpserver/handler_html.go`
- Modify: `internal/adapter/httpserver/handler_html_test.go`

`buildHTMLPageData` のスタブ `ownedSet := map[string]struct{}{}` を `deps.Owned.Get(ctx)` に置換。`OwnedMD5Cache` がエラーの場合は warn ログを出し、空集合で続行 (全件 unowned 表示)。

- [ ] **Step 1: 失敗するテストを書く**

既存の `httpFixture` (handler_test_helpers_test.go) は `Owned` 経由の所持判定を考慮していないため、新たに OwnedMD5Cache をプリロードする helper を helper ファイルに追加する。Owned cache は `port.OwnedChartRepo` を fake にすれば任意の md5 集合で初期化できる。

まず helper を追加:

```go
// 既存 handler_test_helpers_test.go の末尾に追加

// fakeOwnedRepo は port.OwnedChartRepo の fake。任意の md5 集合を返す。
type fakeOwnedRepo struct {
	set map[string]struct{}
}

func (r *fakeOwnedRepo) LoadOwnedMD5Set(_ context.Context, _ string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(r.set))
	for k := range r.set {
		out[k] = struct{}{}
	}
	return out, nil
}

// fakeConfigStore は songdata_db_path に固定値を返す。
type fakeConfigStore struct{ path string }

func (c *fakeConfigStore) Get(_ context.Context, key string) (string, bool, error) {
	if key == "songdata_db_path" {
		return c.path, true, nil
	}
	return "", false, nil
}
func (c *fakeConfigStore) Set(_ context.Context, _ string, _ string) error { return nil }

func newPrimedOwnedCache(t *testing.T, mds5 []string) *usecase.OwnedMD5Cache {
	t.Helper()
	set := map[string]struct{}{}
	for _, m := range mds5 {
		set[m] = struct{}{}
	}
	repo := &fakeOwnedRepo{set: set}
	cfg := &fakeConfigStore{path: "/tmp/fake.db"}
	cache := usecase.NewOwnedMD5Cache(repo, cfg, stubClock{t: time.Now()},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, cache.Reload(context.Background()))
	return cache
}
```

次にテスト本体を追加 (`handler_html_test.go` の末尾):

```go
func TestHTMLHandler_OwnedOnlyFalse_StillColorsByOwnedSet(t *testing.T) {
	fx := newHTTPFixture(t)
	owned := newPrimedOwnedCache(t, []string{"ownedmd5"})

	// fixture 既定の Deps を上書きして Owned 付きの mux を再構築
	mux := httpserver.NewMux(httpserver.Deps{
		Pick:  fx.pickUC,
		Pub:   fx.pubUC,
		Owned: owned,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// PickResult を直接 store に書き込んで Pick 経由のロジックを使わせる代わりに、
	// ここでは public な PublishedTable + SourceChart を仕込んで GET /<slug> を呼ぶ。
	// fixture のソース表 / 公開表セットアップを使う想定。仮に fixture が
	// セットアップヘルパを持たない場合は以下のような直書きでもよい:
	srcID := "01J0SRC000000000000000000A"
	require.NoError(t, fx.srcRepo.Create(context.Background(), domain.SourceTable{
		ID: srcID, InputURL: "https://example.com/t.html",
		InputKind: domain.InputKindHTML, DisplayName: "T", Name: "T",
		LevelOrder: []string{"sl0"}, LastFetchStatus: domain.FetchStatusOK,
	}))
	// chart は SaveFetched 経由で書く方が無難
	require.NoError(t, fx.srcRepo.SaveFetched(context.Background(), srcID, port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "T", LevelOrder: []string{"sl0"}},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "ownedmd5", Level: "sl0", Title: "owned-song", Raw: map[string]any{"md5": "ownedmd5"}},
			{Position: 1, MD5: "othermd5", Level: "sl0", Title: "other-song", Raw: map[string]any{"md5": "othermd5"}},
		},
	}, time.Now()))

	pubID, err := fx.pubUC.Create(context.Background(), domain.PublishedTable{
		Slug: "t", DisplayName: "T", SourceTableID: srcID, OwnedOnly: false,
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	require.NoError(t, err)
	_ = pubID

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/t")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	bodyStr := string(body)
	assert.Contains(t, bodyStr, "owned-song")
	assert.Contains(t, bodyStr, "other-song")
	// HTML テンプレに class="owned" / class="unowned" が出ている前提。
	// テンプレ実装に合わせて文字列を調整 (Plan 1-3 で <tr class="{{if .Owned}}owned{{else}}unowned{{end}}"> を採用済み)
	assert.Contains(t, bodyStr, `class="owned"`)
	assert.Contains(t, bodyStr, `class="unowned"`)
}
```

注: `fx.pubUC.Create` のシグネチャと `PublishedTable.Pick.RefreshMode` の constant 名 (`RefreshModePerRequest` 等) は既存実装を参照して合わせる。`SaveFetched` の引数の `time.Time` はその場で `time.Now()`。HTML テンプレが `class="owned"` / `class="unowned"` を出していることは既存テンプレを確認した上で assertion を書く:

```bash
grep -n "owned\|unowned" /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver/templates/index.html
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/adapter/httpserver/... -run TestHTMLHandler_OwnedOnlyFalse_StillColorsByOwnedSet -v
```

期待: FAIL (現在のスタブは全件 unowned 表示なので owned class が存在しない)。

- [ ] **Step 3: handler_html.go の buildHTMLPageData を修正**

`internal/adapter/httpserver/handler_html.go` の `buildHTMLPageData` を修正:

```go
func buildHTMLPageData(ctx context.Context, deps Deps, pub domain.PublishedTable, r domain.PickResult) htmlPageData {
	ownedSet := map[string]struct{}{}
	if deps.Owned != nil {
		got, err := deps.Owned.Get(ctx)
		if err != nil {
			deps.Log.Warn("owned md5 cache fetch failed in html view", "err", err, "slug", pub.Slug)
		} else {
			ownedSet = got
		}
	}

	levels := make([]htmlLevel, 0, len(r.LevelOrder))
	for _, level := range r.LevelOrder {
		var charts []htmlChart
		for _, c := range r.Charts {
			if c.Level != level {
				continue
			}
			_, owned := ownedSet[c.MD5]
			if pub.OwnedOnly {
				owned = true // 既に絞り込み済み
			}
			charts = append(charts, htmlChart{
				Title: c.Title, Artist: c.Artist, MD5: c.MD5, Owned: owned,
			})
		}
		if len(charts) == 0 {
			continue
		}
		levels = append(levels, htmlLevel{Level: level, Charts: charts})
	}

	return htmlPageData{
		Slug:         pub.Slug,
		DisplayName:  pub.DisplayName,
		Symbol:       pub.Symbol,
		GeneratedAt:  r.GeneratedAt.Local().Format("2006-01-02 15:04:05 MST"),
		TotalCount:   len(r.Charts),
		IsManualMode: pub.Pick.RefreshMode == domain.RefreshModeManual,
		Levels:       levels,
	}
}
```

(コメントヘッダの「Plan 3 では Deps に owned cache を流していない」記述を削除)

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/adapter/httpserver/... -v
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/adapter/httpserver/handler_html.go internal/adapter/httpserver/handler_html_test.go
git commit -m "refactor(httpserver): HTML ビュー所持判定を OwnedMD5Cache 経由にリファクタ"
```

---

## Task 16: DashboardHandler 実装 + Bind 追加

**Files:**
- Create: `internal/app/handler/dashboard_handler.go`
- Create: `internal/app/handler/dashboard_handler_test.go`

`DashboardHandler.Snapshot` は `DashboardSnapshot` を JSON シリアライズ可能な DTO に変換して返す。

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/handler/dashboard_handler_test.go`:

```go
package handler_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestDashboardHandler_Snapshot(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	uc := usecase.NewDashboardUseCase(pickStore)
	at := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	uc.AppendRequest(domain.RequestLogEntry{At: at, Method: "GET", Path: "/sl/data.json", StatusCode: 200, DurationMs: 5, Slug: "sl"})
	uc.AppendFetch(domain.FetchLogEntry{At: at, SourceID: "src1", DisplayName: "Satellite", Status: domain.FetchStatusOK})

	h := handler.NewDashboardHandler(uc)
	got, err := h.Snapshot()
	assert.NoError(t, err)
	assert.Len(t, got.Requests, 1)
	assert.Equal(t, "GET", got.Requests[0].Method)
	assert.Equal(t, "/sl/data.json", got.Requests[0].Path)
	assert.Equal(t, "sl", got.Requests[0].Slug)
	assert.Equal(t, 200, got.Requests[0].StatusCode)
	assert.Equal(t, int64(5), got.Requests[0].DurationMs)
	assert.Len(t, got.Fetches, 1)
	assert.Equal(t, "Satellite", got.Fetches[0].DisplayName)
}
```

- [ ] **Step 2: テスト失敗を確認**

```bash
go test ./internal/app/handler/... -run TestDashboardHandler -v
```

期待: `undefined: handler.NewDashboardHandler`。

- [ ] **Step 3: DashboardHandler 実装**

`internal/app/handler/dashboard_handler.go`:

```go
package handler

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// RequestLogDTO は JSON シリアライズ用のリクエストログ。
type RequestLogDTO struct {
	At         string `json:"at"`         // RFC3339 (UTC)
	Method     string `json:"method"`
	Path       string `json:"path"`
	Slug       string `json:"slug"`
	StatusCode int    `json:"statusCode"`
	DurationMs int64  `json:"durationMs"`
}

// FetchLogDTO は JSON シリアライズ用のソース表更新ログ。
type FetchLogDTO struct {
	At          string `json:"at"`
	SourceID    string `json:"sourceId"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	Error       string `json:"error"`
}

// PickSnapshotDTO は JSON シリアライズ用のピック結果サマリ。
type PickSnapshotDTO struct {
	PublishedID string         `json:"publishedId"`
	GeneratedAt string         `json:"generatedAt"`
	LevelOrder  []string       `json:"levelOrder"`
	LevelCounts map[string]int `json:"levelCounts"`
	TotalCount  int            `json:"totalCount"`
}

// DashboardSnapshotDTO は Snapshot が返す JSON 構造体。
type DashboardSnapshotDTO struct {
	Requests []RequestLogDTO   `json:"requests"`
	Fetches  []FetchLogDTO     `json:"fetches"`
	Picks    []PickSnapshotDTO `json:"picks"`
}

// DashboardHandler は Wails Bind 経由でフロントエンドから呼ばれる。
type DashboardHandler struct {
	uc  *usecase.DashboardUseCase
	ctx context.Context
}

func NewDashboardHandler(uc *usecase.DashboardUseCase) *DashboardHandler {
	return &DashboardHandler{uc: uc, ctx: context.Background()}
}

func (h *DashboardHandler) SetContext(ctx context.Context) { h.ctx = ctx }

// Snapshot は現在のダッシュボードデータを返す。
func (h *DashboardHandler) Snapshot() (DashboardSnapshotDTO, error) {
	s := h.uc.Snapshot()
	out := DashboardSnapshotDTO{
		Requests: make([]RequestLogDTO, 0, len(s.Requests)),
		Fetches:  make([]FetchLogDTO, 0, len(s.Fetches)),
		Picks:    make([]PickSnapshotDTO, 0, len(s.Picks)),
	}
	for _, r := range s.Requests {
		out.Requests = append(out.Requests, RequestLogDTO{
			At: r.At.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Method: r.Method, Path: r.Path, Slug: r.Slug,
			StatusCode: r.StatusCode, DurationMs: r.DurationMs,
		})
	}
	for _, f := range s.Fetches {
		out.Fetches = append(out.Fetches, FetchLogDTO{
			At: f.At.UTC().Format("2006-01-02T15:04:05Z07:00"),
			SourceID: f.SourceID, DisplayName: f.DisplayName,
			Status: string(f.Status), Error: f.Error,
		})
	}
	for _, p := range s.Picks {
		out.Picks = append(out.Picks, PickSnapshotDTO{
			PublishedID: p.PublishedID,
			GeneratedAt: p.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			LevelOrder:  p.LevelOrder,
			LevelCounts: p.LevelCounts,
			TotalCount:  p.TotalCount,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: テスト pass を確認**

```bash
go test ./internal/app/handler/... -run TestDashboardHandler -v
```

期待: 全 PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/app/handler/dashboard_handler.go internal/app/handler/dashboard_handler_test.go
git commit -m "feat(handler): DashboardHandler を追加"
```

---

## Task 17: Services / Bootstrap / app.go / main.go 配線

**Files:**
- Modify: `internal/app/bootstrap.go`
- Modify: `app.go`
- Modify: `main.go`

`DashboardUseCase` を Bootstrap で生成、HTTP server Deps と Source/Pick の hook、Wails event 配信、トレイ SetState 配線、main.go の Bind に DashboardHandler を追加する。

- [ ] **Step 1: bootstrap.go に DashboardUseCase + DashboardHandler 配線を追加**

`internal/app/bootstrap.go` の Services 構造体に追加:

```go
type Services struct {
	DB                    *sql.DB
	Logger                *slog.Logger
	LoggerClose           logger.CloseFunc
	ConfigHandler         *handler.ConfigHandler
	SourceTableHandler    *handler.SourceTableHandler
	PublishedTableHandler *handler.PublishedTableHandler
	PickHandler           *handler.PickHandler
	ServerStatusHandler   *handler.ServerStatusHandler
	OwnedChartHandler     *handler.OwnedChartHandler
	DashboardHandler      *handler.DashboardHandler   // 新規
	SourceTableUseCase    *usecase.SourceTableUseCase
	ServerUseCase         *usecase.ServerUseCase
	PickResultStore       *usecase.PickResultStore     // 新規 (event 連携用)
	DashboardUseCase      *usecase.DashboardUseCase    // 新規
}
```

`Bootstrap()` 関数内、`pickStore := usecase.NewPickResultStore()` の直後あたり (PickUseCase より前) に挿入:

```go
	dashboardUC := usecase.NewDashboardUseCase(pickStore)
	dashboardHandler := handler.NewDashboardHandler(dashboardUC)

	// PickResultStore 変化通知 → DashboardUseCase 経由で event 配信ポイントに繋ぐ
	pickStore.OnChange(func(publishedID string) {
		dashboardUC.NotifyPickChanged(publishedID)
	})

	// ソース表 refresh 完了通知 → ダッシュボード履歴へ
	sourceUC.OnRefreshComplete(func(e usecase.RefreshCompleteEvent) {
		dashboardUC.AppendFetch(domain.FetchLogEntry{
			At: e.At, SourceID: e.SourceID, DisplayName: e.DisplayName,
			Status: e.Status, Error: e.Error,
		})
	})
```

`httpFactory` の Deps を更新:

```go
	httpFactory := func(addr string) usecase.HTTPServer {
		return httpserver.New(addr, httpserver.Deps{
			Pick:      pickUC,
			Pub:       pubUC,
			Owned:     ownedCache,
			Dashboard: dashboardUC,
			Log:       lg,
		})
	}
```

最後の return 部:

```go
	return &Services{
		DB:                    db,
		Logger:                lg,
		LoggerClose:           closeLog,
		ConfigHandler:         configHandler,
		SourceTableHandler:    sourceHandler,
		PublishedTableHandler: pubHandler,
		PickHandler:           pickHandler,
		ServerStatusHandler:   serverHandler,
		OwnedChartHandler:     ownedHandler,
		DashboardHandler:      dashboardHandler,
		SourceTableUseCase:    sourceUC,
		ServerUseCase:         serverUC,
		PickResultStore:       pickStore,
		DashboardUseCase:      dashboardUC,
	}, nil
```

import に `domain` がまだ無ければ追加:

```go
import (
	// ...
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)
```

- [ ] **Step 2: app.go の OnStartup でダッシュボード event 配信を追加**

`app.go` の `startup` 関数内、`a.services.ServerUseCase.OnStatusChange(...)` の下に追加:

```go
	// ダッシュボード event 配信
	a.services.DashboardUseCase.OnRequest(func(e domain.RequestLogEntry) {
		wailsruntime.EventsEmit(ctx, "dashboard:request_logged", e)
	})
	a.services.DashboardUseCase.OnFetch(func(e domain.FetchLogEntry) {
		wailsruntime.EventsEmit(ctx, "dashboard:fetch_logged", e)
	})
	a.services.DashboardUseCase.OnPickChanged(func(publishedID string) {
		wailsruntime.EventsEmit(ctx, "dashboard:pick_changed", publishedID)
	})
```

`a.services.DashboardHandler.SetContext(ctx)` を他の `SetContext` 群と一緒に追加:

```go
	a.services.DashboardHandler.SetContext(ctx)
```

- [ ] **Step 3: app.go の OnStartup でトレイ SetState 配線を追加**

`a.services.ServerUseCase.OnStatusChange(...)` を以下に拡張 (既存の EventsEmit に加えて Tray SetState):

```go
	a.services.ServerUseCase.OnStatusChange(func(s domain.ServerStatus) {
		wailsruntime.EventsEmit(ctx, "server_status:changed", s)
		if a.tray != nil && a.tray.IsRunning() {
			switch s.State {
			case domain.ServerStateRunning:
				a.tray.SetState(tray.StateRunning)
			case domain.ServerStateError:
				a.tray.SetState(tray.StateError)
			default:
				a.tray.SetState(tray.StateIdle)
			}
		}
	})
```

import 追加:

```go
import (
	// ...
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray"
)
```

注: `tray` パッケージはすでに import 済みのはず (App 構造体の `tray *tray.Tray` フィールド)。`tray.StateRunning` 等の定数名は `tray_other.go` の定義に従う (Plan 1 で定義済み)。`tray.StateRunning` が darwin ビルド時に未定義だと build エラーになるため、`tray_darwin.go` 側に同名の no-op 定数を追加する必要があるかも。確認:

```bash
grep -n "StateIdle\|StateRunning\|StateError\|SetState" /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray/*.go
```

`tray_darwin.go` に同名の定数 + ダミーの `SetState` メソッド (no-op) が無ければ追加する (Task 19 で詳細実装するが、ここでは型整合のためのスタブ追加が必要):

`internal/adapter/tray/tray_darwin.go` (もし無い定義があれば):

```go
//go:build darwin

package tray

type State int

const (
	StateIdle State = iota
	StateRunning
	StateError
)

// (既存の Tray 構造体にメソッド追加)
func (t *Tray) SetState(_ State) {} // no-op on macOS
```

- [ ] **Step 4: main.go の Bind 配列に DashboardHandler を追加**

`main.go` の `Bind` 配列を更新:

```go
		Bind: []any{
			myApp,
			services.ConfigHandler,
			services.SourceTableHandler,
			services.PublishedTableHandler,
			services.PickHandler,
			services.ServerStatusHandler,
			services.OwnedChartHandler,
			services.DashboardHandler,    // 追加
		},
```

- [ ] **Step 5: ビルド確認 (darwin)**

```bash
go build ./...
```

期待: エラーなし。

- [ ] **Step 6: TS bindings 再生成**

```bash
wails generate module
```

期待: `frontend/wailsjs/go/handler/DashboardHandler.{d.ts,js}` が生成される。

- [ ] **Step 7: テスト pass を確認**

```bash
go test ./...
```

期待: 全 PASS。

- [ ] **Step 8: コミット**

```bash
git add internal/app/bootstrap.go app.go main.go internal/adapter/tray/tray_darwin.go
git commit -m "feat(app): DashboardUseCase 配線 + Wails event 配信 + tray SetState 連携"
```

---

## Task 18: api.ts に dashboard API + イベント追加

**Files:**
- Modify: `frontend/src/lib/api.ts`

DashboardHandler.Snapshot ラッパと、3 つの新規イベント (`dashboard:request_logged` / `dashboard:fetch_logged` / `dashboard:pick_changed`) の購読 API を追加。

- [ ] **Step 1: api.ts を更新**

`frontend/src/lib/api.ts` の上部 import に追加:

```ts
import { Snapshot as DashboardSnapshot } from '../../wailsjs/go/handler/DashboardHandler';
```

型定義を追加 (既存の他 DTO と同じ場所):

```ts
export type RequestLogDTO = {
  at: string;
  method: string;
  path: string;
  slug: string;
  statusCode: number;
  durationMs: number;
};

export type FetchLogDTO = {
  at: string;
  sourceId: string;
  displayName: string;
  status: 'never' | 'ok' | 'error';
  error: string;
};

export type PickSnapshotDTO = {
  publishedId: string;
  generatedAt: string;
  levelOrder: string[];
  levelCounts: Record<string, number>;
  totalCount: number;
};

export type DashboardSnapshotDTO = {
  requests: RequestLogDTO[];
  fetches: FetchLogDTO[];
  picks: PickSnapshotDTO[];
};
```

`api` オブジェクトに追加:

```ts
  // ---- ダッシュボード ----
  getDashboardSnapshot(): Promise<DashboardSnapshotDTO> {
    return DashboardSnapshot() as Promise<DashboardSnapshotDTO>;
  },
  onDashboardRequestLogged(cb: (e: RequestLogDTO) => void): () => void {
    EventsOn('dashboard:request_logged', cb);
    return () => EventsOff('dashboard:request_logged');
  },
  onDashboardFetchLogged(cb: (e: FetchLogDTO) => void): () => void {
    EventsOn('dashboard:fetch_logged', cb);
    return () => EventsOff('dashboard:fetch_logged');
  },
  onDashboardPickChanged(cb: (publishedID: string) => void): () => void {
    EventsOn('dashboard:pick_changed', cb);
    return () => EventsOff('dashboard:pick_changed');
  },
```

- [ ] **Step 2: ビルド確認**

```bash
cd frontend
npx tsc --noEmit
```

期待: 型エラーなし。

- [ ] **Step 3: コミット**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git add frontend/src/lib/api.ts
git commit -m "feat(frontend): api.ts にダッシュボード API + イベントを追加"
```

---

## Task 19: DashboardTab.svelte 本実装

**Files:**
- Modify: `frontend/src/lib/tabs/DashboardTab.svelte` (Task 8 で作ったプレースホルダを置換)

`onMount` で `getDashboardSnapshot()` 取得 → state に格納 → 3 つのイベントを購読 → 受信時に state 更新。

- [ ] **Step 1: DashboardTab.svelte を全置換**

`frontend/src/lib/tabs/DashboardTab.svelte`:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type DashboardSnapshotDTO, type RequestLogDTO, type FetchLogDTO, type PickSnapshotDTO } from '../api';

  let requests: RequestLogDTO[] = [];
  let fetches: FetchLogDTO[] = [];
  let picks: PickSnapshotDTO[] = [];
  let loading = true;
  let error = '';

  let unsubReq: (() => void) | null = null;
  let unsubFetch: (() => void) | null = null;
  let unsubPick: (() => void) | null = null;

  const REQ_CAP = 100;
  const FETCH_CAP = 100;

  onMount(async () => {
    try {
      const snap: DashboardSnapshotDTO = await api.getDashboardSnapshot();
      requests = snap.requests;
      fetches = snap.fetches;
      picks = snap.picks;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
    unsubReq = api.onDashboardRequestLogged((e) => {
      requests = [e, ...requests].slice(0, REQ_CAP);
    });
    unsubFetch = api.onDashboardFetchLogged((e) => {
      fetches = [e, ...fetches].slice(0, FETCH_CAP);
    });
    unsubPick = api.onDashboardPickChanged(async (_publishedId) => {
      // ピック変化はリストではなく集約なので毎回 Snapshot を取り直す方が確実
      try {
        const snap = await api.getDashboardSnapshot();
        picks = snap.picks;
      } catch (e) {
        console.warn('refresh picks failed', e);
      }
    });
  });

  onDestroy(() => {
    if (unsubReq) unsubReq();
    if (unsubFetch) unsubFetch();
    if (unsubPick) unsubPick();
  });

  function formatJST(iso: string): string {
    if (!iso) return '-';
    try {
      return new Date(iso).toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo', hour12: false });
    } catch {
      return iso;
    }
  }

  function statusBadge(code: number): string {
    if (code >= 500) return 'badge-error';
    if (code >= 400) return 'badge-warning';
    if (code >= 300) return 'badge-info';
    if (code >= 200) return 'badge-success';
    return 'badge-ghost';
  }

  function fetchStatusBadge(s: string): string {
    if (s === 'ok') return 'badge-success';
    if (s === 'error') return 'badge-error';
    return 'badge-ghost';
  }

  function levelCountsLine(counts: Record<string, number>, order: string[]): string {
    const keys = order.length ? order : Object.keys(counts);
    return keys.map((k) => `${k}:${counts[k] ?? 0}`).join(' / ');
  }
</script>

<section class="p-4 space-y-4">
  {#if loading}
    <div class="flex items-center gap-2 text-sm py-4">
      <span class="loading loading-spinner loading-sm"></span>
      <span>読み込み中…</span>
    </div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else}
    <!-- 最近のリクエスト -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">最近のリクエスト ({requests.length})</h2>
        {#if requests.length === 0}
          <div class="text-sm opacity-70 py-2">まだリクエストはありません。</div>
        {:else}
          <div class="overflow-x-auto max-h-96">
            <table class="table table-sm table-pin-rows">
              <thead>
                <tr>
                  <th>時刻 (JST)</th>
                  <th>メソッド</th>
                  <th>パス</th>
                  <th>ステータス</th>
                  <th>経過 ms</th>
                </tr>
              </thead>
              <tbody>
                {#each requests as r, i (`${r.at}-${i}`)}
                  <tr>
                    <td class="text-xs">{formatJST(r.at)}</td>
                    <td>{r.method}</td>
                    <td class="text-xs break-all">{r.path}</td>
                    <td><span class="badge {statusBadge(r.statusCode)}">{r.statusCode}</span></td>
                    <td>{r.durationMs}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>

    <!-- ソース表更新履歴 -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">ソース表更新履歴 ({fetches.length})</h2>
        {#if fetches.length === 0}
          <div class="text-sm opacity-70 py-2">起動以降の更新履歴はありません。</div>
        {:else}
          <div class="overflow-x-auto max-h-72">
            <table class="table table-sm table-pin-rows">
              <thead>
                <tr>
                  <th>時刻 (JST)</th>
                  <th>表示名</th>
                  <th>結果</th>
                  <th>エラー</th>
                </tr>
              </thead>
              <tbody>
                {#each fetches as f, i (`${f.at}-${i}`)}
                  <tr>
                    <td class="text-xs">{formatJST(f.at)}</td>
                    <td>{f.displayName || f.sourceId}</td>
                    <td><span class="badge {fetchStatusBadge(f.status)}">{f.status}</span></td>
                    <td class="text-xs whitespace-pre-line">{f.error}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>

    <!-- 現在のピック結果サマリ -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">現在のピック結果 ({picks.length})</h2>
        {#if picks.length === 0}
          <div class="text-sm opacity-70 py-2">まだピック結果はありません。公開表へのリクエストか「再ピック」で生成されます。</div>
        {:else}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
            {#each picks as p (p.publishedId)}
              <div class="rounded-box border border-base-300 p-3 text-sm">
                <div class="font-mono text-xs opacity-70">{p.publishedId}</div>
                <div>計 {p.totalCount} 曲</div>
                <div class="text-xs opacity-70 mt-1">{levelCountsLine(p.levelCounts, p.levelOrder)}</div>
                <div class="text-xs opacity-50 mt-1">生成: {formatJST(p.generatedAt)}</div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</section>
```

- [ ] **Step 2: 動作確認**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
make dev
```

確認:
- ダッシュボードタブで初期 Snapshot が出る (起動直後はリクエスト・フェッチが空、起動後の RefreshAll が走るとフェッチが順次入る)
- 別タブからソース表追加 / 個別更新 → ダッシュボードタブのフェッチ履歴がリアルタイム更新
- ブラウザで `http://127.0.0.1:50000/<slug>` にアクセス → リクエスト履歴がリアルタイム更新
- 「再ピック」 → ピック結果サマリが更新

- [ ] **Step 3: コミット**

```bash
git add frontend/src/lib/tabs/DashboardTab.svelte
git commit -m "feat(frontend): DashboardTab を実装 (リクエスト/フェッチ/ピック表示)"
```

---

## Task 20: トレイアイコン素材 (純色 PNG) 生成

**Files:**
- Create: `internal/adapter/tray/icons/idle.png`
- Create: `internal/adapter/tray/icons/running.png`
- Create: `internal/adapter/tray/icons/error.png`
- Create: `internal/adapter/tray/icons/gen.go` (`//go:build ignore`)
- Modify: `internal/adapter/tray/icons.go`

16x16 純色 PNG を Go の `image/png` で生成し、本番コードでは `//go:embed` で読み込む。

- [ ] **Step 1: 素材生成スクリプト作成**

`internal/adapter/tray/icons/gen.go`:

```go
//go:build ignore

// Package main は tray アイコン PNG を生成するワンショットスクリプト。
// Usage: go run gen.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type spec struct {
	name string
	c    color.NRGBA
}

func main() {
	dir, err := os.Getwd()
	must(err)
	specs := []spec{
		{"idle.png", color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{"running.png", color.NRGBA{R: 56, G: 161, B: 105, A: 255}},
		{"error.png", color.NRGBA{R: 198, G: 56, B: 56, A: 255}},
	}
	for _, s := range specs {
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				img.Set(x, y, s.c)
			}
		}
		f, err := os.Create(filepath.Join(dir, s.name))
		must(err)
		must(png.Encode(f, img))
		_ = f.Close()
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
```

- [ ] **Step 2: スクリプト実行**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor/internal/adapter/tray/icons
go run gen.go
ls -la *.png
```

期待: `idle.png` / `running.png` / `error.png` が生成される (各 ~80 バイト程度)。

- [ ] **Step 3: icons.go を embed ベースに変更**

`internal/adapter/tray/icons.go` を全置換:

```go
//go:build !darwin

package tray

import _ "embed"

//go:embed icons/idle.png
var iconIdle []byte

//go:embed icons/running.png
var iconRunning []byte

//go:embed icons/error.png
var iconError []byte

// IconFor は与えられた状態に対応するアイコンバイト列を返す。
func IconFor(state State) []byte {
	switch state {
	case StateRunning:
		return iconRunning
	case StateError:
		return iconError
	default:
		return iconIdle
	}
}
```

- [ ] **Step 4: ビルド確認**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
go build ./...
```

期待: エラーなし (embed が PNG を取り込む)。

- [ ] **Step 5: 動作確認 (Windows でのみ目視確認可、macOS は no-op)**

`make dev` の Windows 版での確認は Task 24 (Windows ビルド + 実機 E2E) でまとめて行う。ここでは手元 (macOS) で `go test ./...` が pass することのみ確認:

```bash
go test ./...
```

期待: 全 PASS。

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/tray/icons.go internal/adapter/tray/icons/
git commit -m "feat(tray): 純色 PNG アイコン素材と embed 化を追加 (Idle/Running/Error)"
```

注: PNG ファイルは小さいので git に含めて問題ない。`gen.go` は `//go:build ignore` で本ビルドからは除外される。

---

## Task 21: docs/manual.md (ユーザー向けマニュアル)

**Files:**
- Create: `docs/manual.md`

最小構成。スクリーンショットなし、トラブルシューティングに IPv4 ポート占有問題を含む。

- [ ] **Step 1: docs/manual.md 作成**

`docs/manual.md`:

```markdown
# BMS Random Table Compositor ユーザーマニュアル

## はじめに

BMS Random Table Compositor は、既存の BMS 難易度表をローカルで再ホストし、編集を加えて beatoraja に提供する Windows 向けデスクトップアプリです。主な機能:

- 難易度表のレベル毎ランダムピック
- 所持譜面のみ表示 (beatoraja の `songdata.db` を参照)
- ローカル HTTP サーバとして編集済み難易度表を配信

動作環境: Windows 10 / 11 (推奨)。macOS でも GUI と HTTP サーバは動作するが、システムトレイ常駐は無効でウィンドウクローズ時にアプリが終了する。

## 初回起動

1. 配布された `bms-random-table-compositor.exe` をフォルダに配置して起動
2. 同じフォルダに `compositor.db` (アプリ設定 DB) と `logs/` (日次ログ) が自動作成される (ポータブル運用)
3. 二重起動はブロックされ、既に起動済みの窓が前面化される

## 設定 (サーバ設定タブ)

### ポート番号

HTTP サーバが listen するポート。既定 50000。beatoraja に登録する URL のポート部と一致させる。

### songdata.db のパス

beatoraja の `songdata.db` の絶対パス。「参照…」ボタンから OS のファイル選択ダイアログで指定可能。所持絞り込み機能で使われる。

### サーバ操作

「起動 / 停止 / 再起動」でローカル HTTP サーバを制御する。状態は緑バッジ (稼働中) / 赤バッジ (エラー) / グレー (停止中) で表示される。

### 所持キャッシュ

`songdata.db` から読み込んだ md5 集合のメモリキャッシュ。設定変更時は自動 invalidate されるが、beatoraja でプレイ後に手動で「再読み込み」したい場合はボタンを押す。

## ソース表 (ソース表タブ)

### 追加

URL を入力して「追加」を押す。HTML URL (例: `https://stellabms.xyz/st/table.html`) または `header.json` 直 URL のどちらでも可。追加後にバックグラウンド更新が走り、`source_table_chart` に譜面が展開される。

### 一覧操作

行右クリックで「再取得」「削除」のコンテキストメニュー、または各行末尾のボタンから操作。

## 公開表 (公開表タブ)

### 新規作成

「新規作成」ボタンから以下を設定:

- 表示名: HTML ビューや beatoraja に表示される名前
- slug: URL のパスに使われる (`http://127.0.0.1:50000/<slug>`)。「自動生成」でソース表名から派生
- シンボル: 譜面リストでレベル前に付く記号 (例: `★`)
- ソース表: 元となるソース表
- 所持譜面のみ: チェックすると所持中の譜面のみ表示
- レベル毎の件数: 0 で無制限、N でランダム N 曲ずつ
- 更新モード: per_request (毎回再生成) / daily (同一日付内で固定) / manual (再ピックボタンまで固定)

### 行操作

行右クリックで「編集 / ブラウザで開く / 再ピック (manual のみ) / 削除」。

### beatoraja への登録

beatoraja の難易度表 URL 設定に `http://127.0.0.1:<port>/<slug>` を登録するだけ。アプリ生成の `header.json` / `data.json` が読み込まれる。

## ダッシュボード (ダッシュボードタブ)

3 セクション:

- 最近のリクエスト: HTTP エンドポイントへのアクセス履歴 (新しい順、最大 100 件、起動後のもの)
- ソース表更新履歴: 起動後のソース表取得結果 (成功 / 失敗とエラーメッセージ)
- 現在のピック結果: 公開表ごとの最新ピック (生成時刻、レベル別曲数、合計曲数)

リクエストとフェッチは再起動でリセットされる。長期履歴が必要な場合は `logs/YYYY-MM-DD.log` を参照。

## トラブルシューティング

### ポートが既に使われている

サーバ設定タブで状態が「エラー」、`listen tcp :50000: bind: address already in use` 等のメッセージが出る場合、別のアプリが同じポートを使用中。設定タブで別ポート (例: 50001) に変更し、保存 → 「起動」を押す。

### IPv4 のみで異常応答 (HTTP/0.9 when not allowed 等)

SSH トンネル等が `0.0.0.0:<port>` (IPv4) を先取りしていると、本アプリの listen は IPv6 のみで成功し、`http://127.0.0.1:<port>` (IPv4) アクセス時に他プロセスが応答する状態になる。設定タブで別ポートに変更すれば解消する。

### songdata.db オープン失敗

サーバ設定タブの「所持キャッシュ」セクションにエラーが表示される。パスを再選択し直す。beatoraja が起動中で `songdata.db` がロックされている場合は beatoraja を一旦終了する。

### 二重起動できない

既に別インスタンスが起動中。タスクトレイに常駐していないか確認。プロセスが残っている場合はタスクマネージャから終了。

### ログの場所

実行ファイル隣の `logs/YYYY-MM-DD.log`。日次ローテーションで 7 日分保持。詳細なリクエスト履歴やエラーはここを確認。

## ライセンス

(プロジェクトの LICENSE に従う)
```

- [ ] **Step 2: コミット**

```bash
git add docs/manual.md
git commit -m "docs: ユーザーマニュアル (manual.md) を追加"
```

---

## Task 22: docs/test-plan.md (手動 E2E チェックリスト)

**Files:**
- Create: `docs/test-plan.md`

- [ ] **Step 1: docs/test-plan.md 作成**

`docs/test-plan.md`:

```markdown
# Phase 1 MVP 手動 E2E テスト計画

最終更新: 2026-05-08 (Plan 4)

## 環境

- macOS と Windows の両方で実施
- 既存の `compositor.db` を退避してクリーン起動 (例: `compositor.db.bak` にリネーム)
- beatoraja は別途インストール済みで `songdata.db` のパスが分かっていること

## チェックリスト

### 起動・常駐 (Windows メイン)

- [ ] アプリ起動でメインウィンドウが表示される
- [ ] ウィンドウクローズでトレイに格納される (Windows / Linux のみ)
- [ ] トレイメニュー「設定を開く」「終了」が動く
- [ ] 二重起動で既存ウィンドウが前面化される
- [ ] トレイアイコンがサーバ状態に応じて 3 色で切り替わる (停止: グレー / 起動中: 緑 / エラー: 赤)
- [ ] macOS ではウィンドウクローズで通常終了する

### サーバ設定タブ

- [ ] 初回起動時、ポート 50000 / songdata.db パス空 で表示される
- [ ] ポート変更 → 保存 → 「再起動」で反映される
- [ ] songdata.db を「参照…」ボタンで OS ファイル選択ダイアログから指定できる
- [ ] サーバ「停止」「起動」「再起動」が状態に応じて enable/disable される
- [ ] 起動中は緑バッジ + ポート + 起動時刻 (JST) が表示される
- [ ] エラー時は赤バッジ + エラーメッセージ alert が表示される
- [ ] 所持キャッシュ「再読み込み」ボタンで count が更新される
- [ ] songdata.db パス変更後、所持キャッシュが invalidate される

### ソース表タブ

- [ ] HTML URL (例: stellabms.xyz の SL 表) で追加 → バックグラウンド更新で行が入る
- [ ] header.json 直 URL で追加 → 同上
- [ ] 取得失敗 (例: 不正な URL) 時に `badge-error` + エラー本文が表示される
- [ ] 行右クリック → コンテキストメニュー (再取得 / 削除)
- [ ] 削除で ConfirmDialog (キャンセル / 削除) が出る
- [ ] 「一括再取得」ボタンで全ソース表が同時に再取得される
- [ ] 個別「再取得」ボタンの動作中はその行のスピナーが出る
- [ ] 0 件状態でプレースホルダ ("ソース表が登録されていません") が出る

### 公開表タブ

- [ ] ソース表 0 件のとき新規作成ボタンが disabled、警告 alert が出る
- [ ] CRUD (新規作成 / 編集 / 削除) が動く
- [ ] slug 自動生成ボタンが動く
- [ ] slug 形式不正 / 予約語 / 重複の各エラーが赤字で出る
- [ ] 削除で ConfirmDialog が出る
- [ ] 行右クリック → コンテキストメニュー (4 項目)
- [ ] `refresh_mode != manual` の行で「再ピック」が disabled
- [ ] 「ブラウザで開く」で `http://127.0.0.1:<port>/<slug>` がデフォルトブラウザで開く

### ダッシュボードタブ

- [ ] 初期表示で Snapshot が出る (起動直後は空、RefreshAll 完了後にフェッチが入る)
- [ ] 別タブでソース表追加 → ダッシュボードタブの「ソース表更新履歴」がリアルタイム更新
- [ ] ブラウザで `http://127.0.0.1:<port>/<slug>` にアクセス → 「最近のリクエスト」がリアルタイム更新
- [ ] 「再ピック」 → 「現在のピック結果」が更新
- [ ] 100 件超えで古い行が捨てられる (連続 105 リクエスト等)
- [ ] 0 件状態でプレースホルダが出る
- [ ] 時刻が JST で表示される

### HTTP エンドポイント

- [ ] `GET /<slug>` HTML ビューが表示される (所持/未所持で色分け)
- [ ] OwnedOnly=false 公開表でも実所持で色分けされる (所持あり/なしの両方の譜面が混じる場合)
- [ ] OwnedOnly=true 公開表は全件 owned 表示
- [ ] `GET /<slug>/header.json` の `level_order` が実在レベルのみ
- [ ] `GET /<slug>/data.json` の各モード:
  - per_request: 連続リクエストで結果が変わる
  - daily: 同一日付内で固定
  - manual: 「再ピック」ボタンまで固定
- [ ] `POST /<slug>/_refresh` は manual モードのみ受付、それ以外は 405
- [ ] 存在しない slug は 404
- [ ] HTML ビュー内の `<meta name="bmstable">` が `/{slug}/header.json` (絶対パス) になっている

### beatoraja 接続

- [ ] beatoraja の難易度表 URL に `http://127.0.0.1:<port>/<slug>` を登録 → 譜面一覧が出る
- [ ] OwnedOnly=true 公開表で所持譜面のみが出る
- [ ] OwnedOnly=false 公開表で全譜面が出る
- [ ] manual モード公開表で beatoraja 側からの再読み込みでも結果が変わらない
- [ ] daily モードの日付跨ぎで結果が変わる (時計を 1 日進めて再アクセス)

### 無回帰 (Plan 1-3)

- [ ] 単一インスタンスロックが効く (二重起動で既存窓が前面化)
- [ ] OnBeforeClose で WindowHide される (Win/Linux)
- [ ] `compositor.db` のマイグレーションが冪等 (再起動で問題なし)
- [ ] ログが `logs/YYYY-MM-DD.log` に出力される

## Windows ビルド・実機確認手順

1. `git push origin main` (Plan 2 lessons #5 通り、リモート HEAD を Windows runner が見るため必須)
2. `gh workflow run build-windows.yml --ref main`
3. `gh run list --workflow=build-windows.yml --limit 1` で run-id 確認
4. `gh run watch <run-id>` で完了待機
5. `gh run download <run-id> --name <artifact-name> --dir ./tmp/win`
6. Windows 機 / VM で `bms-random-table-compositor.exe` を起動して上記チェックリストを再実行
```

- [ ] **Step 2: コミット**

```bash
git add docs/test-plan.md
git commit -m "docs: 手動 E2E テスト計画 (test-plan.md) を追加"
```

---

## Task 23: macOS で全タブ動作確認

**Files:** (変更なし、確認のみ)

- [ ] **Step 1: ビルド & 起動**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
make build
open ./build/bin/bms-random-table-compositor.app
```

- [ ] **Step 2: チェックリスト軽め (macOS で動く範囲)**

`docs/test-plan.md` のうち以下を確認:

- 起動 → ウィンドウ表示 → クローズで通常終了 (macOS はトレイ無効)
- サーバ設定: ポート / 参照ボタン / 起動 / 停止 / 再起動
- ソース表: 1 件追加 → ConfirmDialog で削除
- 公開表: 1 件作成 → ConfirmDialog で削除 → ブラウザで開く
- ダッシュボード: 初期表示 → リクエスト数件後の自動更新
- HTML ビュー: 所持判定の色分け (songdata.db に存在する md5 を含むソース表で確認)

- [ ] **Step 3: 問題があれば修正コミット**

問題があれば該当タスクに戻り、原因を特定してから修正コミット。

- [ ] **Step 4: 何も問題なければ進む (コミットなし)**

---

## Task 24: Windows ビルド + 実機 E2E

**Files:** (変更なし、確認のみ)

- [ ] **Step 1: 全コミットを push**

```bash
cd /Users/yudai.kuroki/src/github.com/meta-BE/bms-random-table-compositor
git push origin main
```

- [ ] **Step 2: Windows ビルドワークフロー発火**

```bash
gh workflow run build-windows.yml --ref main
sleep 5
RUN_ID=$(gh run list --workflow=build-windows.yml --limit 1 --json databaseId -q '.[0].databaseId')
echo "run id: $RUN_ID"
gh run watch $RUN_ID
```

- [ ] **Step 3: artifact ダウンロード**

```bash
mkdir -p tmp/win-plan4
gh run download $RUN_ID --dir tmp/win-plan4
ls -la tmp/win-plan4
```

- [ ] **Step 4: Windows 機 / VM へ転送して起動**

artifact 内の `bms-random-table-compositor.exe` を Windows 機に転送し起動。`docs/test-plan.md` のチェックリスト全件を実行。

- [ ] **Step 5: 不具合があれば修正コミット → push → 再ビルド**

繰り返し。

- [ ] **Step 6: 全件 pass で完了**

特にコミット不要。

---

## 完了後の確認

- [ ] `git log --oneline -30` で全 Plan 4 コミットが残っていることを確認
- [ ] `git push origin main` 済み
- [ ] `docs/manual.md` / `docs/test-plan.md` がコミット済み
- [ ] Windows 実機で全 E2E 通過
- [ ] memory 更新 (project_status.md を Plan 4 完了に書き換え、plan4_lessons.md を残せる学びがあれば記録)

---

## 参考メモ

- bms-elsa の参考箇所:
  - `frontend/vite.config.ts` (Tailwind v4 プラグイン構成)
  - `frontend/src/style.css` (`@plugin "daisyui"` 構文)
  - `frontend/src/components/` 各種 daisyUI コンポーネント実装 (ConfirmDialog/ContextMenu の参考)
- Plan 3 lessons #5: Bind ハンドラを追加した直後に `wails generate module` を実行し、TS bindings を再生成する
- Plan 2 lessons #1: 依存追加時はバージョン明示 (`go get module@version`、`npm install -D pkg@x.y.z`)、`go.mod` の `go 1.24.0` を変えない
- Plan 2 lessons #2: `window.confirm` / `window.alert` / `window.prompt` は使わない (Wails WebView で機能しない)。本 Plan の `ConfirmDialog` で置換
- Plan 2 lessons #5: Windows ビルド前に `git push origin main` 必須
- Plan 3 lessons #1: HTML ビュー内の相対参照は絶対パス化が必要だが、Plan 3 で対応済み
