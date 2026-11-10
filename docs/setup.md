# Connecting a Fluentd node

Nothing is installed on the Fluentd host. Fluentd's built-in `monitor_agent` exposes buffer and retry
stats; flui **pulls** them every 10 s over HTTP.

CPU, memory, disk and network of the Fluentd host are **not** collected – `monitor_agent` does not
report them.

## 1. Add the node in flui

**Nodes → Add node**, enter a name and the monitor URL (for example `http://10.0.0.21:24220`).
flui shows the `monitor_agent` snippet below after the node is created.

## 2. Enable `monitor_agent`

```
<source>
  @type monitor_agent
  bind 10.0.0.21        # the host's private IP – never 0.0.0.0 on a public interface
  port 24220
  include_config false  # do not expose your config (it may contain secrets)
</source>
```

Reload Fluentd (`sudo systemctl reload fluentd`). The node turns **Up** in flui within one collection interval.

## Network and firewall

| From | To | Port | Purpose |
|---|---|---|---|
| flui server | Fluentd host | TCP 24220 | Metrics pull |

`monitor_agent` has **no authentication and no TLS**. Allow port 24220 **only from the flui server's IP**, for example:

```sh
# firewalld
sudo firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=10.0.0.10/32 port port=24220 protocol=tcp accept'
sudo firewall-cmd --reload
# ufw
sudo ufw allow from 10.0.0.10 to any port 24220 proto tcp
```

## What is collected

Per output plugin, from `/api/plugins.json`:

| Metric | Meaning |
|---|---|
| `buffer_queue_length` | Chunks waiting to be flushed |
| `buffer_stage_length` | Chunks still being filled |
| `buffer_total_queued_size` | Total buffered bytes |
| `buffer_available_buffer_space_ratios` | Free buffer space (%) |
| `retry_count` | Cumulative retries, also shown as a rate |
| `emit_records`, `write_count` | Shown as rates (records/s, writes/s) |

A node is shown as **Down** when it cannot be reached and **Stale** when no data arrived within
`collector.stale_after` (see [configuration](configuration.md)).

## Reading the node dashboard

The overview charts aggregate all reported output plugins. Buffered bytes, queued chunks and
record rates are summed by timestamp; free buffer is the **lowest** reported percentage, not disk space.
Missing values are excluded, not treated as zero. Buffer charts sum each plugin's peak in each time bucket,
so they are not an exact simultaneous peak. Output rates can count the same record more than once if it
passes through multiple outputs.

The plugin table shows each plugin's latest bucket in the selected range. Search by ID or type;
click a column heading to sort. **Attention first** prioritizes plugins with increasing retries or less
than 10% free buffer. The activity filter includes these plus plugins with buffered data or output activity.
The retry counter is cumulative and does not alone indicate a current failure. **Earlier sample** marks
plugins absent from the newest bucket; these are excluded from activity filters.
Select a plugin name to open its charts. Only one plugin's charts are open at a time.

**Up** means the monitor endpoint is reachable, not that every output delivered its records successfully.
Zero buffered bytes can mean no traffic or fast flushing; use throughput and retries alongside it.
Check the last collection time when a node is down or stale.

### Plugin names without `@id`

Metrics work without `@id`. Fluentd then reports generated IDs such as `object:698`, which can change on
restart and cannot reliably identify a log route. With `include_config false`, flui cannot infer the match
tag or the destination index from these IDs. Keep this setting: exposing the config may expose credentials.

For stable names and continuous history, give each output a unique `@id`, for example `@id out_access`
inside its `<match>` block. Existing history under a generated ID remains under that old ID.
