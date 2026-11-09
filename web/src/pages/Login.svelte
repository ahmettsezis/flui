<script>
  import { login } from '../lib/api.svelte.js';
  let username = $state(''), password = $state(''), error = $state(''), busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = '';
    try { await login(username.trim(), password); }
    catch (err) { error = err.message; }
    finally { busy = false; password = ''; }
  }
</script>

<div class="center">
  <form class="card login" onsubmit={submit}>
    <h1>flui</h1>
    <label>Username<input bind:value={username} autocomplete="username" required maxlength="64" /></label>
    <label>Password<input type="password" bind:value={password} autocomplete="current-password" required /></label>
    {#if error}<p class="error">{error}</p>{/if}
    <button type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
</div>

<style>
  .center { display: grid; place-items: center; min-height: 100vh; }
  .login { width: 20rem; display: flex; flex-direction: column; gap: 0.8rem; }
  h1 { margin: 0 0 0.5rem; text-align: center; }
  label { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.85rem; color: var(--muted); }
  .error { color: var(--bad); margin: 0; font-size: 0.85rem; }
</style>
