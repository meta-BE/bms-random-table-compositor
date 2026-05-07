<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    api,
    type ServerConfig,
    type ServerStatusDTO,
    type OwnedCacheStatusDTO,
  } from '../api';

  let cfg: ServerConfig = { port: 50000, songdataDbPath: '' };
  let saveError = '';
  let saving = false;

  let status: ServerStatusDTO | null = null;
  let statusError = '';
  let busy = false;

  let owned: OwnedCacheStatusDTO | null = null;
  let ownedError = '';

  let unsubscribeStatus: (() => void) | null = null;

  async function load() {
    try {
      cfg = await api.getServerConfig();
    } catch (e: any) {
      saveError = `設定読み込み失敗: ${String(e)}`;
    }
    try {
      status = await api.getServerStatus();
    } catch (e: any) {
      statusError = String(e);
    }
    try {
      owned = await api.getOwnedCacheStatus();
    } catch (e: any) {
      ownedError = String(e);
    }
  }

  async function save() {
    saveError = '';
    saving = true;
    try {
      await api.setServerPort(cfg.port);
      await api.setSongdataDBPath(cfg.songdataDbPath);
    } catch (e: any) {
      saveError = String(e);
    } finally {
      saving = false;
    }
  }

  async function startServer() {
    busy = true;
    statusError = '';
    try { await api.startServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function stopServer() {
    busy = true;
    statusError = '';
    try { await api.stopServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function restartServer() {
    busy = true;
    statusError = '';
    try { await api.restartServer(); } catch (e: any) { statusError = String(e); }
    finally { busy = false; status = await api.getServerStatus(); }
  }

  async function reloadOwned() {
    ownedError = '';
    busy = true;
    try {
      await api.reloadOwnedCache();
      owned = await api.getOwnedCacheStatus();
    } catch (e: any) {
      ownedError = String(e);
      // 失敗時もステータスを取得（lastError が更新されている可能性）
      try { owned = await api.getOwnedCacheStatus(); } catch {}
    } finally {
      busy = false;
    }
  }

  function formatJST(iso: string): string {
    if (!iso) return '-';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString('ja-JP', {
      timeZone: 'Asia/Tokyo',
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
  }

  function stateLabel(s: ServerStatusDTO['state'] | undefined): string {
    switch (s) {
      case 'running': return '起動中';
      case 'stopped': return '停止中';
      case 'error': return 'エラー';
      default: return '不明';
    }
  }

  function stateClass(s: ServerStatusDTO['state'] | undefined): string {
    switch (s) {
      case 'running': return 'state ok';
      case 'stopped': return 'state stopped';
      case 'error': return 'state err';
      default: return 'state';
    }
  }

  onMount(() => {
    load();
    unsubscribeStatus = api.onServerStatusChanged((s) => (status = s));
  });
  onDestroy(() => {
    if (unsubscribeStatus) unsubscribeStatus();
  });
</script>

<section class="tab">
  <h2>サーバ設定</h2>

  <div class="block">
    <h3>基本設定</h3>
    <label>
      ポート番号
      <input type="number" min="1" max="65535" bind:value={cfg.port} />
    </label>
    <label>
      songdata.db パス
      <input type="text" bind:value={cfg.songdataDbPath} placeholder="/path/to/songdata.db" />
    </label>
    {#if saveError}
      <div class="error">{saveError}</div>
    {/if}
    <button on:click={save} disabled={saving}>保存</button>
  </div>

  <div class="block">
    <h3>サーバステータス</h3>
    {#if status}
      <p>
        <span class={stateClass(status.state)}>● {stateLabel(status.state)}</span>
        {#if status.state === 'running'}
          <code>http://127.0.0.1:{status.port}</code>
        {/if}
      </p>
      {#if status.startedAt}
        <p class="meta">起動: {formatJST(status.startedAt)}</p>
      {/if}
      {#if status.lastError}
        <p class="error">{status.lastError}</p>
      {/if}
    {/if}
    {#if statusError}<div class="error">{statusError}</div>{/if}
    <div class="actions">
      <button on:click={startServer} disabled={busy || status?.state === 'running'}>起動</button>
      <button on:click={stopServer} disabled={busy || status?.state !== 'running'}>停止</button>
      <button on:click={restartServer} disabled={busy}>再起動</button>
    </div>
  </div>

  <div class="block">
    <h3>所持譜面キャッシュ</h3>
    {#if owned}
      <p>読み込み済み: {owned.count.toLocaleString()} 曲</p>
      {#if owned.loadedAt}<p class="meta">最終読み込み: {formatJST(owned.loadedAt)}</p>{/if}
      {#if owned.loadedPath}<p class="meta">パス: <code>{owned.loadedPath}</code></p>{/if}
      {#if owned.lastError}<p class="error">{owned.lastError}</p>{/if}
    {/if}
    {#if ownedError}<div class="error">{ownedError}</div>{/if}
    <button on:click={reloadOwned} disabled={busy}>再読み込み</button>
  </div>
</section>

<style>
  .tab { padding: 12px 16px; }
  h2 { margin: 0 0 12px; font-size: 16px; }
  h3 { margin: 0 0 8px; font-size: 14px; }
  .block { border: 1px solid #ddd; padding: 12px; margin-bottom: 12px; background: #fafafa; }
  label { display: block; margin: 6px 0; font-size: 13px; }
  input[type="number"], input[type="text"] {
    width: 100%; box-sizing: border-box; padding: 4px 6px; font-size: 13px;
  }
  .actions { display: flex; gap: 6px; margin-top: 8px; }
  button { padding: 4px 10px; cursor: pointer; }
  .state { font-weight: bold; }
  .state.ok { color: #2c8a3e; }
  .state.stopped { color: #888; }
  .state.err { color: #b00020; }
  .meta { color: #666; font-size: 12px; }
  .error { color: #b00020; margin: 4px 0; }
  code { font-family: monospace; background: #eef; padding: 0 4px; }
</style>
