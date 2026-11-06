# Architecture

How flui works internally. You do not need this page to run flui.

## Components

```
┌──────────────────────────── flui-server ────────────────────────────┐
│  web UI (Svelte, embedded)                                          │
│  api       REST + Server-Sent Events, auth, RBAC                    │
│  collector polls monitor_agent of agentless nodes                   │
│  ingest    receives Fluentd logs (token) and agent data (mTLS)      │
│  pki       internal CA, agent enrollment, certificate renewal       │
│  store     SQLite (WAL): metrics, logs (FTS5), users, audit         │
│  jobs      rollups and retention cleanup                            │
└─────────────────────────────────────────────────────────────────────┘

┌──────────── flui-agent ────────────┐
│  monitor_agent client (localhost)  │
│  sysmetrics  /proc, statfs         │──── mTLS push ───▶ flui-server
│  logtail     rotation-aware tail   │
│  queue       on-disk, size-limited │
└────────────────────────────────────┘
```

Both binaries are written in Go, built without CGO, and have no runtime dependencies.

## Data flow

1. **Agentless metrics:** `collector` requests `/api/plugins.json` from each node (and each worker port) every
   `collector.interval`, converts counters to rates and writes a sample to `metrics_raw`.
2. **Agent metrics:** the agent collects the same data plus system metrics and sends batches to
   `POST /api/v1/agent/push`. `ingest` validates and writes them to `metrics_raw`.
3. **Logs:** Fluentd (`out_http`) or the agent posts log lines to the ingest endpoints; they are stored in `logs`
   and indexed in `logs_fts`.
4. **Live view:** new samples and log lines are published on an in-process bus and streamed to browsers via SSE.
5. **Rollups:** `jobs` aggregates `metrics_raw` → `metrics_1m` → `metrics_1h` (min/max/avg/last) and deletes
   data older than the configured [retention](configuration.md#retention).

## Storage

| Table | Content |
|---|---|
| `nodes`, `agents` | Monitored Fluentd instances, agent certificates |
| `metrics_raw`, `metrics_1m`, `metrics_1h` | Time series, one row per node / plugin / metric / timestamp |
| `logs`, `logs_fts` | Fluentd log lines and full-text index |
| `users`, `sessions`, `api_tokens` | Authentication |
| `audit_log` | Security-relevant events |
| `settings` | Runtime settings (sensitive values encrypted) |

Schema changes are versioned migrations in `internal/store/migrations/`, applied on start.

SQLite allows one writer at a time; writes are batched. A single server comfortably handles a few hundred
nodes. High availability is out of scope for now.

## Repository layout

```
cmd/flui-server/      server entry point
cmd/flui-agent/       agent entry point
internal/
  api/                HTTP handlers, middleware, SSE
  audit/              audit log
  auth/               passwords, sessions, TOTP, RBAC
  collector/          monitor_agent client and scheduler
  config/             configuration loading
  ingest/             log and agent ingestion
  logtail/            log file tailing (agent)
  pki/                internal CA and enrollment
  store/              SQLite access, migrations, rollups
  sysmetrics/         /proc readers (agent)
web/                  UI source (built into internal/webui)
api/openapi.yaml      HTTP API specification
deploy/               systemd, docker, compose, helm, Fluentd snippets
docs/                 documentation
```

## Design principles

- **Small:** a feature must be useful to most Fluentd users to be included.
- **Secure by default:** no option should make flui insecure silently.
- **Zero dependencies at runtime:** one binary, one data directory.
