<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type DashboardSnapshotDTO, type RequestLogDTO, type FetchLogDTO, type PickSnapshotDTO } from '../api';

  let requests: RequestLogDTO[] = [];
  let fetches: FetchLogDTO[] = [];
  let picks: PickSnapshotDTO[] = [];
  let loading = true;
  let error = '';

  let unsubReq: (() => void) | null = null;
  let unsubFetch: (() => void) | null = null;
  let unsubPick: (() => void) | null = null;

  const REQ_CAP = 100;
  const FETCH_CAP = 100;

  onMount(async () => {
    try {
      const snap: DashboardSnapshotDTO = await api.getDashboardSnapshot();
      requests = snap.requests;
      fetches = snap.fetches;
      picks = snap.picks;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
    unsubReq = api.onDashboardRequestLogged((e) => {
      requests = [e, ...requests].slice(0, REQ_CAP);
    });
    unsubFetch = api.onDashboardFetchLogged((e) => {
      fetches = [e, ...fetches].slice(0, FETCH_CAP);
    });
    unsubPick = api.onDashboardPickChanged(async (_publishedId) => {
      // ピック変化はリストではなく集約なので毎回 Snapshot を取り直す
      try {
        const snap = await api.getDashboardSnapshot();
        picks = snap.picks;
      } catch (e) {
        console.warn('refresh picks failed', e);
      }
    });
  });

  onDestroy(() => {
    if (unsubReq) unsubReq();
    if (unsubFetch) unsubFetch();
    if (unsubPick) unsubPick();
  });

  function formatJST(iso: string): string {
    if (!iso) return '-';
    try {
      return new Date(iso).toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo', hour12: false });
    } catch {
      return iso;
    }
  }

  function statusBadge(code: number): string {
    if (code >= 500) return 'badge-error';
    if (code >= 400) return 'badge-warning';
    if (code >= 300) return 'badge-info';
    if (code >= 200) return 'badge-success';
    return 'badge-ghost';
  }

  function fetchStatusBadge(s: string): string {
    if (s === 'ok') return 'badge-success';
    if (s === 'error') return 'badge-error';
    return 'badge-ghost';
  }

  function levelCountsLine(counts: Record<string, number>, order: string[]): string {
    const keys = order.length ? order : Object.keys(counts);
    return keys.map((k) => `${k}:${counts[k] ?? 0}`).join(' / ');
  }
</script>

<section class="p-4 space-y-4">
  {#if loading}
    <div class="flex items-center gap-2 text-sm py-4">
      <span class="loading loading-spinner loading-sm"></span>
      <span>読み込み中…</span>
    </div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else}
    <!-- 最近のリクエスト -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">最近のリクエスト ({requests.length})</h2>
        {#if requests.length === 0}
          <div class="text-sm opacity-70 py-2">まだリクエストはありません。</div>
        {:else}
          <div class="overflow-x-auto max-h-96">
            <table class="table table-sm table-pin-rows">
              <thead>
                <tr>
                  <th>時刻 (JST)</th>
                  <th>メソッド</th>
                  <th>パス</th>
                  <th>ステータス</th>
                  <th>経過 ms</th>
                </tr>
              </thead>
              <tbody>
                {#each requests as r, i (`${r.at}-${i}`)}
                  <tr>
                    <td class="text-xs">{formatJST(r.at)}</td>
                    <td>{r.method}</td>
                    <td class="text-xs break-all">{r.path}</td>
                    <td><span class="badge {statusBadge(r.statusCode)}">{r.statusCode}</span></td>
                    <td>{r.durationMs}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>

    <!-- ソース表更新履歴 -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">ソース表更新履歴 ({fetches.length})</h2>
        {#if fetches.length === 0}
          <div class="text-sm opacity-70 py-2">起動以降の更新履歴はありません。</div>
        {:else}
          <div class="overflow-x-auto max-h-72">
            <table class="table table-sm table-pin-rows">
              <thead>
                <tr>
                  <th>時刻 (JST)</th>
                  <th>表示名</th>
                  <th>結果</th>
                  <th>エラー</th>
                </tr>
              </thead>
              <tbody>
                {#each fetches as f, i (`${f.at}-${i}`)}
                  <tr>
                    <td class="text-xs">{formatJST(f.at)}</td>
                    <td>{f.displayName || f.sourceId}</td>
                    <td><span class="badge {fetchStatusBadge(f.status)}">{f.status}</span></td>
                    <td class="text-xs whitespace-pre-line">{f.error}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>

    <!-- 現在のピック結果サマリ -->
    <div class="card bg-base-100 shadow-sm border border-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">現在のピック結果 ({picks.length})</h2>
        {#if picks.length === 0}
          <div class="text-sm opacity-70 py-2">まだピック結果はありません。公開表へのリクエストか「再ピック」で生成されます。</div>
        {:else}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
            {#each picks as p (p.publishedId)}
              <div class="rounded-box border border-base-300 p-3 text-sm">
                <div class="font-mono text-xs opacity-70">{p.publishedId}</div>
                <div>計 {p.totalCount} 曲</div>
                <div class="text-xs opacity-70 mt-1">{levelCountsLine(p.levelCounts, p.levelOrder)}</div>
                <div class="text-xs opacity-50 mt-1">生成: {formatJST(p.generatedAt)}</div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</section>
