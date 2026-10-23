package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
)

// sqliteFixture builds a handler whose query history is backed by SQLite,
// with count events pre-persisted through a flush cycle.
func sqliteFixture(t *testing.T, count int) http.Handler {
	t.Helper()
	dir := t.TempDir()
	store, err := records.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(dir, "queries.sqlite")
	collector, err := observability.Open(path, 64, 7, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	for i := range count {
		source := []string{"local", "cache", "upstream"}[i%3]
		rcode := "NOERROR"
		if i%10 == 5 {
			rcode = "NXDOMAIN"
		}
		collector.Record(observability.Event{
			Time:       base.Add(time.Duration(i) * time.Second),
			Name:       fmt.Sprintf("host%d.example.test.", i%5),
			Type:       "A",
			Client:     fmt.Sprintf("10.0.0.%d:5353", i%3),
			Source:     source,
			Rcode:      rcode,
			DurationMS: float64(i%4) + 1,
		})
	}
	if err := collector.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	collector, err = observability.Open(path, 64, 7, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = collector.Close(context.Background()) })
	// Lifetime counters live in process memory, so seed the reopened instance
	// the same way production traffic would.
	collector.Record(observability.Event{
		Time: time.Now().UTC(), Name: "live.example.test.", Type: "A",
		Client: "10.0.0.1:5353", Source: "local", Rcode: "NOERROR", DurationMS: 1,
	})
	return (&API{Records: store, DNS: dnsserver.New(store, collector, []string{"127.0.0.1:53"}), Telemetry: collector}).Handler()
}

func getJSON(t *testing.T, handler http.Handler, path string) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	if recorder.Code != http.StatusOK && recorder.Header().Get("Content-Type") == "application/json" {
		return recorder.Code, nil
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: invalid JSON response: %v", path, err)
	}
	return recorder.Code, body
}

func dataSlice(body map[string]any) []map[string]any {
	raw, _ := body["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		out = append(out, entry)
	}
	return out
}

func TestQueriesPaginationWalksAllPages(t *testing.T) {
	handler := sqliteFixture(t, 250)
	cursor := ""
	seen := 0
	pages := 0
	for {
		path := "/api/v1/queries?limit=100"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		code, body := getJSON(t, handler, path)
		if code != http.StatusOK {
			t.Fatalf("queries page failed: %d", code)
		}
		events := dataSlice(body)
		pages++
		if len(events) == 0 {
			break
		}
		seen += len(events)
		next, _ := body["next_cursor"].(float64)
		if seen < 250 {
			if next == 0 {
				t.Fatal("expected next_cursor on a full page")
			}
			cursor = fmt.Sprintf("%d", int64(next))
		}
		if pages > 5 {
			t.Fatal("pagination walk did not terminate")
		}
		if seen >= 250 {
			if _, more := body["next_cursor"]; more {
				t.Fatal("unexpected next_cursor past the last page")
			}
			break
		}
	}
	if pages != 3 || seen != 250 {
		t.Fatalf("expected 3 pages and 250 events, got %d pages and %d events", pages, seen)
	}
}

func TestQueriesFiltersAndTotal(t *testing.T) {
	handler := sqliteFixture(t, 50)
	code, body := getJSON(t, handler, "/api/v1/queries?rcode=NXDOMAIN&count=1")
	if code != http.StatusOK {
		t.Fatalf("filtered query failed: %d", code)
	}
	if total, ok := body["total"].(float64); !ok || total != 5 {
		t.Fatalf("expected 5 NXDOMAIN events, got %v", body["total"])
	}
	if len(dataSlice(body)) != 5 {
		t.Fatalf("unexpected data size: %d", len(dataSlice(body)))
	}
	code, body = getJSON(t, handler, "/api/v1/queries?q=host2.example&count=1")
	if code != http.StatusOK {
		t.Fatalf("search query failed: %d", code)
	}
	if total, ok := body["total"].(float64); !ok || total != 10 {
		t.Fatalf("expected 10 matching events, got %v", body["total"])
	}
	code, _ = getJSON(t, handler, "/api/v1/queries?source=cache&count=1")
	if code != http.StatusOK {
		t.Fatalf("source filter failed: %d", code)
	}
}

func TestQueriesRejectsInvalidParameters(t *testing.T) {
	handler := sqliteFixture(t, 5)
	for _, path := range []string{
		"/api/v1/queries?cursor=abc",
		"/api/v1/queries?cursor=-4",
		"/api/v1/queries?limit=0",
		"/api/v1/queries?limit=501",
		"/api/v1/queries?since=yesterday",
		"/api/v1/queries?sort=sideways",
		"/api/v1/metrics/trend?window=30d",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", path, recorder.Code)
		}
	}
}

func TestQueriesExportStreamsCSV(t *testing.T) {
	handler := sqliteFixture(t, 40)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/queries/export?rcode=NXDOMAIN", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("export failed: %d", recorder.Code)
	}
	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("unexpected content type: %s", recorder.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if lines[0] != "time,name,type,client,source,rcode,duration_ms" {
		t.Fatalf("unexpected CSV header: %q", lines[0])
	}
	if len(lines) != 5 { // header + 4 NXDOMAIN rows
		t.Fatalf("unexpected CSV row count: %d", len(lines)-1)
	}
}

func TestMetricsTrendAndTopsAndResolver(t *testing.T) {
	handler := sqliteFixture(t, 30)
	code, body := getJSON(t, handler, "/api/v1/metrics/trend?window=24h")
	if code != http.StatusOK {
		t.Fatalf("trend failed: %d", code)
	}
	if bucketSeconds, ok := body["bucket_seconds"].(float64); !ok || int(bucketSeconds) != 3600 {
		t.Fatalf("unexpected bucket seconds: %v", body["bucket_seconds"])
	}
	if points, ok := body["data"].([]any); !ok || len(points) != 24 {
		t.Fatalf("unexpected trend shape: %v", body["data"])
	}
	code, body = getJSON(t, handler, "/api/v1/metrics/tops?window=1h")
	if code != http.StatusOK {
		t.Fatalf("tops failed: %d", code)
	}
	tops, ok := body["data"].(map[string]any)
	if !ok || len(tops["domains"].([]any)) == 0 || len(tops["rcodes"].([]any)) == 0 {
		t.Fatalf("unexpected tops shape: %v", body["data"])
	}
	code, body = getJSON(t, handler, "/api/v1/metrics/resolver")
	if code != http.StatusOK {
		t.Fatalf("resolver health failed: %d", code)
	}
	resolver, ok := body["cache"].(map[string]any)
	if !ok || resolver["max"] == nil {
		t.Fatalf("unexpected resolver snapshot: %v", body)
	}
	if upstreams, ok := body["upstreams"].([]any); !ok || len(upstreams) == 0 {
		t.Fatalf("expected upstream entries: %v", body["upstreams"])
	}
}

func TestPrometheusMetricsEndpoint(t *testing.T) {
	handler := sqliteFixture(t, 20)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics failed: %d", recorder.Code)
	}
	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("unexpected content type: %s", recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		"quilldns_queries_total{source=",
		"quilldns_responses_total{rcode=\"NOERROR\"}",
		"quilldns_query_duration_seconds_bucket{le=",
		"quilldns_query_duration_seconds_sum",
		"quilldns_cache_hits_total",
		`quilldns_upstream_requests_total{result="ok",upstream=`,
		"# TYPE quilldns_upstream_latency_seconds_sum counter",
		"quilldns_log_events_dropped_total",
		"go_goroutines",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics output missing %q", expected)
		}
	}
}
