package collector

import (
	"context"
	"database/sql"
	"time"
)

func insertSamples(ctx context.Context, tx *sql.Tx, nodeID int64, ts time.Time, ps []PluginMetrics) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO plugin_samples (node_id, ts, plugin_id, type,
		buffer_queue_length, buffer_queue_byte_size, buffer_stage_length, buffer_stage_byte_size, buffer_total_queued_size,
		buffer_available_ratio, retry_count, emit_records, emit_count, write_count, rollback_count, slow_flush_count, flush_time_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, p := range ps {
		if _, err := stmt.ExecContext(ctx, nodeID, ts.Unix(), p.PluginID, p.Type,
			intOrNil(p.BufferQueueLength), intOrNil(p.BufferQueueByteSize), intOrNil(p.BufferStageLength),
			intOrNil(p.BufferStageByteSize), intOrNil(p.BufferTotalQueuedSize), floatOrNil(p.BufferAvailableRatio),
			intOrNil(p.RetryCount), intOrNil(p.EmitRecords), intOrNil(p.EmitCount), intOrNil(p.WriteCount),
			intOrNil(p.RollbackCount), intOrNil(p.SlowFlushCount), intOrNil(p.FlushTimeCount)); err != nil {
			return err
		}
	}
	return nil
}

func intOrNil(v *float64) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

func floatOrNil(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// PluginSeries is the time series of one output plugin. Values are aligned with TS; nil means no data.
type PluginSeries struct {
	PluginID string  `json:"plugin_id"`
	Type     string  `json:"type"`
	TS       []int64 `json:"ts"`

	BufferTotalQueuedSize []*float64 `json:"buffer_total_queued_size"`
	BufferQueueLength     []*float64 `json:"buffer_queue_length"`
	BufferStageLength     []*float64 `json:"buffer_stage_length"`
	BufferAvailableRatio  []*float64 `json:"buffer_available_ratio"`
	EmitRecordsRate       []*float64 `json:"emit_records_rate"`
	WriteRate             []*float64 `json:"write_rate"`
	RetryRate             []*float64 `json:"retry_rate"`
	RetryCount            []*float64 `json:"retry_count"`
}

type bucket struct {
	ts                                int64
	lastTS                            int64
	queued, queueLen, stageLen, avail *float64
	emit, write, retry                *float64
}

// Series returns per-plugin series for a node between from and to, aggregated into buckets of step seconds.
// Gauges use the bucket's worst value (max size/length, min free space); counters are converted to per-second rates.
func Series(ctx context.Context, db *sql.DB, nodeID int64, from, to time.Time, step int64) ([]PluginSeries, error) {
	if step < 1 {
		step = 1
	}
	rows, err := db.QueryContext(ctx, `SELECT plugin_id, type, ts, buffer_total_queued_size, buffer_queue_length,
		buffer_stage_length, buffer_available_ratio, emit_records, write_count, retry_count
		FROM plugin_samples WHERE node_id = ? AND ts >= ? AND ts <= ?
		ORDER BY plugin_id, ts`, nodeID, from.Unix()-step, to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PluginSeries{}
	var cur *PluginSeries
	var buckets []*bucket
	flush := func() {
		if cur != nil {
			fillSeries(cur, buckets, from.Unix())
			out = append(out, *cur)
		}
	}
	for rows.Next() {
		var (
			pluginID, typ                     string
			ts                                int64
			queued, queueLen, stageLen, avail sql.NullFloat64
			emit, write, retry                sql.NullFloat64
		)
		if err := rows.Scan(&pluginID, &typ, &ts, &queued, &queueLen, &stageLen, &avail, &emit, &write, &retry); err != nil {
			return nil, err
		}
		if cur == nil || cur.PluginID != pluginID {
			flush()
			cur, buckets = &PluginSeries{PluginID: pluginID, Type: typ}, nil
		}
		bts := ts / step * step
		if len(buckets) == 0 || buckets[len(buckets)-1].ts != bts {
			buckets = append(buckets, &bucket{ts: bts})
		}
		b := buckets[len(buckets)-1]
		b.lastTS = ts
		b.queued = maxF(b.queued, queued)
		b.queueLen = maxF(b.queueLen, queueLen)
		b.stageLen = maxF(b.stageLen, stageLen)
		b.avail = minF(b.avail, avail)
		b.emit, b.write, b.retry = ptr(emit), ptr(write), ptr(retry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	flush()
	return out, nil
}

func fillSeries(s *PluginSeries, bs []*bucket, from int64) {
	for i, b := range bs {
		var emitR, writeR, retryR *float64
		if i > 0 {
			p := bs[i-1]
			dt := float64(b.lastTS - p.lastTS)
			emitR, writeR, retryR = rate(b.emit, p.emit, dt), rate(b.write, p.write, dt), rate(b.retry, p.retry, dt)
		}
		if b.ts < from {
			continue
		}
		s.TS = append(s.TS, b.ts)
		s.BufferTotalQueuedSize = append(s.BufferTotalQueuedSize, b.queued)
		s.BufferQueueLength = append(s.BufferQueueLength, b.queueLen)
		s.BufferStageLength = append(s.BufferStageLength, b.stageLen)
		s.BufferAvailableRatio = append(s.BufferAvailableRatio, b.avail)
		s.EmitRecordsRate = append(s.EmitRecordsRate, emitR)
		s.WriteRate = append(s.WriteRate, writeR)
		s.RetryRate = append(s.RetryRate, retryR)
		s.RetryCount = append(s.RetryCount, b.retry)
	}
}

func rate(cur, prev *float64, dt float64) *float64 {
	d, ok := delta(cur, prev)
	if !ok || dt <= 0 {
		return nil
	}
	v := d / dt
	return &v
}

func ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func maxF(cur *float64, v sql.NullFloat64) *float64 {
	if v.Valid && (cur == nil || v.Float64 > *cur) {
		return ptr(v)
	}
	return cur
}

func minF(cur *float64, v sql.NullFloat64) *float64 {
	if v.Valid && (cur == nil || v.Float64 < *cur) {
		return ptr(v)
	}
	return cur
}

// Prune deletes samples older than before.
func Prune(ctx context.Context, db *sql.DB, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM plugin_samples WHERE ts < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
