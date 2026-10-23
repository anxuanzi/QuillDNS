package httpapi

import (
	"maps"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/observability"
)

// trend serves query volume and latency buckets for a selectable window.
func (a *API) trend(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}
	if !slices.Contains(observability.TrendWindows(), window) {
		problem(w, 400, "window must be one of "+strings.Join(observability.TrendWindows(), ", "))
		return
	}
	points, err := a.Telemetry.Trend(r.Context(), time.Now(), window)
	if err != nil {
		problem(w, 503, "query history unavailable")
		return
	}
	respond(w, 200, map[string]any{"data": points, "bucket_seconds": observability.TrendBucketSeconds(window)})
}

// tops serves ranked domains, clients, record types and response codes.
func (a *API) tops(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}
	span, ok := observability.TrendSpan(window)
	if !ok {
		problem(w, 400, "window must be one of "+strings.Join(observability.TrendWindows(), ", "))
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			problem(w, 400, "limit must be between 1 and 50")
			return
		}
		limit = parsed
	}
	// Truncating the window start to the minute keeps repeated requests on one
	// cache key without materially changing minute-scale rankings.
	since := time.Now().Add(-span).Truncate(time.Minute)
	tops, err := a.Telemetry.Tops(r.Context(), since, limit)
	if err != nil {
		problem(w, 503, "query history unavailable")
		return
	}
	respond(w, 200, map[string]any{"data": tops})
}

// resolverHealth serves cache and upstream forwarder health snapshots.
func (a *API) resolverHealth(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]any{
		"cache":     a.DNS.CacheStats(),
		"upstreams": a.DNS.UpstreamHealth(),
	})
}

// prometheusMetrics exposes process and resolver telemetry in the Prometheus
// text exposition format, implemented with the standard library only.
func (a *API) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	var b strings.Builder
	summary := a.Telemetry.Summary()
	writeGauge(&b, "quilldns_build_info", "Build metadata.", 1, map[string]string{"version": "dev"})
	writeGauge(&b, "quilldns_uptime_seconds", "Seconds since the process started serving.", time.Since(a.started).Seconds(), nil)
	for _, source := range []string{"local", "cache", "upstream"} {
		var count uint64
		switch source {
		case "local":
			count = summary.Local
		case "cache":
			count = summary.Cache
		case "upstream":
			count = summary.Upstream
		}
		writeCounter(&b, "quilldns_queries_total", "Total DNS queries answered by source.", count, map[string]string{"source": source})
	}
	writeCounter(&b, "quilldns_query_errors_total", "Queries answered with a non-NOERROR response code.", summary.Errors, nil)
	rcodeCounts := a.Telemetry.RcodeCounts()
	rcodes := make([]string, 0, len(rcodeCounts))
	for rcode := range rcodeCounts {
		rcodes = append(rcodes, rcode)
	}
	slices.Sort(rcodes)
	for _, rcode := range rcodes {
		writeCounter(&b, "quilldns_responses_total", "Total DNS responses by response code.", rcodeCounts[rcode], map[string]string{"rcode": rcode})
	}
	bounds, counts, sumSeconds := a.Telemetry.LatencyHistogram()
	b.WriteString("# HELP quilldns_query_duration_seconds DNS response latency.\n# TYPE quilldns_query_duration_seconds histogram\n")
	for i, bound := range bounds {
		b.WriteString("quilldns_query_duration_seconds_bucket{le=\"" + formatPromFloat(bound) + "\"} " + strconv.FormatUint(counts[i], 10) + "\n")
	}
	b.WriteString("quilldns_query_duration_seconds_bucket{le=\"+Inf\"} " + strconv.FormatUint(counts[len(counts)-1], 10) + "\n")
	b.WriteString("quilldns_query_duration_seconds_sum " + formatPromFloat(sumSeconds) + "\n")
	b.WriteString("quilldns_query_duration_seconds_count " + strconv.FormatUint(counts[len(counts)-1], 10) + "\n")

	cache := a.DNS.CacheStats()
	writeGauge(&b, "quilldns_cache_entries", "Entries currently held in the resolver response cache.", float64(cache.Entries), nil)
	writeGauge(&b, "quilldns_cache_max_entries", "Configured resolver response cache capacity.", float64(cache.Max), nil)
	writeCounter(&b, "quilldns_cache_hits_total", "Responses served from the resolver response cache.", cache.Hits, nil)
	writeCounter(&b, "quilldns_cache_misses_total", "Responses that missed the resolver response cache.", cache.Misses, nil)
	writeCounter(&b, "quilldns_cache_evictions_total", "Entries removed from the resolver response cache.", cache.Evictions, nil)

	for _, upstream := range a.DNS.UpstreamHealth() {
		labels := map[string]string{"upstream": upstream.Address}
		writeCounter(&b, "quilldns_upstream_requests_total", "Forwarded upstream exchanges by result.", upstream.Requests-upstream.Errors, withResult(labels, "ok"))
		writeCounter(&b, "quilldns_upstream_requests_total", "", upstream.Errors, withResult(labels, "error"))
		writeCounterFloat(&b, "quilldns_upstream_latency_seconds_sum", "Cumulative upstream exchange latency.", upstream.LatencySumMS/1000, labels)
		writeCounterFloat(&b, "quilldns_upstream_latency_seconds_count", "Upstream exchanges counted for latency.", float64(upstream.Requests), labels)
	}
	writeCounter(&b, "quilldns_log_events_dropped_total", "Query log events dropped before persistence.", summary.Dropped, nil)
	writeGauge(&b, "go_goroutines", "Number of goroutines that currently exist.", float64(runtime.NumGoroutine()), nil)
	_, _ = w.Write([]byte(b.String()))
}

func withResult(labels map[string]string, result string) map[string]string {
	extended := make(map[string]string, len(labels)+1)
	maps.Copy(extended, labels)
	extended["result"] = result
	return extended
}

func writeCounter(b *strings.Builder, name, help string, value uint64, labels map[string]string) {
	if help != "" {
		b.WriteString("# HELP " + name + " " + help + "\n# TYPE " + name + " counter\n")
	}
	b.WriteString(name + formatLabels(labels) + " " + strconv.FormatUint(value, 10) + "\n")
}

func writeGauge(b *strings.Builder, name, help string, value float64, labels map[string]string) {
	if help != "" {
		b.WriteString("# HELP " + name + " " + help + "\n# TYPE " + name + " gauge\n")
	}
	b.WriteString(name + formatLabels(labels) + " " + formatPromFloat(value) + "\n")
}

// writeCounterFloat emits a counter whose cumulative value is fractional,
// such as summed seconds of upstream latency.
func writeCounterFloat(b *strings.Builder, name, help string, value float64, labels map[string]string) {
	if help != "" {
		b.WriteString("# HELP " + name + " " + help + "\n# TYPE " + name + " counter\n")
	}
	b.WriteString(name + formatLabels(labels) + " " + formatPromFloat(value) + "\n")
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key + `="` + escapePromLabel(labels[key]) + `"`)
	}
	b.WriteByte('}')
	return b.String()
}

func escapePromLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value)
}

func formatPromFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
