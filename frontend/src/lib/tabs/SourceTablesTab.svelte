<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type SourceTableDTO, type AddSourceTableRequest } from '../api';

  let rows: SourceTableDTO[] = [];
  let loading = false;
  let listError = '';
  let addError = '';
  let newUrl = '';
  let busy: Record<string, boolean> = {};
  let unsubscribe: (() => void) | null = null;

  async function load() {
    loading = true;
    listError = '';
    try {
      rows = await api.listSourceTables();
    } catch (e: any) {
      listError = `読み込みエラー: ${String(e)}`;
    } finally {
      loading = false;
    }
  }

  async function add() {
    addError = '';
    if (!newUrl) {
      addError = 'URL を入力してください';
      return;
    }
    const req: AddSourceTableRequest = { url: newUrl };
    try {
      const id = await api.addSourceTable(req);
      newUrl = '';
      await load();
      // 追加直後に取得を試みる（バックグラウンド）
      busy = { ...busy, [id]: true };
      api
        .refreshSourceTable(id)
        .catch((e) => console.warn('initial refresh failed', e))
        .finally(async () => {
          busy = { ...busy, [id]: false };
          await load();
        });
    } catch (e: any) {
      addError = String(e);
    }
  }

  async function refresh(id: string) {
    busy = { ...busy, [id]: true };
    try {
      await api.refreshSourceTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = { ...busy, [id]: false };
    }
  }

  async function refreshAll() {
    loading = true;
    try {
      await api.refreshAllSourceTables();
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      loading = false;
    }
  }

  async function remove(id: string) {
    // Wails WebView では window.confirm が機能しないため、即削除する。
    // 誤操作防止 UI は Plan 4 で別途モーダル化予定。
    try {
      await api.deleteSourceTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    }
  }

  function formatJST(iso: string): string {
    if (!iso) return '-';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString('ja-JP', {
      timeZone: 'Asia/Tokyo',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  async function renameRow(id: string, displayName: string) {
    try {
      await api.updateSourceTableDisplayName(id, displayName);
      await load();
    } catch (e: any) {
      listError = String(e);
    }
  }

  function handleDisplayNameChange(id: string, e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    renameRow(id, target.value);
  }

  function statusLabel(s: SourceTableDTO['lastFetchStatus']): string {
    switch (s) {
      case 'ok':
        return '取得済み';
      case 'error':
        return 'エラー';
      default:
        return '未取得';
    }
  }

  onMount(() => {
    load();
    unsubscribe = api.onSourceTableRefreshAllDone(() => {
      load();
    });
  });

  onDestroy(() => {
    if (unsubscribe) unsubscribe();
  });
</script>

<section class="tab">
  <h2>ソース表</h2>

  <div class="add-form">
    <label class="row">
      <span class="label">URL（HTML / header.json はパス末尾の拡張子で自動判別）</span>
      <input type="text" bind:value={newUrl} placeholder="https://example.com/table.html" />
    </label>
    <div class="actions">
      <button on:click={add} disabled={!newUrl}>追加</button>
      <button on:click={refreshAll} disabled={loading}>すべて再取得</button>
    </div>
    {#if addError}<p class="message err">{addError}</p>{/if}
  </div>

  {#if loading}
    <p>読み込み中...</p>
  {/if}
  {#if listError}<p class="message err">{listError}</p>{/if}

  <table>
    <thead>
      <tr>
        <th>表示名</th>
        <th class="nowrap">略称</th>
        <th class="nowrap">状態</th>
        <th class="nowrap">最終取得</th>
        <th>URL</th>
        <th class="nowrap">操作</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as r (r.id)}
        <tr class:row-error={r.lastFetchStatus === 'error'}>
          <td>
            <input
              type="text"
              value={r.displayName}
              placeholder={r.name || '(取得中)'}
              on:change={(e) => handleDisplayNameChange(r.id, e)}
            />
          </td>
          <td class="nowrap">{r.symbol || ''}</td>
          <td class="nowrap">
            <span class="badge badge-{r.lastFetchStatus}">{statusLabel(r.lastFetchStatus)}</span>
            {#if r.lastFetchStatus === 'error'}
              <span class="err-detail" title={r.lastFetchError}>?</span>
            {/if}
          </td>
          <td class="nowrap">{formatJST(r.lastFetchedAt)}</td>
          <td class="url-cell" title={r.inputUrl}>{r.inputUrl}</td>
          <td class="nowrap">
            <button on:click={() => refresh(r.id)} disabled={busy[r.id]}>更新</button>
            <button on:click={() => remove(r.id)}>削除</button>
          </td>
        </tr>
      {/each}
      {#if rows.length === 0 && !loading}
        <tr>
          <td colspan="6" class="empty">登録なし</td>
        </tr>
      {/if}
    </tbody>
  </table>
</section>

<style>
  .tab { padding: 16px; }
  h2 { margin-top: 0; font-size: 16px; }
  .add-form { border: 1px solid #e0e0e0; padding: 12px; border-radius: 6px; margin-bottom: 16px; }
  .row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
  .label { font-size: 13px; color: #555; }
  input[type="text"], select { padding: 6px 8px; font-size: 13px; }
  .actions { display: flex; gap: 8px; margin-top: 8px; }
  button { padding: 4px 10px; font-size: 13px; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: 0.6; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { border-bottom: 1px solid #eee; padding: 6px 8px; text-align: left; vertical-align: middle; }
  th { background: #fafafa; }
  .nowrap { white-space: nowrap; }
  .url-cell { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { padding: 2px 8px; border-radius: 3px; font-size: 12px; }
  .badge-never { background: #eee; color: #666; }
  .badge-ok    { background: #e8f5e9; color: #2e7d32; }
  .badge-error { background: #ffebee; color: #b71c1c; }
  .err-detail  { color: #b71c1c; cursor: help; margin-left: 4px; }
  .row-error td { background: #fff8f8; }
  .empty { color: #999; text-align: center; padding: 16px; }
  .message.err { color: #b71c1c; margin: 8px 0 0; }
</style>
