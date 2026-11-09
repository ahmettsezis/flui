<script>
  import { api, bytes, rate, num, navigate, session } from '../lib/api.svelte.js';
  import Chart from '../lib/Chart.svelte';
  import NodeForm from './NodeForm.svelte';

  let { id } = $props();
  let node = $state(null), metrics = $state(null), range = $state('1h'), error = $state('');
  let editing = $state(false), showSnippet = $state(false), deleting = $state(false);
  const ranges = [['15m', '15m'], ['1h', '1h'], ['6h', '6h'], ['24h', '24h'], ['2d', '2d'], ['7d', '7d']];
  const palette = ['#2563eb', '#dc2626', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d'];

  async function load() {
    try {
      const [n, m] = await Promise.all([api(`/api/v1/nodes/${id}`), api(`/api/v1/nodes/${id}/metrics?range=${range}`)]);
      node = n; metrics = m; error = '';
    } catch (e) { if (e.status !== 401) error = e.message; }
  }

  $effect(() => {
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  });

  async function remove() {
    if (!deleting) { deleting = true; return; }
    await api(`/api/v1/nodes/${id}`, { method: 'DELETE' });
    navigate('/');
  }

  function ser(field, labelFn, offset = 0) {
    return (metrics?.plugins ?? [])
      .filter(p => p.ts.length)
      .map((p, i) => ({
        label: labelFn(p), data: p[field],
        stroke: palette[(i + offset) % palette.length], width: 1.5,
      }));
  }

  const workerPlugins = $derived((metrics?.plugins ?? []).filter(p => p.ts.length));
  const availSeries = $derived(ser('buffer_available_ratio', p => `w${p.worker + 1} ${p.plugin_id}`));
  const stateLabel = { up: 'Up', down: 'Down', stale: 'Stale', pending: 'Pending' };
  const lastError = $derived(node?.status?.last_error || '');
  const canEdit = $derived(session.user?.role !== 'viewer');
</script>

{#if editing && node}
  <div class="card">
    <h1>Edit node</h1>
    <NodeForm node={node} />
    <button class="link" onclick={() => editing = false}>Cancel</button>
  </div>
{:else if node}
  <div class="head">
    <div>
      <h1>{node.name} <span class="dot {node.status.state}"></span><small>{stateLabel[node.status.state]}</small></h1>
      <p class="muted mono">{node.monitor_url} · workers: {node.workers}</p>
      {#if lastError}<p class="error">Last error: {lastError}</p>{/if}
    </div>
    {#if canEdit}
      <div class="row">
        <button class="secondary" onclick={() => showSnippet = !showSnippet}>Fluentd config</button>
        <button class="secondary" onclick={() => editing = true}>Edit</button>
        <button class="danger" onclick={remove}>{deleting ? 'Confirm delete' : 'Delete'}</button>
      </div>
    {/if}
  </div>

  {#if showSnippet}
    <pre class="snippet">{node.snippet}</pre>
  {/if}

  <div class="cards">
    <div class="stat"><small>Buffered</small><strong>{bytes(node.status.summary?.buffer_bytes)}</strong></div>
    <div class="stat"><small>Queue (chunks)</small><strong>{num(node.status.summary?.queue_length)}</strong></div>
    <div class="stat"><small>Retries</small><strong class:warn={node.status.summary?.retry_count > 0}>{num(node.status.summary?.retry_count)}</strong></div>
    <div class="stat"><small>Free space</small><strong>{node.status.summary?.min_available_ratio == null ? '–' : node.status.summary.min_available_ratio.toFixed(1) + '%'}</strong></div>
    <div class="stat"><small>Throughput</small><strong>{rate(node.status.summary?.emit_records_rate, ' rec/s')}</strong></div>
    <div class="stat"><small>Writes</small><strong>{rate(node.status.summary?.write_rate, ' flushes/s')}</strong></div>
  </div>

  <div class="ranges">
    {#each ranges as [v, label] (v)}
      <button class:active={range === v} onclick={() => range = v}>{label}</button>
    {/each}
  </div>

  {#if workerPlugins.length}
    <div class="card">
      <Chart title="Free buffer space (%)" ts={workerPlugins[0].ts} series={availSeries} percent />
    </div>
    {#each workerPlugins as p (p.worker + '/' + p.plugin_id)}
      <div class="card plugin">
        <h2>worker {p.worker + 1} · <span class="mono">{p.plugin_id}</span> <span class="type">{p.type}</span></h2>
        <div class="grid">
          <Chart title="Buffered (bytes)" ts={p.ts} series={[{ label: 'bytes', data: p.buffer_total_queued_size, stroke: palette[0], width: 1.5 }]} />
          <Chart title="Chunks" ts={p.ts} series={[
            { label: 'queue', data: p.buffer_queue_length, stroke: palette[1], width: 1.5 },
            { label: 'stage', data: p.buffer_stage_length, stroke: palette[4], width: 1.5 },
          ]} />
          <Chart title="Throughput (records/s)" ts={p.ts} series={[{ label: 'emit', data: p.emit_records_rate, stroke: palette[2], width: 1.5 }]} />
          <Chart title="Retries (/s)" ts={p.ts} series={[{ label: 'retry', data: p.retry_rate, stroke: palette[3], width: 1.5 }]} />
        </div>
      </div>
    {/each}
  {:else}
    <div class="card empty">
      <p class="muted">No metric samples in this range yet.{node.status.state === 'up' ? ' Data appears after the first collections.' : ''}</p>
    </div>
  {/if}
{:else if error}
  <div class="card"><p class="error">{error}</p></div>
{:else}
  <p class="muted">Loading…</p>
{/if}

<style>
  .head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.5rem; }
  h1 { margin: 0; font-size: 1.3rem; }
  h1 small { font-size: 0.8rem; color: var(--muted); font-weight: 400; }
  h1 .dot { margin: 0 0.4rem; }
  .row { display: flex; gap: 0.5rem; }
  .muted { color: var(--muted); margin: 0.3rem 0 0; }
  .error { color: var(--bad); }
  .mono { font-family: ui-monospace, monospace; }
  .dot { display: inline-block; width: 0.7rem; height: 0.7rem; border-radius: 50%; background: var(--muted); }
  .dot.up { background: var(--ok); }
  .dot.down { background: var(--bad); }
  .dot.stale { background: var(--warn); }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 0.75rem; margin-bottom: 1rem; }
  .stat { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 0.8rem 1rem; }
  .stat small { display: block; color: var(--muted); margin-bottom: 0.3rem; }
  .stat strong { font-size: 1.15rem; font-variant-numeric: tabular-nums; }
  .warn { color: var(--bad); }
  .ranges { display: flex; gap: 0.4rem; margin-bottom: 1rem; }
  .ranges button { border: 1px solid var(--border); background: var(--card); color: var(--muted); border-radius: 4px; padding: 0.3rem 0.7rem; cursor: pointer; }
  .ranges button.active { color: var(--fg); border-color: var(--fg); }
  .card { margin-bottom: 1rem; }
  .plugin h2 { margin: 0 0 0.8rem; font-size: 0.95rem; }
  .type { color: var(--muted); font-weight: 400; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap: 1rem; }
  .snippet { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 1rem; overflow-x: auto; font-size: 0.8rem; margin-bottom: 1rem; }
  .empty { padding: 2rem; text-align: center; }
</style>
