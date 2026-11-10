# Security

flui is designed to be safe by default. This page describes what it protects, how, and what you are expected to do.
To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Overview

| Area | Default behaviour |
|---|---|
| Transport | HTTPS only, TLS 1.2+ (TLS 1.3 preferred), modern ciphers, HSTS |
| Passwords | Argon2id hashing, minimum length 12 |
| Brute force | Sign-in limited to 10 attempts per minute per IP; account locked for 15 minutes after 5 failures |
| Sessions | Server-side, random 256-bit IDs stored hashed; cookies `HttpOnly; Secure; SameSite=Strict`; idle and absolute timeouts |
| CSRF | Token and same-origin check required on every state-changing request |
| Browser | Strict Content-Security-Policy (no inline scripts), `X-Content-Type-Options`, `frame-ancestors 'none'`, `Referrer-Policy` |
| Access control | Roles checked on every API call (see below) |
| Secrets at rest | Internal CA key encrypted with AES-256-GCM |
| Audit | Sign-ins, failed sign-ins, sign-outs, password changes and node changes are recorded in the database with user, IP and time, and kept for one year |
| Runtime | Non-root, distroless image, read-only filesystem, hardened systemd unit |

## Users and roles

flui creates one user, `admin`, on first start. Its random password is written to
`<data_dir>/initial-admin-password` and must be changed at the first sign-in. If it is lost, run
`flui-server user reset-password admin` on the server.

There is no user management yet: the UI and API cannot create further users. The API already enforces
these roles:

| Role | Can |
|---|---|
| `viewer` | View dashboards and metrics |
| `operator` | Viewer + add, edit, remove and test nodes |
| `admin` | Same as operator for now |

## Trust boundaries

```
 Browser ──(1) HTTPS──▶ flui-server
                            │
                     (2) HTTP│
                            ▼
                  Fluentd monitor_agent
```

1. **Users → flui:** authenticated sessions over TLS.
2. **flui → `monitor_agent`:** plain HTTP without authentication – a limitation of Fluentd.
   Restrict it to the flui server at network level ([how](setup.md#network-and-firewall)) and always set
   `include_config false`, otherwise the endpoint exposes your full Fluentd config including credentials.

## Keys and secrets

| Item | Location | Notes |
|---|---|---|
| Master key | `master_key_file` (`<data_dir>/master.key`) | Encrypts secrets at rest. Mode `0600`. Back it up separately from the database. |
| Internal CA | `<data_dir>/tls/` | Key encrypted with the master key. Signs the self-signed server certificate. |
| Initial admin password | `<data_dir>/initial-admin-password` | Mode `0600`. Deleted after the first password change. |

flui does not log passwords, tokens, session IDs or keys, with one exception: the initial admin password
is written to the log once, when it is created on first start. Change it right away.

## Hardening checklist

- [ ] Use your own TLS certificate (or distribute flui's CA fingerprint to users).
- [ ] Put flui on an internal network; do not expose it to the internet.
- [ ] Restrict `monitor_agent` (TCP 24220) to the flui server's IP.
- [ ] Set `include_config false` in every `monitor_agent` block.
- [ ] Change the initial admin password at the first sign-in.
- [ ] Back up the database and the master key, and store them separately.

## Supply chain

CI runs `govulncheck`, `golangci-lint` (including `gosec`) and `npm audit` on every change, and builds and
smoke-tests the container image. Dependencies are pinned to exact versions; GitHub Actions and base images
are pinned by digest. There are no signed releases yet: build the binary or image from source.
