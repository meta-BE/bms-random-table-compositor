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
