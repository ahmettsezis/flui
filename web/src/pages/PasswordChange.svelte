<script>
  import { api, session, logout } from '../lib/api.svelte.js';
  let current = $state(''), next = $state(''), again = $state(''), error = $state(''), busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    if (next !== again) { error = 'Passwords do not match'; return; }
    busy = true; error = '';
    try {
      await api('/api/v1/auth/password', { method: 'POST', body: { current_password: current, new_password: next } });
      session.user = { ...session.user, must_change_password: false };
    } catch (err) { error = err.message; }
    finally { busy = false; }
  }
</script>

<div class="center">
  <form class="card" onsubmit={submit}>
    <h1>Set a new password</h1>
    <p class="muted">For security, you must change the initial password before continuing.</p>
    <label>Current password<input type="password" bind:value={current} autocomplete="current-password" required /></label>
    <label>New password<input type="password" bind:value={next} autocomplete="new-password" required minlength="8" /></label>
    <label>Repeat<input type="password" bind:value={again} autocomplete="new-password" required /></label>
    {#if error}<p class="error">{error}</p>{/if}
    <button type="submit" disabled={busy}>Change password</button>
    <button type="button" class="link" onclick={logout}>Sign out</button>
  </form>
</div>

<style>
  form { width: 20rem; display: flex; flex-direction: column; gap: 0.8rem; }
  h1 { margin: 0; font-size: 1.15rem; }
  .muted { font-size: 0.85rem; margin: 0; }
  label { color: var(--muted); }
  .error { margin: 0; font-size: 0.85rem; }
</style>
