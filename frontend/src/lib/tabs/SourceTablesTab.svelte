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
