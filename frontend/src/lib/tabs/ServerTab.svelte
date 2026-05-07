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
