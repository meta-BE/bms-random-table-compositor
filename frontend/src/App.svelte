<script lang="ts">
  import ServerTab from './lib/tabs/ServerTab.svelte';
</script>

<main>
  <header>
    <h1>BMS Random Table Compositor</h1>
  </header>
  <ServerTab />
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    color: #1b2636;
    min-height: 100vh;
  }
  header {
    padding: 16px;
    border-bottom: 1px solid #e0e0e0;
  }
  header h1 { margin: 0; font-size: 18px; }
</style>
