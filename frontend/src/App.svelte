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
