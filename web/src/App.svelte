<script>
  let status = $state('checking');

  async function check() {
    try {
      const res = await fetch('/readyz', { cache: 'no-store' });
      status = res.ok ? 'ok' : 'unavailable';
    } catch {
      status = 'unavailable';
    }
  }

  $effect(() => {
    check();
    const id = setInterval(check, 10000);
    return () => clearInterval(id);
  });
</script>

<header>
  <strong>flui</strong>
  <span class="status {status}">server: {status}</span>
</header>

<main>
  <section>
    <h1>Fluentd monitoring</h1>
    <p>No nodes yet. Node management, dashboards and sign-in arrive in the next releases.</p>
  </section>
</main>

<style>
  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 1.5rem;
    border-bottom: 1px solid var(--border);
    background: var(--card);
  }
  main {
    max-width: 960px;
    margin: 2rem auto;
    padding: 0 1.5rem;
  }
  section {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 1.5rem;
  }
  h1 {
    margin-top: 0;
    font-size: 1.25rem;
  }
  p,
  .status {
    color: var(--muted);
  }
  .status.ok {
    color: var(--ok);
  }
  .status.unavailable {
    color: var(--bad);
  }
</style>
