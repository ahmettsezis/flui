# FAQ

## A node shows **Down**

flui cannot reach `monitor_agent`. From the flui server:

```sh
curl -s http://<fluentd-host>:24220/api/plugins.json | head
```

- Connection refused → check `bind` / `port` in the `monitor_agent` block and that Fluentd was reloaded.
- Timeout → a firewall is blocking TCP 24220 from the flui server ([rules](agentless.md#network-and-firewall)).

For agent nodes, check the agent: `sudo journalctl -u flui-agent -n 50`.

## A node shows **Stale**

Data stopped arriving but the node was reachable before. Usually Fluentd is overloaded or restarting, or the
agent cannot reach flui. Check the node's **Logs** tab and the agent journal.

## No Fluentd logs appear

- The `<label @FLUENT_LOG>` block is missing or Fluentd was not reloaded.
- Wrong token → flui logs `ingest: invalid token` and Fluentd logs HTTP 401. Regenerate the snippet for the node.
- TLS error in Fluentd's log → `tls_ca_cert_path` does not point to flui's CA ([agentless.md](agentless.md#3-ship-fluentd-logs-out_http)).

## Browser shows a certificate warning

flui uses a self-signed certificate by default. Configure your own certificate ([TLS](configuration.md#tls)).

## `flui-agent enroll` fails with "CA fingerprint mismatch"

The fingerprint does not match the server you are connecting to. Copy the command again from flui; if it still
fails, something between the agent and flui is intercepting TLS (e.g. a proxy). Do not skip the check.

## I lost the admin password

On the flui server:

```sh
sudo -u flui flui-server user reset-password admin
```

## The database is growing too fast

Lower `retention.logs` first – logs are usually the largest part. Check **Settings → Storage** for sizes.

## How do I back up flui?

See [Backup and restore](installation.md#backup-and-restore).

## Does flui support Prometheus, Kubernetes discovery, alerts, SSO?

Not yet. Alerts and SSO (OIDC/LDAP) are on the roadmap. Prometheus is intentionally not required.
