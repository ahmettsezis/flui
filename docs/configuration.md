# Configuration

flui works without any configuration. Change settings only when you need to.

- **File:** `/etc/flui/flui.yaml` (change with `flui-server --config <path>`).
- **Environment:** every key can be overridden with `FLUI_` + the key in upper case, dots replaced by `_`.
  Example: `server.listen` → `FLUI_SERVER_LISTEN`.
- **Precedence:** environment > file > defaults.
- **Durations** use `s`, `m`, `h` or `d` (days), e.g. `30s`, `15m`, `12h`, `30d`.

## Example

```yaml
server:
  listen: ":8443"
  public_url: "https://flui.example.com:8443"
  tls:
    cert_file: /etc/flui/tls/flui.crt
    key_file: /etc/flui/tls/flui.key

retention: 30d
```

## Server

| Key | Default | Description |
|---|---|---|
| `server.listen` | `:8443` | Address and port to listen on |
| `server.public_url` | auto | URL users use to reach flui |
| `server.tls.cert_file` | – | PEM certificate (chain). Empty = self-signed, see [TLS](#tls) |
| `server.tls.key_file` | – | PEM private key |
| `server.trusted_proxies` | `[]` | CIDRs of reverse proxies allowed to set `X-Forwarded-For` |
| `data_dir` | `/var/lib/flui` (`/data` in Docker) | Database, TLS files, master key |
| `master_key_file` | `<data_dir>/master.key` | 32-byte key used to encrypt secrets at rest; created if missing |
| `log.level` | `info` | `debug`, `info`, `warn`, `error` |
| `log.format` | `json` | `json` or `text` |

## Authentication

| Key | Default | Description |
|---|---|---|
| `auth.session_idle_timeout` | `30m` | Sign out after inactivity |
| `auth.session_max_lifetime` | `12h` | Sign out after this time regardless of activity |
| `auth.password_min_length` | `12` | Minimum password length |
| `auth.lockout_threshold` | `5` | Failed sign-ins before the account is locked |
| `auth.lockout_duration` | `15m` | How long the account stays locked |

## Collection

| Key | Default | Description |
|---|---|---|
| `collector.interval` | `10s` | How often nodes are polled |
| `collector.timeout` | `5s` | Timeout per request to `monitor_agent` |
| `collector.stale_after` | `30s` | Mark a node **Stale** if no data arrived for this long |

## Retention

| Key | Default | Description |
|---|---|---|
| `retention` | `2d` | How long metric samples are kept; maximum `365d` |

## TLS

flui always serves HTTPS.

- **Own certificate (recommended):** set `server.tls.cert_file` and `server.tls.key_file`. flui reloads them
  when the files change, so certificate renewal (e.g. certbot) needs no restart.
- **Self-signed (default):** flui creates an internal CA and a server certificate in `<data_dir>/tls/`.
  Show the fingerprint with `flui-server tls fingerprint`.
