<script>
  import { session, refreshSession, logout, navigate } from './lib/api.svelte.js';
  import Login from './pages/Login.svelte';
  import PasswordChange from './pages/PasswordChange.svelte';
  import NodeList from './pages/NodeList.svelte';
  import NodeForm from './pages/NodeForm.svelte';
  import NodeDetail from './pages/NodeDetail.svelte';

  let route = $state(location.hash.slice(1) || '/');

  function onHash() { route = location.hash.slice(1) || '/'; }

  $effect(() => { refreshSession(); });

  $effect(() => {
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  });

  $effect(() => {
    if (!session.ready) return;
    if (!session.user && route !== '/login') navigate('/login');
    else if (session.user?.must_change_password && route !== '/password') navigate('/password');
    else if (session.user && !session.user.must_change_password && (route === '/login' || route === '/password')) navigate('/');
  });

  const nodeMatch = $derived(route.match(/^\/nodes\/(\d+)$/));
</script>

{#if !session.ready}
  <div class="center">…</div>
{:else if !session.user}
  <Login />
{:else if session.user.must_change_password}
  <PasswordChange />
{:else}
  <header class="topbar">
    <a class="brand" href="#/">flui</a>
    <nav>
      <a href="#/" class:active={route === '/' || route.startsWith('/nodes')}>Nodes</a>
    </nav>
    <div class="right">
      <span class="user">{session.user.username} <em>{session.user.role}</em></span>
      <button class="link" onclick={logout}>Sign out</button>
    </div>
  </header>
  <main>
    {#if route === '/'}
      <NodeList />
    {:else if route === '/nodes/new'}
      <NodeForm />
    {:else if nodeMatch}
      <NodeDetail id={Number(nodeMatch[1])} />
    {:else}
      <div class="card"><h2>Not found</h2><p><a href="#/">Back to nodes</a></p></div>
    {/if}
  </main>
{/if}

<style>
  .topbar {
    display: flex; align-items: center; gap: 1.5rem;
    padding: 0.6rem 1.5rem; background: var(--card);
    border-bottom: 1px solid var(--border);
  }
  .brand { font-weight: 700; font-size: 1.1rem; color: var(--fg); text-decoration: none; }
  nav a { color: var(--muted); text-decoration: none; padding: 0.25rem 0.5rem; border-radius: 4px; }
  nav a.active { color: var(--fg); background: var(--bg); }
  .right { margin-left: auto; display: flex; align-items: center; gap: 1rem; }
  .user { color: var(--muted); font-size: 0.85rem; }
  .user em { font-style: normal; background: var(--bg); border: 1px solid var(--border); border-radius: 4px; padding: 0.1rem 0.4rem; }
  main { max-width: 1080px; margin: 1.5rem auto; padding: 0 1.5rem; }
</style>
