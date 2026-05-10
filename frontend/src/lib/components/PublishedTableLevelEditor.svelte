<script lang="ts">
  import type {
    PublishedTableLevelInputDTO,
    PublishedTableLevelMappingInputDTO,
    SourceTableDTO,
  } from '../api';

  // 親から levels と sources を受け取り、双方向バインディング
  export let levels: PublishedTableLevelInputDTO[] = [];
  export let sources: SourceTableDTO[] = [];

  // バルク適用入力（ローカル状態）
  let bulkM = 0;
  let bulkN = 0;

  function addLevel() {
    levels = [
      ...levels,
      {
        name: `Lv${levels.length + 1}`,
        perMappingPick: 0,
        totalPick: 0,
        mappings: [],
      },
    ];
  }

  function removeLevel(i: number) {
    levels = levels.filter((_, idx) => idx !== i);
  }

  function moveLevel(i: number, delta: number) {
    const j = i + delta;
    if (j < 0 || j >= levels.length) return;
    const next = [...levels];
    [next[i], next[j]] = [next[j], next[i]];
    levels = next;
  }

  function addMapping(i: number) {
    if (sources.length === 0) return;
    const next = [...levels];
    next[i] = {
      ...next[i],
      mappings: [
        ...next[i].mappings,
        { sourceTableId: sources[0].id, sourceLevel: '' },
      ],
    };
    levels = next;
  }

  function removeMapping(i: number, j: number) {
    const next = [...levels];
    next[i] = {
      ...next[i],
      mappings: next[i].mappings.filter((_, idx) => idx !== j),
    };
    levels = next;
  }

  function applyBulk() {
    levels = levels.map((lv) => ({
      ...lv,
      perMappingPick: bulkM,
      totalPick: bulkN,
    }));
  }

  function sourceLevelOptions(sourceId: string): { value: string; label: string }[] {
    const s = sources.find((x) => x.id === sourceId);
    if (!s) return [];
    const symbol = s.symbol ?? '';
    return s.levelOrder.map((lvl) => ({ value: lvl, label: `${symbol}${lvl}` }));
  }

  // バリデーション警告
  function levelHasOverflow(lv: PublishedTableLevelInputDTO): boolean {
    return lv.totalPick > 0 && lv.perMappingPick * lv.mappings.length >= lv.totalPick;
  }
  function levelHasNoMappings(lv: PublishedTableLevelInputDTO): boolean {
    return lv.mappings.length === 0;
  }
</script>

<div class="space-y-4">
  <!-- バルク適用パネル -->
  <div class="bg-base-200 p-3 rounded">
    <div class="font-semibold mb-2 text-sm">全レベル一括適用</div>
    <div class="flex gap-2 items-end flex-wrap">
      <label class="form-control">
        <span class="label-text text-xs">レベルごとピック曲数 (m)</span>
        <input type="number" min="0" class="input input-bordered input-sm w-24" bind:value={bulkM} />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">全体ピック曲数 (n)</span>
        <input type="number" min="0" class="input input-bordered input-sm w-24" bind:value={bulkN} />
      </label>
      <button class="btn btn-sm btn-primary" type="button" on:click={applyBulk}>全レベルに適用</button>
    </div>
    <p class="text-xs opacity-70 mt-2">
      各マッピングから m 曲を最低保証し、合計が n 曲になるよう全体プールから補填します
      （n=0 または m × マッピング数 ≥ n のときは補填なし）。
    </p>
  </div>

  <!-- レベル一覧テーブル -->
  <div>
    <div class="flex justify-between items-center mb-2">
      <h3 class="font-semibold text-sm">公開レベル一覧</h3>
      <button class="btn btn-sm btn-outline" type="button" on:click={addLevel}>+ レベル追加</button>
    </div>
    {#if levels.length === 0}
      <div class="text-sm opacity-70 py-4">公開レベルがありません。「+ レベル追加」から作成してください。</div>
    {:else}
      <div class="overflow-x-auto">
        <table class="table table-zebra table-sm">
          <thead>
            <tr>
              <th>並び</th>
              <th>名前</th>
              <th>マッピング</th>
              <th title="レベルごとピック曲数（マッピング 1 件あたり）">m</th>
              <th title="全体ピック曲数（公開レベル合計目標）">n</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each levels as lv, i (i)}
              <tr>
                <td class="whitespace-nowrap">
                  <button class="btn btn-xs" type="button" on:click={() => moveLevel(i, -1)} disabled={i === 0}>▲</button>
                  <button class="btn btn-xs" type="button" on:click={() => moveLevel(i, 1)} disabled={i === levels.length - 1}>▼</button>
                </td>
                <td>
                  <input type="text" class="input input-bordered input-xs w-32" bind:value={lv.name} />
                </td>
                <td>
                  <div class="flex flex-wrap gap-1 items-center">
                    {#each lv.mappings as mp, j (j)}
                      <div class="badge badge-outline gap-1 p-2">
                        <select class="select select-xs" bind:value={mp.sourceTableId}>
                          {#each sources as s}
                            <option value={s.id}>{s.displayName || s.name || s.inputUrl}</option>
                          {/each}
                        </select>
                        <select class="select select-xs" bind:value={mp.sourceLevel}>
                          <option value="">(未選択)</option>
                          {#each sourceLevelOptions(mp.sourceTableId) as opt}
                            <option value={opt.value}>{opt.label}</option>
                          {/each}
                        </select>
                        <button class="btn btn-xs btn-ghost" type="button" on:click={() => removeMapping(i, j)}>✕</button>
                      </div>
                    {/each}
                    <button class="btn btn-xs btn-outline" type="button" on:click={() => addMapping(i)} disabled={sources.length === 0}>+</button>
                  </div>
                  {#if levelHasNoMappings(lv)}
                    <div class="text-xs text-warning mt-1">マッピングが 0 件です。ピック結果は空になります。</div>
                  {/if}
                </td>
                <td>
                  <input type="number" min="0" class="input input-bordered input-xs w-16" bind:value={lv.perMappingPick} />
                </td>
                <td>
                  <input type="number" min="0" class="input input-bordered input-xs w-16" bind:value={lv.totalPick} />
                  {#if levelHasOverflow(lv)}
                    <div class="text-xs text-warning mt-1" title="m × マッピング数 ≥ n のため n は無視されます">⚠ n 無効</div>
                  {/if}
                </td>
                <td>
                  <button class="btn btn-xs btn-error btn-outline" type="button" on:click={() => removeLevel(i)}>削除</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="text-xs opacity-70 mt-2">
          m = レベルごとピック曲数 / n = 全体ピック曲数。詳細はマウスホバーで表示。
        </p>
      </div>
    {/if}
  </div>
</div>
