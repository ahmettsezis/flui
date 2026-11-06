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

retention:
  raw: 2d
  minute: 30d
  hour: 365d
  logs: 14d
```

## Server

| Key | Default | Description |
|---|---|---|
| `server.listen` | `:8443` | Address and port to listen on |
| `server.public_url` | auto | URL users and Fluentd hosts use to reach flui; used in generated snippets |
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
| `auth.require_totp_for_admins` | `false` | Force admins to enable two-factor authentication |

## Collection

| Key | Default | Description |
|---|---|---|
| `collector.interval` | `10s` | How often agentless nodes are polled |
| `collector.timeout` | `5s` | Timeout per request to `monitor_agent` |
| `collector.stale_after` | `30s` | Mark a node **Stale** if no data arrived for this long |
| `ingest.max_body_bytes` | `1048576` | Maximum size of a single log/agent upload |
| `ingest.rate_limit` | `20` | Uploads per second allowed per node |

## Retention

Metrics are stored at three resolutions and rolled up automatically. Every value is configurable;
the maximum is `365d`.

| Key | Default | Resolution |
|---|---|---|
| `retention.raw` | `2d` | Every sample (`collector.interval`) |
| `retention.minute` | `30d` | 1-minute min / max / avg |
| `retention.hour` | `365d` | 1-hour min / max / avg |
| `retention.logs` | `30d` | Fluentd log lines |

Charts pick the best resolution for the selected time range. The current database size is shown under
**Settings → Storage**.

## TLS

flui always serves HTTPS.

- **Own certificate (recommended):** set `server.tls.cert_file` and `server.tls.key_file`. flui reloads them
  when the files change, so certificate renewal (e.g. certbot) needs no restart.
- **Self-signed (default):** flui creates an internal CA and a server certificate in `<data_dir>/tls/`.
  Show the fingerprint with `flui-server tls fingerprint`.

Agents always use mTLS with flui's internal CA, independent of the certificate shown to browsers.

## Agent

`/etc/flui-agent/agent.yaml`, environment prefix `FLUI_AGENT_`.

| Key | Default | Description |
|---|---|---|
| `server_url` | – | flui URL, written by `flui-agent enroll` |
| `data_dir` | `/var/lib/flui-agent` | Certificate, key, offline queue, log offsets |
| `interval` | `10s` | Collection interval |
| `queue_max_bytes` | `104857600` | Offline queue limit (100 MB); oldest data is dropped first |
| `targets[].name` | – | Display name of the Fluentd instance |
| `targets[].monitor_url` | `http://127.0.0.1:24220` | `monitor_agent` URL |
| `targets[].workers` | `1` | Number of Fluentd workers |
| `targets[].systemd_unit` | `fluentd` | Find the process by systemd unit … |
| `targets[].pid_file` | – | … or by PID file |
| `targets[].log_file` | `/var/log/fluent/fluentd.log` | Fluentd log file to tail |
| `targets[].buffer_paths` | `[]` | Buffer directories to measure |
