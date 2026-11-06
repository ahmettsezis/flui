# flui

A minimal, secure web UI for monitoring one or more [Fluentd](https://www.fluentd.org/) instances.

- **Buffers & throughput** – queue/stage size, available buffer space, retries, records/s per output plugin.
- **System resources** – CPU, memory, disk and network of the Fluentd host (agent mode).
- **Fluentd logs** – warnings and errors from Fluentd itself, searchable and live-tailed *(later release)*.
- **Easy to run** – a single static binary with an embedded UI and SQLite. No external database, no Prometheus.
- **Secure by default** – TLS everywhere, Argon2id passwords, optional TOTP 2FA, role-based access, audit log.

> **Status:** early development. Interfaces and commands may change before `v1.0`.

## How it works

flui supports two collection modes. You can mix them in the same installation.

| | Agentless | Agent |
|---|---|---|
| Install on Fluentd host | Nothing (config snippet only) | `flui-agent` (single binary) |
| Buffers, retries, throughput | Yes | Yes |
| Fluentd logs | Yes (Fluentd pushes them) | Yes (agent tails the log file) |
| CPU / memory / disk / network | No | Yes |
| Connection direction | flui → Fluentd `:24220` | agent → flui `:8443` (mTLS) |

```
 Browser ──HTTPS──▶ flui-server ──pull──▶ Fluentd monitor_agent :24220   (agentless)
                         ▲   ▲
                         │   └──HTTPS push── Fluentd logs (out_http)       (agentless)
                         └──────mTLS push── flui-agent ── Fluentd          (agent)
```

## Quick start

### Docker

```sh
docker run -d --name flui -p 8443:8443 -v flui-data:/data ghcr.io/ahmettsezis/flui:latest
docker logs flui 2>&1 | grep "initial admin password"
```

### Linux VM (systemd)

```sh
sudo dpkg -i flui-server_<version>_linux_amd64.deb   # or: sudo rpm -i flui-server-<version>.x86_64.rpm
sudo systemctl enable --now flui-server
sudo cat /var/lib/flui/initial-admin-password
```

Open `https://<host>:8443`, sign in as `admin` and change the password.
Then add your first Fluentd node from **Nodes → Add node**.

## Documentation

| Topic | |
|---|---|
| [Installation](docs/installation.md) | Packages, Docker, Compose, Helm, first login |
| [Agentless mode](docs/agentless.md) | Fluentd config snippets, firewall rules |
| [Agent mode](docs/agent.md) | Installing and enrolling `flui-agent` |
| [Configuration](docs/configuration.md) | All settings and retention |
| [Security](docs/security.md) | Security model and hardening |
| [Architecture](docs/architecture.md) | Components and data flow |
| [FAQ](docs/faq.md) | Troubleshooting |

## Security

Please report security issues as described in [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE)
