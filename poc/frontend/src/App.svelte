<script lang="ts">
  import { onMount } from 'svelte';
  import { GetConfig, SaveAndStart, Stop, GetStatus, OpenURL } from '../wailsjs/go/main/App';

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

  function onOpen() {
    if (running && runningPort > 0) {
      OpenURL(`http://localhost:${runningPort}/`);
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
      <button class="link-btn" on:click={onOpen}>開く</button>
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
  .link-btn {
    background: none;
    border: none;
    color: #1565c0;
    text-decoration: underline;
    cursor: pointer;
    padding: 0;
    font: inherit;
  }
  .link-btn:hover {
    color: #0d47a1;
  }
</style>
