import { test } from 'node:test';
import assert from 'node:assert/strict';
import { aggregatePlugins, pluginRows, filterAndSortPlugins } from './metrics.js';

test('aggregates sparse plugin timelines by timestamp and preserves unavailable values', () => {
  const result = aggregatePlugins([
    { ts: [10, 30], buffer_total_queued_size: [4, null], buffer_available_ratio: [90, null], emit_records_rate: [null, 3] },
    { ts: [20, 30], buffer_total_queued_size: [8, null], buffer_available_ratio: [70, 60], emit_records_rate: [2, 4] },
    { ts: [10, 30], buffer_total_queued_size: [5, null], buffer_available_ratio: [80, 95], emit_records_rate: [null, null] },
  ]);
  assert.deepEqual(result.ts, [10, 20, 30]);
  assert.deepEqual(result.buffer_total_queued_size, [9, 8, null]);
  assert.deepEqual(result.buffer_available_ratio, [80, 70, 60]);
  assert.deepEqual(result.emit_records_rate, [null, 2, 7]);
  assert.deepEqual(result.retry_rate, [null, null, null]);
  assert.deepEqual(aggregatePlugins([]).ts, []);
});

test('activity uses recent rates and does not revive old samples', () => {
  const rows = pluginRows([
    { plugin_id: 'out_access', type: 'opensearch', ts: [10, 20], retry_count: [100, 100], retry_rate: [null, 0] },
    { plugin_id: 'out_access_2', type: 'opensearch', ts: [20], emit_records_rate: [5] },
    { plugin_id: 'object:old', type: 'opensearch', ts: [10], retry_rate: [2] },
    { plugin_id: 'low_buffer', type: 'file', ts: [20], buffer_available_ratio: [5] },
    { plugin_id: 'unknown', type: 'http', ts: [10, 20], buffer_total_queued_size: [123, null] },
  ]);
  assert.deepEqual(rows.map(p => p.state), ['Idle', 'Active', 'Earlier sample', 'Attention', 'No values']);
  assert.notEqual(rows[0].key, rows[1].key);
  assert.equal(rows[4].buffer_total_queued_size, null);
  const attention = filterAndSortPlugins(rows, '', 'all', 'attention', false);
  assert.equal(attention[0].plugin_id, 'low_buffer');
  assert.equal(attention.at(-1).plugin_id, 'object:old');
  assert.equal(filterAndSortPlugins(rows, 'OPENSearch', 'active', 'plugin_id', false).length, 1);
  assert.equal(filterAndSortPlugins(rows, '', 'attention', 'plugin_id', false).length, 1);
  for (const descending of [true, false]) {
    assert.equal(filterAndSortPlugins(rows, '', 'all', 'retry_count', descending)[0].plugin_id, 'out_access');
  }
});
