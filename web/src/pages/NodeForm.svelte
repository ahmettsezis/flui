<script>
  import { untrack } from 'svelte';
  import { api, navigate } from '../lib/api.svelte.js';

  let { id = null, node = null } = $props();
  let name = $state(untrack(() => node?.name ?? ''));
  let monitor_url = $state(untrack(() => node?.monitor_url ?? 'http://'));
  let error = $state(''), fieldError = $state({}), testResult = $state(null), busy = $state(false);
  let added = $state(null); // created node with snippet (only on create)

  async function test() {
    busy = true; testResult = null; error = '';
    try {
      const r = await api('/api/v1/nodes/test', { method: 'POST', body: { monitor_url } });
      testResult = r;
    } catch (e) { applyErr(e); }
    finally { busy = false; }
  }

  async function submit(e) {
    e.preventDefault();
    busy = true; error = ''; fieldError = {};
    try {
      const r = node
        ? await api(`/api/v1/nodes/${node.id}`, { method: 'PUT', body: { name, monitor_url } })
        : await api('/api/v1/nodes', { method: 'POST', body: { name, monitor_url } });
      if (node) navigate(`/nodes/${node.id}`);
      else added = r;
    } catch (e2) { applyErr(e2); }
    finally { busy = false; }
  }

  function applyErr(e) {
    if (e.field) fieldError[e.field] = e.message;
    else error = e.message;
  }
</script>

{#if added}
  <div class="card">
    <h1>Node added</h1>
    <p>Copy this block into the Fluentd configuration on <strong>{added.name}</strong> and reload Fluentd.
       flui will start collecting within one interval.</p>
    <pre class="snippet">{added.snippet}</pre>
    <p class="muted">Allow TCP {added.monitor_url.match(/:(\d+)$/)?.[1] ?? '24220'} only from the flui server — monitor_agent has no authentication.</p>
    <div class="row">
      <a class="btn" href="#/nodes/{added.id}">Open node</a>
      <a class="link" href="#/">Back to nodes</a>
    </div>
  </div>
{:else}
  <div class="head">
    <h1>{node ? `Edit ${node.name}` : 'Add node'}</h1>
  </div>
  <form class="card" onsubmit={submit}>
    <p class="muted">flui pulls metrics from Fluentd's <code>monitor_agent</code>. Nothing is installed on the host.</p>
    <label>
      Name
      <input bind:value={name} required maxlength="64" placeholder="fluentd-web-01" />
      {#if fieldError.name}<span class="error">{fieldError.name}</span>{/if}
    </label>
    <label>
      Monitor agent URL
      <input bind:value={monitor_url} required placeholder="http://10.0.0.21:24220" class="mono" />
      <small class="muted">Host and port of Fluentd's <code>monitor_agent</code>.</small>
      {#if fieldError.monitor_url}<span class="error">{fieldError.monitor_url}</span>{/if}
    </label>
    {#if testResult}
      {#if testResult.ok}
        <p class="ok">Reachable — {testResult.output_plugins} output plugins found</p>
      {:else}
        <p class="error">Cannot reach {testResult.monitor_url}: {testResult.error}</p>
      {/if}
    {/if}
    {#if error}<p class="error">{error}</p>{/if}

    <div class="row">
      <button type="submit" disabled={busy}>{node ? 'Save' : 'Add node'}</button>
      <button type="button" class="secondary" onclick={test} disabled={busy}>Test connection</button>
      <a class="link" href="#/">Cancel</a>
    </div>
  </form>
{/if}

<style>
  .head { margin-bottom: 1rem; }
  h1 { margin: 0; font-size: 1.3rem; }
  form { display: flex; flex-direction: column; gap: 1rem; max-width: 34rem; }
  h1, .muted { margin-bottom: 0; }
  label { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.85rem; }
  .mono { font-family: ui-monospace, monospace; }
  .row { display: flex; gap: 1rem; align-items: center; }
  .ok { color: var(--ok); margin: 0; }
  .error { color: var(--bad); margin: 0; }
  .snippet { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 1rem; overflow-x: auto; font-size: 0.8rem; }
</style>
