# Agent mode

`flui-agent` is a single small binary (~10 MB, < 20 MB RAM) that runs next to Fluentd. It:

- reads Fluentd's `monitor_agent` on **localhost** (port 24220 is never exposed),
- collects CPU, memory, disk and network for the Fluentd process and host,
- tails the Fluentd log file,
- **pushes** everything to flui over **mTLS**. It only makes outbound connections to flui on TCP 8443.

## 1. Enable `monitor_agent` on localhost

```
<source>
  @type monitor_agent
  bind 127.0.0.1
  port 24220
  include_config false
</source>
```

## 2. Install the agent

```sh
# Debian / Ubuntu
sudo dpkg -i flui-agent_<version>_linux_amd64.deb
# RHEL / Rocky / Alma
sudo rpm -i flui-agent-<version>.x86_64.rpm
```

The agent runs as user `flui-agent` and is added to the Fluentd group so it can read the Fluentd log file
and buffer directories. It does not need root.

## 3. Enroll

In flui: **Nodes → Add node → Agent**. flui shows a command with a one-time token (valid 1 hour):

```sh
sudo flui-agent enroll \
  --server https://flui.example.com:8443 \
  --token <ENROLLMENT_TOKEN> \
  --ca-fingerprint sha256:<FINGERPRINT>
sudo systemctl enable --now flui-agent
```

What happens:

1. The agent connects to flui and checks that the server's CA matches `--ca-fingerprint` (no trust on first use).
2. It creates a private key locally (the key never leaves the host) and sends a certificate request with the token.
3. flui's internal CA issues a short-lived client certificate. The agent renews it automatically.

## 4. Configure targets

`/etc/flui-agent/agent.yaml` – the defaults match a standard `fluent-package` install:

```yaml
server_url: https://flui.example.com:8443   # written by `enroll`
interval: 10s

targets:
  - name: fluentd
    monitor_url: http://127.0.0.1:24220
    workers: 1
    systemd_unit: fluentd                     # or: pid_file: /var/run/fluent/fluentd.pid
    log_file: /var/log/fluent/fluentd.log
    buffer_paths:
      - /var/log/fluent/buffer
```

Several Fluentd instances on one host? Add more entries to `targets`. Restart the agent after changes.

All agent settings are listed in [configuration.md](configuration.md#agent).

## What is collected

Everything from [agentless mode](agentless.md#what-is-collected), plus:

| Area | Metrics |
|---|---|
| Fluentd process (supervisor + workers) | CPU %, RSS, open file descriptors, threads, disk read/write bytes |
| Host | CPU %, memory, load average |
| Network | NIC rx/tx bytes/s, errors and drops; number of Fluentd TCP connections |
| Disk | Size of each buffer path; usage % of the filesystem it lives on |
| Logs | Fluentd log file, rotation-aware |

If flui is unreachable, the agent queues data on disk (default limit 100 MB) and sends it later.

## Revoke or remove

- **Revoke:** in flui, **Nodes → (node) → Revoke agent**. The certificate is rejected immediately.
- **Uninstall:** `sudo systemctl disable --now flui-agent`, remove the package, delete `/var/lib/flui-agent`.
