<script>
  import { api, bytes, rate, num, navigate, session } from '../lib/api.svelte.js';
  import Chart from '../lib/Chart.svelte';
  import NodeForm from './NodeForm.svelte';
  import { aggregatePlugins, pluginRows, filterAndSortPlugins, pluginKey } from '../lib/metrics.js';

  let { id } = $props();
  let node = $state(null), metrics = $state(null), range = $state('1h'), error = $state('');
  let editing = $state(false), showSnippet = $state(false), deleting = $state(false);
  let query = $state(''), filter = $state('all'), sort = $state('attention'), descending = $state(false);
  let selectedKey = $state(null);
  const ranges = [['15m', '15m'], ['1h', '1h'], ['6h', '6h'], ['24h', '24h'], ['2d', '2d'], ['7d', '7d']];
  const palette = ['#2563eb', '#dc2626', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d'];

  async function load(nodeID, selectedRange, signal) {
    try {
      const [n, m] = await Promise.all([api(`/api/v1/nodes/${nodeID}`), api(`/api/v1/nodes/${nodeID}/metrics?range=${selectedRange}`)]);
      if (signal.aborted) return;
      node = n; metrics = m; error = '';
    } catch (e) { if (!signal.aborted && e.status !== 401) error = e.message; }
  }

  $effect(() => {
    const nodeID = id, selectedRange = range, controller = new AbortController();
    load(nodeID, selectedRange, controller.signal);
    const t = setInterval(() => load(nodeID, selectedRange, controller.signal), 10000);
    return () => { controller.abort(); clearInterval(t); };
  });

  async function remove() {
    if (!deleting) { deleting = true; return; }
    await api(`/api/v1/nodes/${id}`, { method: 'DELETE' });
    navigate('/');
  }

  function setSort(field) {
    descending = sort === field ? !descending : !['plugin_id', 'type', 'buffer_available_ratio'].includes(field);
    sort = field;
  }

  const allPlugins = $derived((metrics?.plugins ?? []).filter(p => p.ts.length));
  const totals = $derived(aggregatePlugins(allPlugins));
  const rows = $derived(pluginRows(allPlugins));
  const visibleRows = $derived(filterAndSortPlugins(rows, query, filter, sort, descending));
  const selected = $derived(allPlugins.find(p => pluginKey(p) === selectedKey));
  const hasAnonymous = $derived(allPlugins.some(p => p.plugin_id.startsWith('object:')));
  const columns = [
    ['plugin_id', 'Plugin'], ['type', 'Type'],
    ['buffer_total_queued_size', 'Buffered'], ['buffer_queue_length', 'Queue'],
    ['retry_count', 'Retry counter'], ['emit_records_rate', 'Records/s'], ['buffer_available_ratio', 'Free buffer'],
  ];
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
      <p class="muted mono">{node.monitor_url}</p>
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
    <div class="stat"><small>Retry counter</small><strong>{num(node.status.summary?.retry_count)}</strong></div>
    <div class="stat"><small>Lowest free buffer</small><strong>{node.status.summary?.min_available_ratio == null ? '–' : node.status.summary.min_available_ratio.toFixed(1) + '%'}</strong></div>
    <div class="stat"><small>Throughput</small><strong>{rate(node.status.summary?.emit_records_rate, ' rec/s')}</strong></div>
    <div class="stat"><small>Writes</small><strong>{rate(node.status.summary?.write_rate, ' flushes/s')}</strong></div>
  </div>
  {#if error}<p class="error" role="alert">Could not refresh: {error}. Showing the last loaded data.</p>{/if}
  <p class="muted freshness">Last collection: {node.status.last_seen ? new Date(node.status.last_seen).toLocaleString() : 'Waiting for data'}.
    Up means monitor_agent is reachable; it does not confirm delivery to OpenSearch.</p>

  <div class="ranges">
    {#each ranges as [v, label] (v)}
      <button class:active={range === v} onclick={() => range = v}>{label}</button>
    {/each}
  </div>

  {#if allPlugins.length}
    <div class="card">
      <h2>All outputs</h2>
      <p class="muted help">Totals across reported output plugins. Free buffer is the lowest reported percentage, not disk space.
        Missing values are excluded; gaps are not zero. Buffer values use each time bucket's peak.</p>
      <div class="grid">
        <Chart title="Total buffered (bytes)" ts={totals.ts} series={[{ label: 'bytes', data: totals.buffer_total_queued_size, stroke: palette[0] }]} />
        <Chart title="Total throughput (records/s)" ts={totals.ts} series={[{ label: 'records/s', data: totals.emit_records_rate, stroke: palette[2] }]} />
        <Chart title="Total queued chunks" ts={totals.ts} series={[{ label: 'queue', data: totals.buffer_queue_length, stroke: palette[4] }]} />
        <Chart title="Lowest free buffer (%)" ts={totals.ts} series={[{ label: 'free buffer', data: totals.buffer_available_ratio, stroke: palette[5] }]} percent />
      </div>
    </div>
    <div class="card">
      <h2>Output plugins</h2>
      {#if hasAnonymous}
        <p class="muted help"><code>object:…</code> is a Fluentd-generated plugin ID. Metrics work without an explicit ID, but the log route
          cannot be inferred from it. A unique <code>@id</code> in each output gives it a readable, stable name. Keep <code>include_config false</code>.</p>
      {/if}
      <div class="filters">
        <label>Search plugins<input type="search" placeholder="Plugin ID or type" bind:value={query} /></label>
        <label>Show<select bind:value={filter}>
          <option value="all">All plugins</option>
          <option value="active">Active or needs attention</option>
          <option value="attention">Needs attention</option>
        </select></label>
        <button class="secondary" onclick={() => { sort = 'attention'; descending = false; }}>Attention first</button>
      </div>
      <p class="muted help">{visibleRows.length} / {rows.length} plugins · Latest bucket per plugin in the selected range.
        Attention: retries increasing or free buffer below 10%. Active: buffered data or output activity.
        Retry counter is cumulative; “–” means unavailable. Earlier samples may belong to removed plugins.</p>
      <div class="tablewrap">
        <table>
          <thead><tr>
            <th scope="col">Activity</th>
            {#each columns as [field, label]}
              <th scope="col" aria-sort={sort === field ? (descending ? 'descending' : 'ascending') : 'none'}>
                <button class="sort" onclick={() => setSort(field)}>{label}{sort === field ? (descending ? ' ↓' : ' ↑') : ''}</button>
              </th>
            {/each}
          </tr></thead>
          <tbody>
            {#each visibleRows as p (p.key)}
              <tr class:selected={selectedKey === p.key}>
                <td class:warn={p.attention && !p.historical}>{p.state}</td>
                <td class="plugin-name"><button class="link" aria-expanded={selectedKey === p.key}
                  onclick={() => selectedKey = selectedKey === p.key ? null : p.key}>{p.plugin_id}</button></td>
                <td>{p.type}</td>
                <td class="numeric">{bytes(p.buffer_total_queued_size)}</td>
                <td class="numeric">{num(p.buffer_queue_length)}</td>
                <td class="numeric">{num(p.retry_count)}</td>
                <td class="numeric">{rate(p.emit_records_rate)}</td>
                <td class="numeric">{p.buffer_available_ratio == null ? '–' : p.buffer_available_ratio.toFixed(1) + '%'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if !visibleRows.length}<p class="empty muted">No plugins match these filters.</p>{/if}
      <p class="muted help">Select a plugin name to open its charts below. Node totals always include all reported outputs.</p>
    </div>
    {#if selected}
      {@const p = selected}
      <div class="card plugin">
        <div class="head">
          <h2><span class="mono">{p.plugin_id}</span> <span class="type">{p.type}</span></h2>
          <button class="secondary" onclick={() => selectedKey = null}>Close detail</button>
        </div>
        <p class="muted help">Last bucket: {new Date(p.ts.at(-1) * 1000).toLocaleString()}</p>
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
    {/if}
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
  h2 { margin: 0 0 0.8rem; font-size: 1rem; }
  .freshness { margin-bottom: 1rem; font-size: 0.85rem; }
  .help { font-size: 0.85rem; line-height: 1.5; margin-bottom: 1rem; }
  .filters { display: flex; flex-wrap: wrap; gap: 0.75rem; align-items: end; margin-bottom: 0.75rem; }
  .filters label { display: grid; gap: 0.3rem; font-size: 0.85rem; }
  .tablewrap { max-height: 440px; overflow: auto; }
  table { width: 100%; border-collapse: separate; border-spacing: 0; font-size: 0.85rem; }
  th, td { padding: 0.65rem; border-bottom: 1px solid var(--border); text-align: left; white-space: nowrap; }
  th { position: sticky; top: 0; background: var(--card); z-index: 1; }
  .sort { background: none; color: var(--fg); padding: 0; font-weight: 600; white-space: nowrap; }
  .numeric { text-align: right; font-variant-numeric: tabular-nums; }
  .plugin-name { min-width: 180px; max-width: 320px; white-space: normal; overflow-wrap: anywhere; }
  .plugin-name button { text-align: left; }
  tr.selected, tbody tr:hover { background: var(--bg); }
  .plugin h2 { margin: 0 0 0.8rem; font-size: 0.95rem; }
  .type { color: var(--muted); font-weight: 400; }
  .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; }
  @media (max-width: 700px) { .grid { grid-template-columns: minmax(0, 1fr); } }
  .snippet { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 1rem; overflow-x: auto; font-size: 0.8rem; margin-bottom: 1rem; }
  .empty { padding: 2rem; text-align: center; }
</style>
