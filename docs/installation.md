# Installation

flui is a single binary: **`flui-server`** – the web UI, API and storage. Install it once;
nothing is installed on the Fluentd hosts ([setup](setup.md)).

There are no prebuilt packages or images yet; flui is built from source.

## Requirements

| | Minimum |
|---|---|
| OS | Linux (amd64 or arm64), or any host with Docker |
| CPU / RAM | 1 vCPU, 256 MB RAM for ~50 Fluentd nodes |
| Disk | ~1 GB per 50 nodes per year of metrics |
| Fluentd | v1.14+ (`fluent-package` / `td-agent` 4+) |
| Build | Go 1.26+ and Node.js ≥ 20.19 (not needed for the Docker build) |

Ports: flui listens on **TCP 8443** (HTTPS). For network rules, see [setup](setup.md#network-and-firewall).

## Option 1 – Build and install on a Linux VM

```sh
git clone https://github.com/ahmettsezis/flui.git
cd flui
make build            # builds the UI into internal/webui/dist, then bin/flui-server
```

Install with the bundled systemd unit:

```sh
sudo install -m 0755 bin/flui-server /usr/bin/flui-server
sudo useradd --system --home /var/lib/flui --shell /usr/sbin/nologin flui
sudo install -d -o flui -g flui -m 0700 /var/lib/flui
sudo install -D -m 0644 deploy/flui.yaml /etc/flui/flui.yaml
sudo install -m 0644 deploy/systemd/flui-server.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now flui-server
```

This installs:

| Path | Purpose |
|---|---|
| `/usr/bin/flui-server` | Binary |
| `/etc/flui/flui.yaml` | Configuration ([reference](configuration.md)) |
| `/var/lib/flui/` | Data: database, TLS files, master key |
| `flui-server.service` | Hardened systemd unit, runs as user `flui` |

## Option 2 – Docker

```sh
git clone https://github.com/ahmettsezis/flui.git && cd flui
docker build -f deploy/docker/Dockerfile -t flui .
docker run -d --name flui \
  -p 8443:8443 \
  -v flui-data:/data \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  flui
```

The image is distroless, runs as non-root (UID 65532) and stores everything in `/data`.

## Option 3 – Docker Compose

```sh
git clone https://github.com/ahmettsezis/flui.git && cd flui
docker compose -f deploy/compose/docker-compose.yml up -d --build
```

## First login

On first start flui creates the user `admin` with a random password and writes it **once** to the log
and to `<data_dir>/initial-admin-password` (mode `0600`):

```sh
sudo cat /var/lib/flui/initial-admin-password            # systemd install
docker logs flui 2>&1 | grep "initial admin password"    # Docker / Compose
```

1. Open `https://<host>:8443`. With the default self-signed certificate your browser will warn once;
   compare the fingerprint with `flui-server tls fingerprint`, or [use your own certificate](configuration.md#tls).
2. Sign in as `admin`. You must set a new password; the initial password file is deleted.
3. Add Fluentd nodes: [Connecting a Fluentd node](setup.md).

## Upgrade

Rebuild and restart. Database migrations run automatically on start. `flui.db` contains only metric
samples, so deleting it and starting fresh is also safe.
