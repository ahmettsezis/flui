<script>
  import { api, bytes, rate, num } from '../lib/api.svelte.js';
  let nodes = $state(null), error = $state('');

  async function load() {
    try { nodes = await api('/api/v1/nodes'); error = ''; }
    catch (e) { if (e.status !== 401) error = e.message; }
  }

  $effect(() => {
    load();
    const id = setInterval(load, 10000);
    return () => clearInterval(id);
  });

  const stateLabel = { up: 'Up', down: 'Down', stale: 'Stale', pending: 'Pending' };
</script>

<div class="head">
  <h1>Nodes</h1>
  <a class="btn" href="#/nodes/new">+ Add node</a>
</div>

{#if error}<p class="error">{error}</p>{/if}
{#if nodes === null}
  <p class="muted">Loading…</p>
{:else if nodes.length === 0}
  <div class="card empty">
    <h2>No nodes yet</h2>
    <p>Add a Fluentd instance to start monitoring its buffers, retries and throughput.</p>
    <a class="btn" href="#/nodes/new">Add your first node</a>
  </div>
{:else}
  <div class="card tablewrap">
    <table>
      <thead>
        <tr>
          <th>Name</th><th>Status</th><th>Monitor URL</th><th>Workers</th><th>Plugins</th>
          <th class="num">Buffered</th><th class="num">Queue</th><th class="num">Retries</th><th class="num">Free space</th><th class="num">Records/s</th>
        </tr>
      </thead>
      <tbody>
        {#each nodes as n (n.id)}
          <tr onclick={() => location.hash = `#/nodes/${n.id}`}>
            <td class="name">{n.name}</td>
            <td><span class="dot {n.status.state}"></span>{stateLabel[n.status.state] ?? n.status.state}</td>
            <td class="mono">{n.monitor_url}</td>
            <td class="num">{n.workers}</td>
            <td class="num">{n.status.summary?.plugins ?? '–'}</td>
            <td class="num">{bytes(n.status.summary?.buffer_bytes)}</td>
            <td class="num">{num(n.status.summary?.queue_length)}</td>
            <td class="num" class:warn={n.status.summary?.retry_count > 0}>{num(n.status.summary?.retry_count)}</td>
            <td class="num">{n.status.summary?.min_available_ratio == null ? '–' : n.status.summary.min_available_ratio.toFixed(1) + '%'}</td>
            <td class="num">{rate(n.status.summary?.emit_records_rate)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<style>
  .head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
  h1 { margin: 0; font-size: 1.3rem; }
  .empty { text-align: center; padding: 2.5rem; }
  .empty h2 { margin-top: 0; font-size: 1.1rem; }
  .empty p { color: var(--muted); }
  .tablewrap { padding: 0; overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th { text-align: left; color: var(--muted); font-weight: 600; padding: 0.7rem 0.8rem; border-bottom: 1px solid var(--border); white-space: nowrap; }
  td { padding: 0.6rem 0.8rem; border-bottom: 1px solid var(--border); white-space: nowrap; }
  tbody tr { cursor: pointer; }
  tbody tr:hover { background: var(--bg); }
  tbody tr:last-child td { border-bottom: none; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .name { font-weight: 600; }
  .mono { font-family: ui-monospace, monospace; font-size: 0.8rem; color: var(--muted); }
  .warn { color: var(--bad); font-weight: 600; }
  .dot { display: inline-block; width: 0.6rem; height: 0.6rem; border-radius: 50%; margin-right: 0.4rem; background: var(--muted); }
  .dot.up { background: var(--ok); }
  .dot.down { background: var(--bad); }
  .dot.stale { background: var(--warn); }
</style>
