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
}
