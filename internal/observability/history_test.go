package observability

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// persistFlushes records events and closes so the worker flushes them to
// SQLite, then reopens the collector for API-path reads.
func persistFlushes(t *testing.T, path string, events []Event) *Collector {
	t.Helper()
	c, err := Open(path, 64, 7, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		c.Record(e)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, 64, 7, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close(context.Background()) })
	return reopened
}

func seedEvents(count int, base time.Time) []Event {
	events := make([]Event, 0, count)
	for i := range count {
		source := "local"
		if i%3 == 1 {
			source = "cache"
		} else if i%3 == 2 {
			source = "upstream"
		}
		rcode := "NOERROR"
		if i%10 == 5 {
			rcode = "NXDOMAIN"
		}
		events = append(events, Event{
			Time:       base.Add(-time.Duration(i) * time.Second),
			Name:       "host" + itoa(i%7) + ".example.test.",
			Type:       "A",
			Client:     "10.0.0." + itoa(i%4) + ":5353",
			Source:     source,
			Rcode:      rcode,
			DurationMS: float64(i%5) + 0.5,
		})
	}
	return events
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func TestSearchPaginatesNewestFirstWithoutOverlap(t *testing.T) {
	base := time.Now().UTC()
	reopened := persistFlushes(t, filepath.Join(t.TempDir(), "queries.sqlite"), seedEvents(250, base))
	ctx := context.Background()
	filter := QueryFilter{Limit: 100}
	seen := make(map[int64]bool)
	pages := 0
	for {
		events, hasMore, err := reopened.Search(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(events) == 0 {
			break
		}
		previous := int64(0)
		for i, e := range events {
			if i > 0 && e.ID >= previous {
				t.Fatalf("page not in descending id order: %d after %d", e.ID, previous)
			}
			previous = e.ID
			if seen[e.ID] {
				t.Fatalf("duplicate event id %d across pages", e.ID)
			}
			seen[e.ID] = true
		}
		if !hasMore {
			break
		}
		filter.Cursor = events[len(events)-1].ID
		if pages > 10 {
			t.Fatal("pagination walk did not terminate")
		}
	}
	if pages != 3 {
		t.Fatalf("expected 3 pages of 250 events at limit 100, got %d", pages)
	}
	if len(seen) != 250 {
		t.Fatalf("expected 250 unique events, got %d", len(seen))
	}
}

func TestSearchFiltersCombine(t *testing.T) {
	base := time.Now().UTC()
	reopened := persistFlushes(t, filepath.Join(t.TempDir(), "queries.sqlite"), seedEvents(30, base))
	ctx := context.Background()
	cases := []struct {
		name   string
		filter QueryFilter
		verify func([]Event) bool
	}{
		{"source", QueryFilter{Source: "cache", Limit: 500}, func(events []Event) bool {
			return len(events) == 10 && slices.ContainsFunc(events, func(e Event) bool { return e.Source != "cache" }) == false
		}},
		{"rcode", QueryFilter{Rcode: "NXDOMAIN", Limit: 500}, func(events []Event) bool {
			return len(events) == 3
		}},
		{"name substring", QueryFilter{Name: "3.example", Limit: 500}, func(events []Event) bool {
			return len(events) == 4
		}},
		{"q matches client", QueryFilter{Q: "10.0.0.2", Limit: 500}, func(events []Event) bool {
			return len(events) == 7
		}},
		{"time window", QueryFilter{Since: base.Add(-11 * time.Second), Until: base.Add(-4 * time.Second), Limit: 500}, func(events []Event) bool {
			return len(events) == 7
		}},
		{"combined", QueryFilter{Source: "upstream", Rcode: "NXDOMAIN", Limit: 500}, func(events []Event) bool {
			return len(events) == 1
		}},
	}
	for _, testCase := range cases {
		events, hasMore, err := reopened.Search(ctx, testCase.filter)
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if hasMore {
			t.Fatalf("%s: unexpected hasMore", testCase.name)
		}
		if !testCase.verify(events) {
			t.Fatalf("%s: unexpected result (%d events)", testCase.name, len(events))
		}
	}
}

func TestSearchSortsAscendingAndCounts(t *testing.T) {
	base := time.Now().UTC()
	reopened := persistFlushes(t, filepath.Join(t.TempDir(), "queries.sqlite"), seedEvents(30, base))
	ctx := context.Background()
	events, _, err := reopened.Search(ctx, QueryFilter{Sort: "asc", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 10 || events[0].ID > events[1].ID {
		t.Fatalf("ascending sort broken: %d then %d", events[0].ID, events[1].ID)
	}
	total, err := reopened.Count(ctx, QueryFilter{Source: "local"})
	if err != nil || total != 10 {
		t.Fatalf("count mismatch: %d %v", total, err)
	}
}

func TestTopAggregationsRankAndBound(t *testing.T) {
	base := time.Now().UTC()
	reopened := persistFlushes(t, filepath.Join(t.TempDir(), "queries.sqlite"), seedEvents(30, base))
	tops, err := reopened.Tops(context.Background(), base.Add(-time.Hour), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tops.Domains) == 0 || len(tops.Domains) > 5 {
		t.Fatalf("unexpected domain ranking size: %d", len(tops.Domains))
	}
	for i := 1; i < len(tops.Domains); i++ {
		if tops.Domains[i].Count > tops.Domains[i-1].Count {
			t.Fatal("tops not ranked by descending count")
		}
	}
	var nxCount int64
	for _, entry := range tops.Rcodes {
		if entry.Value == "NXDOMAIN" {
			nxCount = entry.Count
		}
	}
	if nxCount != 3 {
		t.Fatalf("expected 3 NXDOMAIN responses, got %d", nxCount)
	}
}

func TestSearchAndTopsFallBackToRing(t *testing.T) {
	c := New(20)
	base := time.Now().UTC()
	for _, e := range seedEvents(6, base) {
		c.Record(e)
	}
	events, hasMore, err := c.Search(context.Background(), QueryFilter{Source: "cache", Limit: 5})
	if err != nil || len(events) != 2 || hasMore {
		t.Fatalf("ring search mismatch: %d %v %v", len(events), hasMore, err)
	}
	total, err := c.Count(context.Background(), QueryFilter{Source: "cache"})
	if err != nil || total != 2 {
		t.Fatalf("ring count mismatch: %d %v", total, err)
	}
	tops, err := c.Tops(context.Background(), base.Add(-time.Hour), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tops.Types) == 0 || len(tops.Types) > 3 {
		t.Fatalf("ring tops mismatch: %d", len(tops.Types))
	}
}

func TestLatencyHistogramAndRcodeCounters(t *testing.T) {
	c := New(4)
	for i := range 10 {
		c.Record(Event{Rcode: "NOERROR", DurationMS: 1.5, Source: "local"})
		_ = i
	}
	c.Record(Event{Rcode: "NXDOMAIN", DurationMS: 30, Source: "upstream"})
	c.Record(Event{Rcode: "SERVFAIL", DurationMS: 2500, Source: "error"})
	bounds, counts, sumSeconds := c.LatencyHistogram()
	if len(bounds) != 14 || len(counts) != 15 {
		t.Fatalf("unexpected histogram shape: %d bounds, %d counts", len(bounds), len(counts))
	}
	if counts[len(counts)-1] != 12 {
		t.Fatalf("expected 12 total observations, got %d", counts[len(counts)-1])
	}
	// Ten 1.5ms samples land under the 2ms bound (index 2); the 30ms sample
	// lifts the 50ms bound (index 6) to 11; the 2.5s sample tops the 5s bound.
	if counts[2] != 10 || counts[6] != 11 || counts[13] != 12 {
		t.Fatalf("unexpected cumulative buckets: %v", counts)
	}
	if sumSeconds < 2.54 || sumSeconds > 2.55 {
		t.Fatalf("unexpected sum: %v", sumSeconds)
	}
	rcodes := c.RcodeCounts()
	if rcodes["NOERROR"] != 10 || rcodes["NXDOMAIN"] != 1 || rcodes["SERVFAIL"] != 1 {
		t.Fatalf("unexpected rcode counters: %v", rcodes)
	}
}

func TestTrendWindowsShapeBuckets(t *testing.T) {
	c := New(64)
	base := time.Now().UTC()
	for i := range 5 {
		c.Record(Event{Time: base.Add(-time.Duration(i) * time.Minute), Source: "local", Rcode: "NOERROR"})
	}
	ctx := context.Background()
	for window, want := range map[string]int{"1h": 12, "24h": 24, "7d": 28} {
		points, err := c.Trend(ctx, base, window)
		if err != nil {
			t.Fatalf("%s: %v", window, err)
		}
		if len(points) != want {
			t.Fatalf("%s: expected %d buckets, got %d", window, want, len(points))
		}
		var total int64
		for _, p := range points {
			total += p.Queries
		}
		if total != 5 {
			t.Fatalf("%s: expected 5 recorded queries in window, got %d", window, total)
		}
	}
	if _, err := c.Trend(ctx, base, "30d"); err == nil {
		t.Fatal("unknown trend window accepted")
	}
	if TrendBucketSeconds("24h") != 3600 || TrendBucketSeconds("1h") != 300 {
		t.Fatal("unexpected bucket widths")
	}
}
