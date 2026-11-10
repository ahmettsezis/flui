# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- **Breaking:** flui is now agentless-only. The `nodes.mode` column is dropped, the unused `ingest.*`
  settings and `auth.require_totp_for_admins` are removed, and `retention` is now a single duration
  (`retention: 30d`) instead of a `raw`/`minute`/`hour`/`logs` map.
- **Breaking:** multi-worker support is removed. Each node polls a single `monitor_agent` URL; the
  `workers` fields and the `worker` column are dropped from the API and database.
- Metric columns that were collected but never displayed (`buffer_*_byte_size`, `emit_count`,
  `rollback_count`, `slow_flush_count`, `flush_time_count`) are no longer stored.
- **Removed:** the `flui-server backup` command. `flui.db` holds only metric samples and can be
  replaced by a fresh database at any time.
- Node details now show aggregate charts and a searchable, sortable output table with activity filters.
  Plugin charts open on selection instead of rendering every plugin at once.
- Explain generated Fluentd IDs, missing metrics, buffer percentages and retry counters in the dashboard.

### Added

- Project documentation and repository layout.
- `flui-server` skeleton: configuration (file + `FLUI_*` environment), SQLite storage with migrations,
  master key encryption, internal CA with automatic self-signed server certificate,
  HTTPS (TLS 1.2+) with security headers, `/healthz` and `/readyz`, embedded web UI.
- Container image (distroless, non-root) and Docker Compose example.
- Local users and sign-in: Argon2id hashing, server-side sessions, CSRF and origin checks, account lockout,
  login rate limiting, roles (admin / operator / viewer), bootstrap admin with forced password change,
  `flui-server user reset-password`, audit log.
- Agentless node management: add, edit, delete and test Fluentd nodes from the UI; the collector polls
  `monitor_agent` and stores buffer, retry and throughput metrics with retention.
- UI: sign-in, node overview, node detail with per-plugin charts (uPlot), generated `monitor_agent` snippet,
  light and dark theme.
