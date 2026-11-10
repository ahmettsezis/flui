# Architecture

How flui works internally. You do not need this page to run flui.

## Components

```
┌──────────────────────────── flui-server ────────────────────────────┐
│  web UI (Svelte, embedded)                                          │
│  api        REST, auth, RBAC                                        │
│  collector  polls monitor_agent of every node                       │
│  pki        internal CA, self-signed server certificate renewal     │
│  store      SQLite (WAL): metrics, users, sessions, audit           │
│  jobs       retention and session cleanup                           │
└─────────────────────────────────────────────────────────────────────┘
```

flui is a single Go binary, built without CGO, with no runtime dependencies.

## Data flow

1. **Metrics:** `collector` requests `/api/plugins.json` from each node every
   `collector.interval`, converts counters to rates and writes a sample to `plugin_samples`.
2. **Cleanup:** `jobs` deletes samples older than the configured [retention](configuration.md#retention)
   and prunes expired sessions once an hour.

## Storage

| Table | Content |
|---|---|
| `nodes` | Monitored Fluentd instances |
| `plugin_samples` | Time series, one row per node / plugin / timestamp |
| `users`, `sessions` | Authentication |
| `audit_log` | Security-relevant events |
| `settings` | Runtime settings |

Schema changes are versioned migrations in `internal/store/migrations/`, applied on start.

SQLite allows one writer at a time; writes are batched. A single server comfortably handles a few hundred
nodes. High availability is out of scope for now.

## Repository layout

```
cmd/flui-server/      server entry point
internal/
  api/                HTTP handlers, middleware
  auth/               passwords, sessions, RBAC
  collector/          monitor_agent client and scheduler
  config/             configuration loading
  nodes/              node registry and monitor_agent snippet
  pki/                internal CA and server certificate
  secret/             master key loading
  server/             wiring, TLS, housekeeping jobs
  store/              SQLite access, migrations
  webui/              embedded built UI
web/                  UI source (built into internal/webui)
deploy/               systemd, docker, compose, Fluentd snippets
docs/                 documentation
```

## Design principles

- **Small:** a feature must be useful to most Fluentd users to be included.
- **Secure by default:** no option should make flui insecure silently.
- **Zero dependencies at runtime:** one binary, one data directory.
