<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { ClipboardSetText } from '../../../wailsjs/runtime/runtime';
  import {
    api,
    type PublishedTableDTO,
    type SourceTableDTO,
    type RefreshMode,
    type ServerConfig,
    type PublishedTableLevelInputDTO,
  } from '../api';
  import { confirm } from '../components/confirm';
  import ContextMenu, { type MenuItem } from '../components/ContextMenu.svelte';
  import PublishedTableLevelEditor from '../components/PublishedTableLevelEditor.svelte';

  type FormMode = 'create' | { kind: 'edit'; id: string };
  type CreateKind = 'wizard' | 'blank';

  let rows: PublishedTableDTO[] = [];
  let sources: SourceTableDTO[] = [];
  let loading = true;
  let listError = '';

  // 作成方法選択ダイアログ
  let createPickerOpen = false;

  let formMode: FormMode = 'create';
  let formOpen = false;
  let createKind: CreateKind = 'blank';
  let wizardSourceId = '';

  let form = {
    slug: '',
    displayName: '',
    symbol: '',
    ownedOnly: false,
    refreshMode: 'manual' as RefreshMode,
    sortOrder: 0,
    levels: [] as PublishedTableLevelInputDTO[],
  };
  let formError = '';
  let saving = false;
  let slugStatus: 'idle' | 'ok' | 'invalid_format' | 'reserved' | 'duplicate' = 'idle';
  let slugDirty = false;

  let serverPort = 50000;

  let menu: ContextMenu;

  let toastMsg = '';
  let toastKind: 'success' | 'error' = 'success';
  let toastTimer: ReturnType<typeof setTimeout> | undefined;

  function showToast(msg: string, kind: 'success' | 'error' = 'success') {
    toastMsg = msg;
    toastKind = kind;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toastMsg = ''; }, 2500);
  }

  onDestroy(() => {
    if (toastTimer) clearTimeout(toastTimer);
  });

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
    try {
      sources = await api.listSourceTables();
    } catch (e) {
      console.warn(e);
    }
  }

  async function loadServerCfg() {
    try {
      const cfg: ServerConfig = await api.getServerConfig();
      serverPort = cfg.port;
    } catch (e) {
      console.warn(e);
    }
  }

  function openCreatePicker() {
    if (sources.length > 0) {
      wizardSourceId = sources[0].id;
      createKind = 'wizard';
    } else {
      createKind = 'blank';
    }
    createPickerOpen = true;
  }

  function startCreateWithKind() {
    createPickerOpen = false;
    formMode = 'create';
    form = {
      slug: '',
      displayName: '',
      symbol: '',
      ownedOnly: false,
      refreshMode: 'manual',
      sortOrder: 0,
      levels: [],
    };
    slugStatus = 'idle';
    slugDirty = false;
    formError = '';
    formOpen = true;
  }

  async function openEdit(row: PublishedTableDTO) {
    try {
      const full = await api.getPublishedTable(row.id);
      formMode = { kind: 'edit', id: row.id };
      form = {
        slug: full.slug,
        displayName: full.displayName,
        symbol: full.symbol,
        ownedOnly: full.ownedOnly,
        refreshMode: full.refreshMode,
        sortOrder: full.sortOrder,
        levels: full.levels.map((lv) => ({
          name: lv.name,
          perMappingPick: lv.perMappingPick,
          totalPick: lv.totalPick,
          mappings: lv.mappings.map((mp) => ({
            sourceTableId: mp.sourceTableId,
            sourceLevel: mp.sourceLevel,
          })),
        })),
      };
      slugStatus = 'ok';
      slugDirty = true;
      formError = '';
      formOpen = true;
    } catch (e) {
      showToast(`公開表の取得に失敗: ${(e as Error).message}`, 'error');
    }
  }

  function closeForm() {
    formOpen = false;
  }

  async function suggestSlug() {
    const sourceId = createKind === 'wizard' ? wizardSourceId : sources[0]?.id ?? '';
    if (!sourceId) return;
    try {
      const s = await api.suggestSlugFromSource(sourceId);
      form.slug = s;
      slugDirty = true;
      await checkSlug();
    } catch (e) {
      console.warn(e);
    }
  }

  async function checkSlug() {
    if (!form.slug) {
      slugStatus = 'idle';
      return;
    }
    try {
      const excludeId = formMode === 'create' ? '' : formMode.id;
      const v = await api.validateSlug(form.slug, excludeId);
      if (v.ok) {
        slugStatus = 'ok';
      } else {
        slugStatus = v.reason as typeof slugStatus;
      }
    } catch (e) {
      slugStatus = 'invalid_format';
    }
  }

  async function save() {
    if (!form.displayName.trim()) {
      formError = '表示名は必須です';
      return;
    }
    if (slugStatus !== 'ok') {
      formError = 'slug が不正です';
      return;
    }
    saving = true;
    formError = '';
    try {
      if (formMode === 'create' && createKind === 'wizard') {
        if (!wizardSourceId) {
          formError = 'ソース表を選択してください';
          saving = false;
          return;
        }
        await api.createPublishedTableFromSource({
          sourceTableId: wizardSourceId,
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
        });
      } else if (formMode === 'create') {
        await api.createPublishedTable({
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          ownedOnly: form.ownedOnly,
          refreshMode: form.refreshMode,
          levels: form.levels,
        });
      } else {
        await api.updatePublishedTable({
          id: formMode.id,
          slug: form.slug,
          displayName: form.displayName,
          symbol: form.symbol,
          ownedOnly: form.ownedOnly,
          refreshMode: form.refreshMode,
          sortOrder: form.sortOrder,
          levels: form.levels,
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
    } catch (e) {
      console.warn(e);
    }
  }

  async function openInBrowser(row: PublishedTableDTO) {
    try {
      await api.openPublishedTableURL(row.slug, serverPort);
    } catch (e) {
      console.warn(e);
    }
  }

  async function copyURL(row: PublishedTableDTO) {
    const url = `http://127.0.0.1:${serverPort}/${row.slug}`;
    try {
      await ClipboardSetText(url);
      showToast(`「${row.displayName}」のURLをコピーしました`, 'success');
    } catch (e) {
      console.warn(e);
      showToast('URLのコピーに失敗しました', 'error');
    }
  }

  async function manualRefresh(row: PublishedTableDTO) {
    try {
      await api.manualRefreshPick(row.id);
    } catch (e) {
      console.warn(e);
    }
  }

  function onRowContextMenu(e: MouseEvent, row: PublishedTableDTO) {
    const items: MenuItem[] = [
      { label: '編集', onClick: () => void openEdit(row) },
      { label: 'ブラウザで開く', onClick: () => void openInBrowser(row) },
      { label: 'URLをコピー', onClick: () => void copyURL(row) },
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
    <button class="btn btn-primary btn-sm" on:click={openCreatePicker}>新規作成</button>
  </div>

  {#if sources.length === 0}
    <div class="alert alert-warning text-sm">「ソース表からウィザード生成」を使うには、まず「ソース表」タブでソース表を 1 つ以上登録してください。</div>
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
                <th>所持絞り込み</th>
                <th>更新</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row (row.id)}
                <tr on:contextmenu={(e) => onRowContextMenu(e, row)}>
                  <td>{row.displayName}</td>
                  <td
                    class="font-mono text-xs cursor-pointer hover:bg-base-200"
                    title="クリックでURLをコピー"
                    on:click={() => copyURL(row)}
                  >{row.slug}</td>
                  <td>{row.symbol}</td>
                  <td>{row.ownedOnly ? '有' : '無'}</td>
                  <td><span class="badge badge-ghost">{modeLabel(row.refreshMode)}</span></td>
                  <td class="whitespace-nowrap">
                    <button class="btn btn-xs" on:click={() => openInBrowser(row)}>開く</button>
                    <button class="btn btn-xs" on:click={() => void openEdit(row)}>編集</button>
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

<!-- 作成方法選択ダイアログ -->
{#if createPickerOpen}
  <dialog class="modal modal-open">
    <div class="modal-box max-w-md">
      <h3 class="font-bold text-base">公開表の作成方法</h3>
      <div class="space-y-3 mt-3 text-sm">
        <label class="label cursor-pointer justify-start gap-3 {sources.length === 0 ? 'opacity-50' : ''}">
          <input type="radio" class="radio radio-sm" bind:group={createKind} value="wizard" disabled={sources.length === 0} />
          <span class="label-text">ソース表からウィザード生成（推奨）</span>
        </label>
        {#if createKind === 'wizard'}
          <div class="ml-7">
            <select class="select select-bordered select-sm w-full" bind:value={wizardSourceId}>
              {#each sources as s}
                <option value={s.id}>{s.displayName || s.name || s.inputUrl}</option>
              {/each}
            </select>
            <p class="text-xs opacity-70 mt-1">選んだソース表の各レベルが公開レベルとして自動生成されます。</p>
          </div>
        {/if}
        <label class="label cursor-pointer justify-start gap-3">
          <input type="radio" class="radio radio-sm" bind:group={createKind} value="blank" />
          <span class="label-text">ブランクから作成</span>
        </label>
        {#if createKind === 'blank'}
          <p class="text-xs opacity-70 ml-7">公開レベルとマッピングを手で組み立てます。</p>
        {/if}
      </div>
      <div class="modal-action">
        <button class="btn btn-sm" on:click={() => (createPickerOpen = false)}>キャンセル</button>
        <button class="btn btn-primary btn-sm" on:click={startCreateWithKind} disabled={createKind === 'wizard' && !wizardSourceId}>次へ</button>
      </div>
    </div>
  </dialog>
{/if}

<!-- 作成 / 編集モーダル -->
{#if formOpen}
  <dialog class="modal modal-open">
    <div class="modal-box max-w-4xl">
      <h3 class="font-bold text-base">{formMode === 'create' ? (createKind === 'wizard' ? 'ウィザード生成: 公開表' : 'ブランク作成: 公開表') : '公開表を編集'}</h3>
      <div class="space-y-3 mt-2 text-sm">
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
        <label class="label cursor-pointer justify-start gap-3">
          <input type="checkbox" class="checkbox checkbox-sm" bind:checked={form.ownedOnly} />
          <span class="label-text">所持譜面のみ表示</span>
        </label>
        <div class="form-control w-full">
          <label class="label py-1"><span class="label-text">更新モード</span></label>
          <select class="select select-bordered select-sm w-full" bind:value={form.refreshMode}>
            <option value="per_request">毎回(リクエスト毎に再生成)</option>
            <option value="daily">日次 (同一日付内で固定)</option>
            <option value="manual">手動 (再ピックボタンまで固定)</option>
          </select>
        </div>

        {#if !(formMode === 'create' && createKind === 'wizard')}
          <!-- ウィザード作成時はレベル編集を省略（保存後に編集モーダルで調整） -->
          <div class="divider my-2"></div>
          <PublishedTableLevelEditor bind:levels={form.levels} {sources} />
        {:else}
          <div class="alert alert-info text-xs">
            ウィザード生成: ソース表「{sources.find(s => s.id === wizardSourceId)?.displayName || sources.find(s => s.id === wizardSourceId)?.name}」のレベル体系を反映した公開表が作成されます。詳細編集は保存後に「編集」から行えます。
          </div>
        {/if}
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

{#if toastMsg}
  <div class="toast toast-center toast-bottom z-50">
    <div class="alert {toastKind === 'success' ? 'bg-primary text-primary-content' : 'bg-error text-error-content'}">
      <span>{toastMsg}</span>
    </div>
  </div>
{/if}
