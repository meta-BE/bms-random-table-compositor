<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PublishedTableDTO, type SourceTableDTO, type RefreshMode, type ServerConfig } from '../api';
  import { confirm } from '../components/confirm';
  import ContextMenu, { type MenuItem } from '../components/ContextMenu.svelte';

  type FormMode = 'create' | { kind: 'edit'; id: string };

  let rows: PublishedTableDTO[] = [];
  let sources: SourceTableDTO[] = [];
  let loading = true;
  let listError = '';

  let formMode: FormMode = 'create';
  let formOpen = false;
  let form = {
    slug: '',
    displayName: '',
    symbol: '',
    sourceTableId: '',
    ownedOnly: false,
    pickPerLevel: 0,
    refreshMode: 'per_request' as RefreshMode,
    sortOrder: 0,
  };
  let formError = '';
  let saving = false;
  let slugStatus: 'idle' | 'ok' | 'invalid_format' | 'reserved' | 'duplicate' = 'idle';
  let slugDirty = false;

  let serverPort = 50000;

  let menu: ContextMenu;

  onMount(async () => {
    await Promise.all([reload(), loadSources(), loadServerCfg()]);
  });

  async function reload() {
    loading = true;
    listError = '';
    try {
      rows = await api.listPublishedTables();
    } catch (e) {
      listError = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function loadSources() {
    try { sources = await api.listSourceTables(); } catch (e) { console.warn(e); }
  }

  async function loadServerCfg() {
    try {
      const cfg: ServerConfig = await api.getServerConfig();
      serverPort = cfg.port;
    } catch (e) { console.warn(e); }
  }

  function openCreate() {
    formMode = 'create';
    form = {
      slug: '', displayName: '', symbol: '', sourceTableId: sources[0]?.id ?? '',
      ownedOnly: false, pickPerLevel: 0, refreshMode: 'per_request', sortOrder: 0,
    };
    slugStatus = 'idle';
    slugDirty = false;
    formError = '';
    formOpen = true;
  }

  function openEdit(row: PublishedTableDTO) {
    formMode = { kind: 'edit', id: row.id };
    form = { ...row };
    // 既存 slug は DB に保存済み = valid とみなす。slug を変更すると on:input で checkSlug が再実行される。
    slugStatus = 'ok';
    slugDirty = true;
    formError = '';
    formOpen = true;
  }

  function closeForm() {
    formOpen = false;
  }

  async function suggestSlug() {
    if (!form.sourceTableId) return;
    try {
      const s = await api.suggestSlugFromSource(form.sourceTableId);
      form.slug = s;
      slugDirty = true;
      await checkSlug();
    } catch (e) { console.warn(e); }
  }

  async function checkSlug() {
    if (!form.slug) { slugStatus = 'idle'; return; }
    try {
      const excludeId = formMode === 'create' ? '' : formMode.id;
      const v = await api.validateSlug(form.slug, excludeId);
      if (v.ok) { slugStatus = 'ok'; }
      else { slugStatus = (v.reason as typeof slugStatus); }
    } catch (e) {
      slugStatus = 'invalid_format';
    }
  }

  async function save() {
    if (!form.displayName.trim()) { formError = '表示名は必須です'; return; }
    if (!form.sourceTableId) { formError = 'ソース表を選択してください'; return; }
    if (slugStatus !== 'ok') { formError = 'slug が不正です'; return; }
    saving = true;
    formError = '';
    try {
      if (formMode === 'create') {
        await api.createPublishedTable({
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          sourceTableId: form.sourceTableId,
          ownedOnly: form.ownedOnly,
          pickPerLevel: form.pickPerLevel,
          refreshMode: form.refreshMode,
        });
      } else {
        await api.updatePublishedTable({
          id: formMode.id,
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          sourceTableId: form.sourceTableId,
          ownedOnly: form.ownedOnly,
          pickPerLevel: form.pickPerLevel,
          refreshMode: form.refreshMode,
          sortOrder: form.sortOrder,
        });
      }
      formOpen = false;
      await reload();
    } catch (e) {
      formError = (e as Error).message;
    } finally {
      saving = false;
    }
  }

  async function remove(row: PublishedTableDTO) {
    const ok = await confirm({
      title: '公開表を削除',
      message: `公開表「${row.displayName}」(slug: ${row.slug}) を削除します。続行しますか？`,
      confirmLabel: '削除',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.deletePublishedTable(row.id);
      await reload();
    } catch (e) { console.warn(e); }
  }

  async function openInBrowser(row: PublishedTableDTO) {
    try { await api.openPublishedTableURL(row.slug, serverPort); } catch (e) { console.warn(e); }
  }

  async function manualRefresh(row: PublishedTableDTO) {
    try { await api.manualRefreshPick(row.id); } catch (e) { console.warn(e); }
  }

  function onRowContextMenu(e: MouseEvent, row: PublishedTableDTO) {
    const items: MenuItem[] = [
      { label: '編集', onClick: () => openEdit(row) },
      { label: 'ブラウザで開く', onClick: () => void openInBrowser(row) },
      { label: '再ピック', disabled: row.refreshMode !== 'manual', onClick: () => void manualRefresh(row) },
      { label: '削除', danger: true, onClick: () => void remove(row) },
    ];
    menu.open(e, items);
  }

  function modeLabel(m: RefreshMode): string {
    return m === 'per_request' ? '毎回' : m === 'daily' ? '日次' : '手動';
  }
</script>

<section class="p-4 space-y-4">
  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold">公開表</h2>
    <button class="btn btn-primary btn-sm" on:click={openCreate} disabled={sources.length === 0}>新規作成</button>
  </div>

  {#if sources.length === 0}
    <div class="alert alert-warning text-sm">公開表を作成するには、まず「ソース表」タブでソース表を 1 つ以上登録してください。</div>
  {/if}

  <div class="card bg-base-100 shadow-sm border border-base-200">
    <div class="card-body">
      {#if loading}
        <div class="flex items-center gap-2 text-sm py-4">
          <span class="loading loading-spinner loading-sm"></span>
          <span>読み込み中…</span>
        </div>
      {:else if listError}
        <div class="alert alert-error text-sm">{listError}</div>
      {:else if rows.length === 0}
        <div class="text-sm opacity-70 py-4">公開表がまだ作成されていません。「新規作成」から始めてください。</div>
      {:else}
        <div class="overflow-x-auto">
          <table class="table table-sm">
            <thead>
              <tr>
                <th>表示名</th>
                <th>slug</th>
                <th>シンボル</th>
                <th>ソース表</th>
                <th>所持絞り込み</th>
                <th>件数/レベル</th>
                <th>更新</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row (row.id)}
                {@const src = sources.find((s) => s.id === row.sourceTableId)}
                <tr on:contextmenu={(e) => onRowContextMenu(e, row)}>
                  <td>{row.displayName}</td>
                  <td class="font-mono text-xs">{row.slug}</td>
                  <td>{row.symbol}</td>
                  <td class="text-xs">{src?.displayName || src?.name || '(削除済み)'}</td>
                  <td>{row.ownedOnly ? '有' : '無'}</td>
                  <td>{row.pickPerLevel === 0 ? '無制限' : row.pickPerLevel}</td>
                  <td><span class="badge badge-ghost">{modeLabel(row.refreshMode)}</span></td>
                  <td class="whitespace-nowrap">
                    <button class="btn btn-xs" on:click={() => openInBrowser(row)}>開く</button>
                    <button class="btn btn-xs" on:click={() => openEdit(row)}>編集</button>
                    <button class="btn btn-xs btn-error btn-outline" on:click={() => remove(row)}>削除</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</section>

<!-- 作成 / 編集モーダル -->
{#if formOpen}
  <dialog class="modal modal-open">
    <div class="modal-box max-w-2xl">
      <h3 class="font-bold text-base">{formMode === 'create' ? '新規公開表' : '公開表を編集'}</h3>
      <div class="space-y-2 mt-2 text-sm">
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">表示名</span></label>
          <input class="input input-bordered input-sm w-full" bind:value={form.displayName} />
        </div>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">slug (URL に使われる)</span></label>
          <div class="join w-full">
            <input
              class="input input-bordered input-sm join-item flex-1"
              bind:value={form.slug}
              on:input={() => { slugDirty = true; void checkSlug(); }}
            />
            <button class="btn btn-sm join-item" type="button" on:click={suggestSlug}>自動生成</button>
          </div>
          {#if slugDirty && slugStatus !== 'idle' && slugStatus !== 'ok'}
            <div class="text-xs text-error mt-1">
              {slugStatus === 'invalid_format' ? '形式が不正 (a-z, 0-9, ハイフンのみ、先頭は英数字、最大63文字)' :
               slugStatus === 'reserved' ? '予約語です' :
               slugStatus === 'duplicate' ? '既に使われています' : ''}
            </div>
          {/if}
        </div>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">シンボル (例: ★, ▲)</span></label>
          <input class="input input-bordered input-sm w-32" bind:value={form.symbol} />
        </div>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">ソース表</span></label>
          <select class="select select-bordered select-sm w-full" bind:value={form.sourceTableId}>
            {#each sources as s}
              <option value={s.id}>{s.displayName || s.name || s.inputUrl}</option>
            {/each}
          </select>
        </div>
        <label class="label cursor-pointer justify-start gap-3">
          <input type="checkbox" class="checkbox checkbox-sm" bind:checked={form.ownedOnly} />
          <span class="label-text">所持譜面のみ表示</span>
        </label>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">レベル毎の件数 (0=無制限)</span></label>
          <input class="input input-bordered input-sm w-32" type="number" min="0" bind:value={form.pickPerLevel} />
        </div>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">更新モード</span></label>
          <select class="select select-bordered select-sm w-full" bind:value={form.refreshMode}>
            <option value="per_request">毎回(リクエスト毎に再生成)</option>
            <option value="daily">日次 (同一日付内で固定)</option>
            <option value="manual">手動 (再ピックボタンまで固定)</option>
          </select>
        </div>
      </div>
      {#if formError}<div class="alert alert-error text-sm mt-3">{formError}</div>{/if}
      <div class="modal-action">
        <button class="btn btn-sm" on:click={closeForm}>キャンセル</button>
        <button class="btn btn-primary btn-sm" disabled={saving} on:click={save}>
          {#if saving}<span class="loading loading-spinner loading-xs"></span>{/if}
          保存
        </button>
      </div>
    </div>
  </dialog>
{/if}

<ContextMenu bind:this={menu} />
