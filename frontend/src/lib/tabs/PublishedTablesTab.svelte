<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    api,
    type PublishedTableDTO,
    type CreatePublishedTableRequest,
    type UpdatePublishedTableRequest,
    type SourceTableDTO,
    type ServerStatusDTO,
    type SlugValidation,
  } from '../api';

  let rows: PublishedTableDTO[] = [];
  let sources: SourceTableDTO[] = [];
  let serverStatus: ServerStatusDTO | null = null;
  let listError = '';
  let formError = '';
  let formMode: 'closed' | 'create' | 'edit' = 'closed';
  let editingId: string = '';
  let busy = false;
  let unsubscribeStatus: (() => void) | null = null;

  // フォーム状態
  let f = blankForm();
  let slugValidation: SlugValidation = { ok: true };
  let slugTimer: ReturnType<typeof setTimeout> | null = null;

  function blankForm(): CreatePublishedTableRequest {
    return {
      slug: '',
      displayName: '',
      symbol: '',
      sourceTableId: '',
      ownedOnly: false,
      pickPerLevel: 0,
      refreshMode: 'per_request',
    };
  }

  async function load() {
    listError = '';
    try {
      rows = await api.listPublishedTables();
      sources = await api.listSourceTables();
      serverStatus = await api.getServerStatus();
    } catch (e: any) {
      listError = `読み込みエラー: ${String(e)}`;
    }
  }

  function openCreate() {
    formMode = 'create';
    editingId = '';
    f = blankForm();
    formError = '';
    slugValidation = { ok: true };
  }

  function openEdit(row: PublishedTableDTO) {
    formMode = 'edit';
    editingId = row.id;
    f = {
      slug: row.slug,
      displayName: row.displayName,
      symbol: row.symbol,
      sourceTableId: row.sourceTableId,
      ownedOnly: row.ownedOnly,
      pickPerLevel: row.pickPerLevel,
      refreshMode: row.refreshMode,
    };
    formError = '';
    slugValidation = { ok: true };
  }

  function closeForm() {
    formMode = 'closed';
    editingId = '';
    formError = '';
  }

  function debounceValidateSlug() {
    if (slugTimer) clearTimeout(slugTimer);
    slugTimer = setTimeout(async () => {
      if (!f.slug) {
        slugValidation = { ok: false, reason: 'invalid_format' };
        return;
      }
      try {
        slugValidation = await api.validateSlug(f.slug, editingId);
      } catch (e: any) {
        slugValidation = { ok: false, reason: String(e) };
      }
    }, 300);
  }

  async function suggestSlug() {
    if (!f.sourceTableId) return;
    try {
      f.slug = await api.suggestSlugFromSource(f.sourceTableId);
      debounceValidateSlug();
    } catch (e: any) {
      formError = `slug 候補の生成に失敗: ${String(e)}`;
    }
  }

  async function submitForm() {
    formError = '';
    if (!f.displayName) {
      formError = '表示名は必須です';
      return;
    }
    if (!f.sourceTableId) {
      formError = 'ソース表を選択してください';
      return;
    }
    if (!slugValidation.ok) {
      formError = `slug が無効: ${slugValidation.reason}`;
      return;
    }
    busy = true;
    try {
      if (formMode === 'create') {
        await api.createPublishedTable(f);
      } else if (formMode === 'edit') {
        const req: UpdatePublishedTableRequest = { ...f, id: editingId, sortOrder: 0 };
        await api.updatePublishedTable(req);
      }
      closeForm();
      await load();
    } catch (e: any) {
      formError = String(e);
    } finally {
      busy = false;
    }
  }

  async function remove(id: string) {
    // Plan 2 lessons: window.confirm は Wails WebView で機能しないため即削除
    busy = true;
    try {
      await api.deletePublishedTable(id);
      await load();
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = false;
    }
  }

  async function manualRefresh(id: string) {
    busy = true;
    try {
      await api.manualRefreshPick(id);
    } catch (e: any) {
      listError = String(e);
    } finally {
      busy = false;
    }
  }

  async function openInBrowser(slug: string) {
    if (!serverStatus || serverStatus.state !== 'running') {
      listError = 'サーバが起動していません';
      return;
    }
    try {
      await api.openPublishedTableURL(slug, serverStatus.port);
    } catch (e: any) {
      listError = String(e);
    }
  }

  function sourceLabel(id: string): string {
    const s = sources.find((x) => x.id === id);
    if (!s) return id;
    const name = s.displayName || s.name || s.inputUrl;
    if (s.lastFetchStatus === 'never') return `${name} (未取得)`;
    return name;
  }

  onMount(() => {
    load();
    unsubscribeStatus = api.onServerStatusChanged((s) => (serverStatus = s));
  });

  onDestroy(() => {
    if (unsubscribeStatus) unsubscribeStatus();
    if (slugTimer) clearTimeout(slugTimer);
  });
</script>

<section class="tab">
  <h2>公開表</h2>

  {#if listError}
    <div class="error">{listError}</div>
  {/if}

  {#if formMode === 'closed'}
    <button class="primary" on:click={openCreate} disabled={busy}>+ 公開表を追加</button>
  {/if}

  {#if formMode !== 'closed'}
    <div class="form">
      <h3>{formMode === 'create' ? '公開表を追加' : '公開表を編集'}</h3>
      <label>
        表示名
        <input type="text" bind:value={f.displayName} />
      </label>
      <label>
        ソース表
        <select bind:value={f.sourceTableId}>
          <option value="">— 選択 —</option>
          {#each sources as s}
            <option value={s.id}>{sourceLabel(s.id)}</option>
          {/each}
        </select>
      </label>
      <label>
        Slug
        <span class="slug-row">
          <input type="text" bind:value={f.slug} on:input={debounceValidateSlug} />
          <button type="button" on:click={suggestSlug} disabled={!f.sourceTableId}>ソース表名から生成</button>
        </span>
        {#if !slugValidation.ok}
          <span class="slug-err">slug が無効: {slugValidation.reason}</span>
        {/if}
      </label>
      <label>
        Symbol
        <input type="text" bind:value={f.symbol} />
      </label>
      <label class="checkbox">
        <input type="checkbox" bind:checked={f.ownedOnly} />
        所持譜面のみ表示する
      </label>
      <label>
        レベルあたりの最大曲数 (0 = 無制限)
        <input type="number" min="0" bind:value={f.pickPerLevel} />
      </label>
      <label>
        ピック更新モード
        <select bind:value={f.refreshMode}>
          <option value="per_request">per_request (アクセス毎)</option>
          <option value="daily">daily (1 日 1 回)</option>
          <option value="manual">manual (手動)</option>
        </select>
      </label>
      {#if formError}
        <div class="error">{formError}</div>
      {/if}
      <div class="actions">
        <button on:click={submitForm} disabled={busy}>保存</button>
        <button on:click={closeForm} disabled={busy}>キャンセル</button>
      </div>
    </div>
  {/if}

  {#if rows.length === 0}
    <p class="empty">公開表が登録されていません。</p>
  {:else}
    <table>
      <thead>
        <tr><th>表示名</th><th>Slug</th><th>ソース表</th><th>所持限定</th><th>各レベル</th><th>モード</th><th></th></tr>
      </thead>
      <tbody>
        {#each rows as row}
          <tr>
            <td>{row.displayName}</td>
            <td><code>/{row.slug}</code></td>
            <td>{sourceLabel(row.sourceTableId)}</td>
            <td>{row.ownedOnly ? '✓' : ''}</td>
            <td>{row.pickPerLevel === 0 ? '無制限' : row.pickPerLevel}</td>
            <td>{row.refreshMode}</td>
            <td class="ops">
              <button on:click={() => openInBrowser(row.slug)} disabled={busy}>開く</button>
              <button on:click={() => openEdit(row)} disabled={busy}>編集</button>
              {#if row.refreshMode === 'manual'}
                <button on:click={() => manualRefresh(row.id)} disabled={busy}>再ピック</button>
              {/if}
              <button class="danger" on:click={() => remove(row.id)} disabled={busy}>削除</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<style>
  .tab { padding: 12px 16px; }
  h2 { margin: 0 0 12px; font-size: 16px; }
  .error { color: #b00020; margin: 8px 0; }
  .empty { color: #999; }
  .form { border: 1px solid #ccc; padding: 12px; margin: 12px 0; background: #fafafa; }
  .form h3 { margin: 0 0 8px; font-size: 14px; }
  .form label { display: block; margin: 6px 0; font-size: 13px; }
  .form label.checkbox { display: flex; align-items: center; gap: 6px; }
  .form input[type="text"], .form select, .form input[type="number"] {
    width: 100%; box-sizing: border-box; padding: 4px 6px; font-size: 13px;
  }
  .slug-row { display: flex; gap: 6px; }
  .slug-row input { flex: 1; }
  .slug-err { color: #b00020; font-size: 12px; }
  .actions { display: flex; gap: 6px; margin-top: 8px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { padding: 4px 8px; border-bottom: 1px solid #eee; text-align: left; }
  td.ops { display: flex; gap: 4px; }
  button { padding: 3px 8px; cursor: pointer; }
  button.primary { background: #1b2636; color: #fff; padding: 6px 12px; }
  button.danger { color: #b00020; }
</style>
