const sumFields = [
  'buffer_total_queued_size', 'buffer_queue_length', 'buffer_stage_length',
  'emit_records_rate', 'write_rate', 'retry_rate',
];
const fields = [...sumFields, 'buffer_available_ratio', 'retry_count'];
export const pluginKey = p => p.plugin_id;

// Align by timestamp, never by array index: plugins can appear/disappear independently.
// Unknown values stay unknown; aggregates include only values actually reported.
export function aggregatePlugins(plugins) {
  const buckets = new Map();
  for (const p of plugins) {
    p.ts.forEach((ts, i) => {
      if (!buckets.has(ts)) buckets.set(ts, {});
      const b = buckets.get(ts);
      for (const field of sumFields) {
        const value = p[field]?.[i];
        if (Number.isFinite(value)) b[field] = (b[field] ?? 0) + value;
      }
      const free = p.buffer_available_ratio?.[i];
      if (Number.isFinite(free)) b.buffer_available_ratio = Math.min(b.buffer_available_ratio ?? Infinity, free);
    });
  }
  const ts = [...buckets.keys()].sort((a, b) => a - b);
  return Object.fromEntries([
    ['ts', ts],
    ...[...sumFields, 'buffer_available_ratio'].map(field => [field, ts.map(t => buckets.get(t)[field] ?? null)]),
  ]);
}

export function pluginRows(plugins) {
  const newest = Math.max(0, ...plugins.map(p => p.ts.at(-1) ?? 0));
  return plugins.filter(p => p.ts.length).map(p => {
    const i = p.ts.length - 1;
    const row = { ...p, key: pluginKey(p), sampledAt: p.ts[i] };
    for (const field of fields) row[field] = Number.isFinite(p[field]?.[i]) ? p[field][i] : null;
    row.attention = row.retry_rate > 0 || (row.buffer_available_ratio != null && row.buffer_available_ratio < 10);
    row.active = ['buffer_total_queued_size', 'buffer_queue_length', 'buffer_stage_length', 'emit_records_rate', 'write_rate']
      .some(field => row[field] > 0);
    row.historical = row.sampledAt < newest;
    row.state = row.historical ? 'Earlier sample' : row.attention ? 'Attention' : row.active ? 'Active'
      : fields.every(field => row[field] == null) ? 'No values' : 'Idle';
    return row;
  });
}

export function filterAndSortPlugins(rows, query, filter, sort, descending) {
  const search = query.trim().toLowerCase();
  return rows.filter(p => `${p.plugin_id} ${p.type}`.toLowerCase().includes(search))
    .filter(p => filter === 'all' || (!p.historical && (filter === 'attention' ? p.attention : p.attention || p.active)))
    .sort((a, b) => {
      let result;
      if (sort === 'attention') {
        result = Number(a.historical) - Number(b.historical)
          || Number(b.attention) - Number(a.attention)
          || Number(b.active) - Number(a.active)
          || (b.buffer_total_queued_size ?? -1) - (a.buffer_total_queued_size ?? -1);
      } else {
        const av = a[sort], bv = b[sort];
        // Missing values always sort last, in either direction.
        if (av == null || bv == null) return av == null ? (bv == null ? 0 : 1) : -1;
        result = typeof av === 'string' ? av.localeCompare(bv) : av - bv;
        if (descending) result *= -1;
      }
      return result || a.plugin_id.localeCompare(b.plugin_id);
    });
}
