package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20

// PluginMetrics is one output plugin from monitor_agent's /api/plugins.json. Absent values are nil.
type PluginMetrics struct {
	PluginID     string `json:"plugin_id"`
	Category     string `json:"plugin_category"`
	Type         string `json:"type"`
	OutputPlugin bool   `json:"output_plugin"`

	BufferQueueLength     *float64 `json:"buffer_queue_length"`
	BufferQueueByteSize   *float64 `json:"buffer_queue_byte_size"`
	BufferStageLength     *float64 `json:"buffer_stage_length"`
	BufferStageByteSize   *float64 `json:"buffer_stage_byte_size"`
	BufferTotalQueuedSize *float64 `json:"buffer_total_queued_size"`
	BufferAvailableRatio  *float64 `json:"buffer_available_buffer_space_ratios"`
	RetryCount            *float64 `json:"retry_count"`
	EmitRecords           *float64 `json:"emit_records"`
	EmitCount             *float64 `json:"emit_count"`
	WriteCount            *float64 `json:"write_count"`
	RollbackCount         *float64 `json:"rollback_count"`
	SlowFlushCount        *float64 `json:"slow_flush_count"`
	FlushTimeCount        *float64 `json:"flush_time_count"`
}

func newHTTPClient(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	tr.MaxIdleConnsPerHost = 2
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects are not followed")
		},
	}
}

// fetchPlugins returns the output plugins reported by the monitor_agent at baseURL.
func fetchPlugins(ctx context.Context, client *http.Client, baseURL string) ([]PluginMetrics, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/api/plugins.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("monitor_agent returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Plugins []PluginMetrics `json:"plugins"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid monitor_agent response: %w", err)
	}
	out := body.Plugins[:0]
	for _, p := range body.Plugins {
		if p.OutputPlugin || p.Category == "output" {
			if len(p.PluginID) > 128 {
				p.PluginID = p.PluginID[:128]
			}
			if len(p.Type) > 64 {
				p.Type = p.Type[:64]
			}
			out = append(out, p)
		}
	}
	return out, nil
}

// cleanErr strips the request URL from transport errors to keep messages short.
func cleanErr(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		if uerr.Timeout() {
			return errors.New("timed out")
		}
		return uerr.Err
	}
	return err
}
