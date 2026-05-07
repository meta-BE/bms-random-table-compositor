<script lang="ts">
  import { onMount } from 'svelte';

  export let title: string;
  export let message: string;
  export let confirmLabel: string = 'OK';
  export let cancelLabel: string = 'キャンセル';
  export let danger: boolean = false;
  export let onResult: (ok: boolean) => void;

  let dialog: HTMLDialogElement;

  onMount(() => {
    dialog.showModal();
  });

  function handleConfirm() {
    dialog.close();
    onResult(true);
  }

  function handleCancel() {
    dialog.close();
    onResult(false);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      handleCancel();
    }
  }
</script>

<dialog bind:this={dialog} class="modal" on:keydown={handleKeydown}>
  <div class="modal-box">
    <h3 class="font-bold text-lg">{title}</h3>
    <p class="py-4 whitespace-pre-line">{message}</p>
    <div class="modal-action">
      <button class="btn" on:click={handleCancel}>{cancelLabel}</button>
      <button
        class="btn"
        class:btn-error={danger}
        class:btn-primary={!danger}
        on:click={handleConfirm}>{confirmLabel}</button>
    </div>
  </div>
</dialog>
