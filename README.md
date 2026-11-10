# flui

A minimal, secure web UI for monitoring one or more [Fluentd](https://www.fluentd.org/) instances.

- **Buffers & throughput** – queue/stage size, available buffer space, retries, records/s per output plugin.
- **Nothing to install on Fluentd hosts** – flui polls Fluentd's built-in `monitor_agent` (TCP 24220).
- **Easy to run** – a single static binary with an embedded UI and SQLite. No external database, no Prometheus.
- **Secure by default** – TLS everywhere, Argon2id passwords, role-based access, audit log.

> **Status:** early development. Interfaces and commands may change before `v1.0`.

## How it works

```
 Browser ──HTTPS──▶ flui-server ──pull──▶ Fluentd monitor_agent :24220
```

flui pulls `/api/plugins.json` from every configured node every `collector.interval` (default 10 s) and
stores buffer, retry and throughput metrics in SQLite. Nothing is installed on the Fluentd hosts; you only
add a `monitor_agent` source block to the Fluentd config ([docs/setup.md](docs/setup.md)).

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
| [Installation](docs/installation.md) | Packages, Docker, Compose, first login |
| [Connecting nodes](docs/setup.md) | `monitor_agent` snippet, firewall rules |
| [Configuration](docs/configuration.md) | All settings and retention |
| [Security](docs/security.md) | Security model and hardening |
| [Architecture](docs/architecture.md) | Components and data flow |
| [FAQ](docs/faq.md) | Troubleshooting |

## Security

Please report security issues as described in [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE)
