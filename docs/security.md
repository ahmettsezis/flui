# Security

flui is designed to be safe by default. This page describes what it protects, how, and what you are expected to do.
To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Overview

| Area | Default behaviour |
|---|---|
| Transport | HTTPS only, TLS 1.2+ (TLS 1.3 preferred), modern ciphers, HSTS |
| Passwords | Argon2id hashing, minimum length 12 |
| Two-factor | TOTP (RFC 6238), optional per user, can be required for admins |
| Brute force | Per-IP and per-account rate limiting, account lockout after 5 failures |
| Sessions | Server-side, random 256-bit IDs; cookies `HttpOnly; Secure; SameSite=Strict`; idle and absolute timeouts |
| CSRF | Token required on every state-changing request |
| Browser | Strict Content-Security-Policy (no inline scripts), `X-Content-Type-Options`, `frame-ancestors 'none'`, `Referrer-Policy` |
| Access control | Roles checked on every API call (see below) |
| Secrets at rest | Tokens stored as hashes; CA key and sensitive settings encrypted with AES-256-GCM |
| Agents | mTLS with short-lived certificates, revocable; CA pinned at enrollment |
| Ingest | Per-node tokens, size and rate limits, strict input validation |
| Audit | Sign-ins, failed sign-ins and every configuration change are logged with user, IP and time |
| Runtime | Non-root, distroless image, read-only filesystem, hardened systemd unit |

## Roles

| Role | Can |
|---|---|
| `viewer` | View dashboards, metrics and logs |
| `operator` | Viewer + add, edit and remove nodes and agents |
| `admin` | Operator + manage users, settings and retention; view the audit log |

## Trust boundaries

```
 Browser ──(1) HTTPS──▶ flui-server ◀──(3) mTLS── flui-agent
                          │    ▲
                (2) HTTP  │    │ (4) HTTPS + node token
                          ▼    │
                   Fluentd monitor_agent / out_http
```

1. **Users → flui:** authenticated sessions over TLS.
2. **flui → `monitor_agent` (agentless):** plain HTTP without authentication – a limitation of Fluentd.
   Restrict it to the flui server at network level ([how](agentless.md#network-and-firewall)) or use the agent.
3. **Agent → flui:** mutual TLS. The agent only makes outbound connections.
4. **Fluentd → flui (log push):** HTTPS with a per-node bearer token. Tokens can be rotated per node.

Log lines from Fluentd are treated as untrusted input: they are size-limited, stored as data and always
escaped when rendered.

## Keys and secrets

| Item | Location | Notes |
|---|---|---|
| Master key | `master_key_file` (`<data_dir>/master.key`) | Encrypts secrets in the database. Mode `0600`. Back it up separately from the database. |
| Internal CA | `<data_dir>/tls/` | Key encrypted with the master key. Signs agent and self-signed server certificates. |
| Initial admin password | `<data_dir>/initial-admin-password` | Deleted after the first password change. |

flui never logs passwords, tokens, session IDs or keys.

## Hardening checklist

- [ ] Use your own TLS certificate (or distribute flui's CA fingerprint to users).
- [ ] Put flui on an internal network; do not expose it to the internet.
- [ ] Restrict `monitor_agent` (TCP 24220) to the flui server, or use agent mode.
- [ ] Set `include_config false` in every `monitor_agent` block.
- [ ] Enable `auth.require_totp_for_admins`.
- [ ] Give users the lowest role they need.
- [ ] Back up the database and the master key, and store them separately.
- [ ] Verify release signatures ([SECURITY.md](../SECURITY.md#verifying-releases)).

## Supply chain

Every release is built in GitHub Actions and ships with checksums, an SBOM (SPDX) and cosign signatures.
CI runs `govulncheck`, `gosec`, `golangci-lint`, `npm audit` and a Trivy image scan; dependencies are updated
by Renovate.
