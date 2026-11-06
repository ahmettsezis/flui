# Agentless mode

Nothing is installed on the Fluentd host. You add two config blocks to Fluentd:

1. **Metrics** – Fluentd's built-in `monitor_agent` exposes buffer and retry stats; flui **pulls** them every 10 s.
2. **Logs** – Fluentd's built-in `out_http` **pushes** Fluentd's own logs to flui.

CPU, memory, disk and network are **not** available in this mode. Use the [agent](agent.md) if you need them.

## 1. Add the node in flui

**Nodes → Add node → Agentless**, enter a name and the monitor URL (for example `http://10.0.0.21:24220`).
flui generates the two snippets below with a node-specific token already filled in. The token is shown once.

## 2. Enable metrics (`monitor_agent`)

```
<source>
  @type monitor_agent
  bind 10.0.0.21        # the host's private IP – never 0.0.0.0 on a public interface
  port 24220
  include_config false  # do not expose your config (it may contain secrets)
</source>
```

**Multiple workers:** with `<system> workers N </system>`, each worker listens on `port + worker_id`
(24220, 24221, …). Set **Workers = N** on the node in flui and all of them are collected.

## 3. Ship Fluentd logs (`out_http`)

```
<label @FLUENT_LOG>
  <filter fluent.**>
    @type record_transformer
    <record>
      tag ${tag}
      time ${time}
      host ${hostname}
    </record>
  </filter>
  <match fluent.**>
    @type http
    endpoint https://flui.example.com:8443/api/v1/ingest/logs
    headers {"Authorization":"Bearer <NODE_TOKEN>"}
    json_array true
    tls_ca_cert_path /etc/fluent/flui-ca.pem   # omit if flui uses a publicly trusted certificate
    retryable_response_codes [429, 500, 502, 503, 504]
    <format>
      @type json
    </format>
    <buffer>
      flush_interval 5s
      chunk_limit_size 512k
      total_limit_size 16m
      overflow_action drop_oldest_chunk
    </buffer>
  </match>
</label>
```

- Copy flui's CA certificate (`<data_dir>/tls/ca.pem` on the flui server, or **Settings → TLS → Download CA**)
  to `/etc/fluent/flui-ca.pem`.
- Keep the config file readable only by the Fluentd user, it contains the token:
  `chmod 0640 /etc/fluent/fluentd.conf`.
- If you already have a `<label @FLUENT_LOG>`, put both outputs inside one `<match>` with `@type copy`.

Reload Fluentd (`sudo systemctl reload fluentd`). The node turns **Up** in flui within one collection interval.

## Network and firewall

| From | To | Port | Purpose |
|---|---|---|---|
| flui server | Fluentd host | TCP 24220 (+ one per extra worker) | Metrics pull |
| Fluentd host | flui server | TCP 8443 | Log push |

`monitor_agent` has **no authentication and no TLS**. Allow port 24220 **only from the flui server's IP**, for example:

```sh
# firewalld
sudo firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=10.0.0.10/32 port port=24220 protocol=tcp accept'
sudo firewall-cmd --reload
# ufw
sudo ufw allow from 10.0.0.10 to any port 24220 proto tcp
```

If you cannot restrict the network, use the [agent](agent.md) instead – it reads `monitor_agent` on localhost
and only makes outbound mTLS connections.

## What is collected

Per output plugin, from `/api/plugins.json`:

| Metric | Meaning |
|---|---|
| `buffer_queue_length`, `buffer_queue_byte_size` | Chunks waiting to be flushed |
| `buffer_stage_length`, `buffer_stage_byte_size` | Chunks still being filled |
| `buffer_total_queued_size` | Total buffered bytes |
| `buffer_available_buffer_space_ratios` | Free buffer space (%) |
| `retry_count`, `retry` | Retries and next retry time |
| `emit_records`, `emit_count`, `write_count` | Shown as rates (records/s, writes/s) |
| `rollback_count`, `slow_flush_count`, `flush_time_count` | Output health |

A node is shown as **Down** when it cannot be reached and **Stale** when no data arrived within
`collector.stale_after` (see [configuration](configuration.md)).
