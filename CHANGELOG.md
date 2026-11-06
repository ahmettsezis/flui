# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Project documentation and repository layout.
- `flui-server` skeleton: configuration (file + `FLUI_*` environment), SQLite storage with migrations,
  `backup` command, master key encryption, internal CA with automatic self-signed server certificate,
  HTTPS (TLS 1.2+) with security headers, `/healthz` and `/readyz`, embedded web UI.
- Container image (distroless, non-root) and Docker Compose example.
