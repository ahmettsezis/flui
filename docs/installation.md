# Installation

flui is a single binary: **`flui-server`** – the web UI, API and storage. Install it once;
nothing is installed on the Fluentd hosts ([setup](setup.md)).

## Requirements

| | Minimum |
|---|---|
| OS | Linux (amd64 or arm64). Docker and Kubernetes are also supported. |
| CPU / RAM | 1 vCPU, 256 MB RAM for ~50 Fluentd nodes |
| Disk | ~1 GB per 50 nodes per year of metrics; logs depend on volume |
| Fluentd | v1.14+ (`fluent-package` / `td-agent` 4+) |

Ports: flui listens on **TCP 8443** (HTTPS). For network rules, see [setup](setup.md#network-and-firewall).

## Option 1 – Linux package (recommended for VMs)

Download the package for your platform from the [releases page](https://github.com/ahmettsezis/flui/releases) and verify it:

```sh
sha256sum --ignore-missing -c checksums.txt
```

Install and start:

```sh
# Debian / Ubuntu
sudo dpkg -i flui-server_<version>_linux_amd64.deb
# RHEL / Rocky / Alma
sudo rpm -i flui-server-<version>.x86_64.rpm

sudo systemctl enable --now flui-server
```

The package installs:

| Path | Purpose |
|---|---|
| `/usr/bin/flui-server` | Binary |
| `/etc/flui/flui.yaml` | Configuration ([reference](configuration.md)) |
| `/var/lib/flui/` | Data: database, TLS files, master key |
| `flui-server.service` | Hardened systemd unit, runs as user `flui` |

## Option 2 – Tarball

```sh
tar -xzf flui-server_<version>_linux_amd64.tar.gz
sudo install -m 0755 flui-server /usr/local/bin/
sudo useradd --system --home /var/lib/flui --shell /usr/sbin/nologin flui
sudo install -d -o flui -g flui -m 0700 /var/lib/flui
sudo install -D -m 0644 flui.yaml /etc/flui/flui.yaml
sudo install -m 0644 flui-server.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now flui-server
```

## Option 3 – Docker

```sh
docker run -d --name flui \
  -p 8443:8443 \
  -v flui-data:/data \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  ghcr.io/ahmettsezis/flui:<version>
```

The image is distroless, runs as non-root (UID 65532) and stores everything in `/data`.

## Option 4 – Docker Compose

```sh
curl -fsSLO https://raw.githubusercontent.com/ahmettsezis/flui/main/deploy/compose/docker-compose.yml
docker compose up -d
```

## Option 5 – Kubernetes (Helm)

```sh
helm install flui oci://ghcr.io/ahmettsezis/charts/flui --namespace flui --create-namespace
```

flui uses SQLite, so it runs as a single replica (StatefulSet with a PersistentVolumeClaim).

## First login

On first start flui creates the user `admin` with a random password and writes it **once** to the log
and to `<data_dir>/initial-admin-password` (mode `0600`):

```sh
sudo cat /var/lib/flui/initial-admin-password          # package / tarball
docker logs flui 2>&1 | grep "initial admin password"  # Docker
```

1. Open `https://<host>:8443`. With the default self-signed certificate your browser will warn once;
   compare the fingerprint with `flui-server tls fingerprint`, or [use your own certificate](configuration.md#tls).
2. Sign in as `admin`. You must set a new password; the initial password file is deleted.
3. Add Fluentd nodes: [Connecting a Fluentd node](setup.md).

## Upgrade

Install the new package / image and restart. Database migrations run automatically on start.
Take a [backup](#backup-and-restore) first.

## Backup and restore

```sh
sudo -u flui flui-server backup /var/backups/flui-$(date +%F).db   # online, consistent snapshot
```

Also back up the master key (`/var/lib/flui/master.key` by default) **separately** – without it,
encrypted settings in the database cannot be read.

To restore: stop flui, replace `<data_dir>/flui.db` with the backup, make sure the matching master key is in
place, start flui.
