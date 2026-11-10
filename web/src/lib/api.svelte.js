let csrfToken = null;
export const session = $state({ user: null, ready: false });

function setSession(user, csrf) {
  session.user = user;
  session.ready = true;
  csrfToken = csrf;
}

export class ApiError extends Error {
  constructor(status, body) {
    super(body?.error || `HTTP ${status}`);
    this.status = status;
    this.field = body?.field;
  }
}

export async function api(path, { method = 'GET', body } = {}) {
  const headers = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken;
  const res = await fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
  if (res.status === 401 && !path.startsWith('/api/v1/auth/login')) {
    session.user = null;
    csrfToken = null;
    location.hash = '#/login';
    throw new ApiError(401, null);
  }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, data);
  return data;
}

export async function refreshSession() {
  try {
    const r = await fetch('/api/v1/auth/me', { headers: { Accept: 'application/json' } });
    if (r.ok) {
      const data = await r.json();
      setSession(data.user, data.csrf_token);
      return;
    }
  } catch { /* offline or no session */ }
  setSession(null, null);
}

export async function login(username, password) {
  const res = await api('/api/v1/auth/login', { method: 'POST', body: { username, password } });
  setSession(res.user, res.csrf_token);
}

export async function logout() {
  await api('/api/v1/auth/logout', { method: 'POST' });
  session.user = null;
  csrfToken = null;
}

export function navigate(path) { location.hash = '#' + path; }

// fmt helpers
export function bytes(v) {
  if (v == null) return '–';
  const u = ['B', 'KiB', 'MiB', 'GiB'];
  let i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${u[i]}`;
}
export function rate(v, unit = '/s') { return v == null ? '–' : `${v >= 100 ? Math.round(v) : v.toFixed(1)}${unit}`; }
export function num(v) { return v == null ? '–' : Math.round(v).toLocaleString('en'); }
export function pct(v) { return v == null ? '–' : v.toFixed(1) + '%'; }
export const stateLabel = { up: 'Up', down: 'Down', stale: 'Stale', pending: 'Pending' };
