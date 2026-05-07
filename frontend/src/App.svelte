<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
  import SourceTablesTab from './lib/tabs/SourceTablesTab.svelte';

  type TabKey = 'server' | 'source-tables';
  let active: TabKey = 'server';
</script>

<main>
  <header>
    <h1>BMS Random Table Compositor</h1>
    <nav>
      <button class:active={active === 'server'} on:click={() => (active = 'server')}>サーバ設定</button>
      <button class:active={active === 'source-tables'} on:click={() => (active = 'source-tables')}>ソース表</button>
    </nav>
  </header>
  {#if active === 'server'}
    <ServerTab />
  {:else if active === 'source-tables'}
    <SourceTablesTab />
  {/if}
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    color: #1b2636;
    min-height: 100vh;
  }
  header {
    padding: 12px 16px;
    border-bottom: 1px solid #e0e0e0;
  }
  header h1 { margin: 0 0 8px; font-size: 18px; }
  nav { display: flex; gap: 4px; }
  nav button {
    padding: 4px 12px;
    border: 1px solid #ccc;
    background: #fafafa;
    cursor: pointer;
    font-size: 13px;
  }
  nav button.active {
    background: #1b2636;
    color: #fff;
    border-color: #1b2636;
  }
</style>
