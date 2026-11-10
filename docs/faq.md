# FAQ

## A node shows **Down**

flui cannot reach `monitor_agent`. From the flui server:

```sh
curl -s http://<fluentd-host>:24220/api/plugins.json | head
```

- Connection refused → check `bind` / `port` in the `monitor_agent` block and that Fluentd was reloaded.
- Timeout → a firewall is blocking TCP 24220 from the flui server ([rules](setup.md#network-and-firewall)).

## A node shows **Stale**

Data stopped arriving but the node was reachable before. Usually Fluentd is overloaded or restarting.
Check the node's last collection time and the Fluentd log.

## Plugins are named `object:<number>`

Fluentd generates those IDs for outputs without an explicit `@id`. Add a unique `@id` to every
`<match>` block and reload; see [Plugin names without `@id`](setup.md#plugin-names-without-id).

## Browser shows a certificate warning

flui uses a self-signed certificate by default. Configure your own certificate ([TLS](configuration.md#tls)).

## I lost the admin password

On the flui server:

```sh
sudo -u flui flui-server user reset-password admin
```

## The database is growing too fast

Lower `retention` ([configuration](configuration.md#retention)).

## Does flui support Prometheus, Kubernetes discovery, alerts, SSO?

Not yet. Alerts and SSO (OIDC/LDAP) are on the roadmap. Prometheus is intentionally not required.
