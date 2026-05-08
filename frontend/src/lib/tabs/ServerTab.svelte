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
        <div class="form-control w-full">
          <label class="label"><span class="label-text">ポート番号</span></label>
          <input
            class="input input-bordered input-sm w-40"
            type="number"
            min="1"
            max="65535"
            bind:value={cfg.port}
          />
        </div>

        <div class="form-control w-full">
          <label class="label"><span class="label-text">songdata.db のパス</span></label>
          <div class="join w-full">
            <input class="input input-bordered input-sm join-item flex-1" type="text" bind:value={cfg.songdataDbPath} />
            <button class="btn btn-sm join-item" type="button" on:click={pickPath}>参照…</button>
          </div>
        </div>

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
          <span class="badge badge-primary">稼働中</span>
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
