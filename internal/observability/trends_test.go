package observability

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestLatencySummaryRollingPercentilesAndSourceAverages(t *testing.T) {
	c := New(100)
	for i := 1; i <= 100; i++ {
		source := "local"
		if i > 50 {
			source = "upstream"
		}
		c.Record(Event{Source: source, Rcode: "NOERROR", DurationMS: float64(i)})
	}
	s := c.Summary()
	if s.Queries != 100 || s.P95LatencyMS != 95 || s.P99LatencyMS != 99 {
		t.Fatalf("unexpected percentiles: %+v", s)
	}
	if math.Abs(s.LocalAvgLatencyMS-25.5) > 0.01 || math.Abs(s.UpstreamAvgLatencyMS-75.5) > 0.01 {
		t.Fatalf("unexpected source averages: %+v", s)
	}
}

func TestTrendSQLiteFiveMinuteBuckets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.sqlite")
	c, err := Open(path, 32, 7, 100000)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(5 * time.Minute)
	c.Record(Event{Time: base.Add(3 * time.Second), Name: "one.test.", Source: "local", Rcode: "NOERROR", DurationMS: 2})
	c.Record(Event{Time: base.Add(7 * time.Second), Name: "two.test.", Source: "upstream", Rcode: "NOERROR", DurationMS: 10})
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, 32, 7, 100000)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(context.Background())
	points, err := reopened.Trend(context.Background(), base.Add(time.Minute), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 12 || points[11].Queries != 2 || points[11].Local != 1 || points[11].Upstream != 1 || math.Abs(points[11].AverageLatencyMS-6) > 0.01 {
		t.Fatalf("unexpected stored trend: %+v", points)
	}
	if points[11].Time.IsZero() {
		t.Fatal("live bucket lost its timestamp")
	}
}

// pollTrend returns the first trend within the deadline whose query total is
// want, mirroring how the dashboard and the browser e2e wait for freshly
// flushed history to appear.
func pollTrend(t *testing.T, c *Collector, now time.Time, want int64) []TrendPoint {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		points, err := c.Trend(context.Background(), time.Now().UTC(), "1h")
		if err != nil {
			t.Fatal(err)
		}
		if sumQueries(points) >= want {
			return points
		}
		if time.Now().After(deadline) {
			t.Fatalf("trend never reached %d queries; last: %+v", want, points)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Trend must surface freshly flushed events immediately: dashboards poll the
// trend right after traffic starts, so only completed buckets may be cached.
func TestTrendIncludesFreshEventsWithinCache(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "queries.sqlite"), 64, 7, 100000)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	now := time.Now().UTC()

	c.Record(Event{Time: now, Name: "a.test.", Source: "local", Rcode: "NOERROR", DurationMS: 1})
	if first := pollTrend(t, c, now, 1); first[len(first)-1].Local != 1 {
		t.Fatalf("unexpected source split after first flush: %+v", first[len(first)-1])
	}

	c.Record(Event{Time: time.Now().UTC(), Name: "b.test.", Source: "upstream", Rcode: "NOERROR", DurationMS: 2})
	c.Record(Event{Time: time.Now().UTC(), Name: "c.test.", Source: "cache", Rcode: "NOERROR", DurationMS: 3})
	second := pollTrend(t, c, now.Add(time.Second), 3)
	last := second[len(second)-1]
	if last.Upstream != 1 || last.Cache != 1 || last.Local != 1 {
		t.Fatalf("unexpected source split after second flush: %+v", last)
	}
	if last.Time.IsZero() {
		t.Fatal("live bucket lost its timestamp")
	}
}

func sumQueries(points []TrendPoint) int64 {
	var sum int64
	for _, p := range points {
		sum += p.Queries
	}
	return sum
}

func TestSummarySplitsHardFailuresFromNXDomain(t *testing.T) {
	c := New(16)
	for _, rcode := range []string{"NOERROR", "NXDOMAIN", "NXDOMAIN", "SERVFAIL", "REFUSED", "NOERROR"} {
		c.Record(Event{Source: "upstream", Rcode: rcode, DurationMS: 1})
	}
	s := c.Summary()
	if s.Errors != 4 || s.NXDomain != 2 || s.Failures != 2 {
		t.Fatalf("unexpected error split: %+v", s)
	}
}
