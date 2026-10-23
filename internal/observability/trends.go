package observability

import (
	"context"
	"fmt"
	"time"
)

// TrendPoint is one fixed-width bucket of observed query volume and latency.
type TrendPoint struct {
	Time             time.Time `json:"time"`
	Queries          int64     `json:"queries"`
	AverageLatencyMS float64   `json:"average_latency_ms"`
	Local            int64     `json:"local"`
	Cache            int64     `json:"cache"`
	Upstream         int64     `json:"upstream"`
}

// trendWindow describes one selectable chart span.
type trendWindow struct {
	buckets int
	width   time.Duration
}

// trendWindows are the windows the API accepts; wider windows widen buckets
// so aggregation cost stays flat as history grows.
var trendWindows = map[string]trendWindow{
	"1h":  {buckets: 12, width: 5 * time.Minute},
	"24h": {buckets: 24, width: time.Hour},
	"7d":  {buckets: 28, width: 6 * time.Hour},
}

// TrendWindows lists the supported trend window names in stable order.
func TrendWindows() []string {
	return []string{"1h", "24h", "7d"}
}

// TrendBucketSeconds returns the bucket width of a trend window in seconds.
func TrendBucketSeconds(window string) int {
	if span, ok := trendWindows[window]; ok {
		return int(span.width.Seconds())
	}
	return int(trendWindows["1h"].width.Seconds())
}

// TrendSpan converts a window name to its duration; ok is false when unknown.
func TrendSpan(window string) (time.Duration, bool) {
	span, ok := trendWindows[window]
	if !ok {
		return 0, false
	}
	return time.Duration(span.buckets) * span.width, true
}

const trendCacheTTL = 10 * time.Minute

type cachedTrend struct {
	immutable []TrendPoint
	end       time.Time
	expires   time.Time
}

// Trend returns a window of fixed-width buckets, including empty intervals.
// Completed buckets are immutable, so they are cached per bucket boundary for
// the dashboard's polling; the newest, still-open bucket is always computed
// live so fresh traffic appears within one flush interval.
func (c *Collector) Trend(ctx context.Context, now time.Time, window string) ([]TrendPoint, error) {
	span, ok := trendWindows[window]
	if !ok {
		return nil, fmt.Errorf("unknown trend window %q", window)
	}
	return c.computeTrend(ctx, now, window, span)
}

func (c *Collector) computeTrend(ctx context.Context, now time.Time, window string, span trendWindow) ([]TrendPoint, error) {
	width := span.width
	end := now.UTC().Truncate(width)
	start := end.Add(time.Duration(span.buckets-1) * -width)
	points := make([]TrendPoint, span.buckets)
	for i := range points {
		points[i].Time = start.Add(time.Duration(i) * width)
	}
	if c.dbRead == nil {
		return fillTrendFromRing(c, start, width, points), nil
	}

	var immutable []TrendPoint
	c.trendMu.Lock()
	cached, fresh := c.trendCache[window]
	c.trendMu.Unlock()
	if fresh && cached.end.Equal(end) && now.Before(cached.expires) {
		immutable = cached.immutable
	} else {
		immutable = make([]TrendPoint, span.buckets)
		for i := range immutable {
			immutable[i].Time = points[i].Time
		}
		if err := c.trendRange(ctx, start, start, end, width, immutable); err != nil {
			return nil, err
		}
		c.trendMu.Lock()
		if c.trendCache == nil {
			c.trendCache = make(map[string]cachedTrend)
		}
		c.trendCache[window] = cachedTrend{immutable: immutable, end: end, expires: now.Add(trendCacheTTL)}
		c.trendMu.Unlock()
	}
	copy(points, immutable)

	if end.Before(now) {
		if err := c.trendRange(ctx, start, end, end.Add(width), width, points); err != nil {
			return nil, err
		}
	}
	return points, nil
}

// trendRange aggregates stored events in [from, to) into the bucket grid whose
// first bucket starts at gridStart.
func (c *Collector) trendRange(ctx context.Context, gridStart, from, to time.Time, width time.Duration, out []TrendPoint) error {
	rows, err := c.dbRead.QueryContext(ctx, fmt.Sprintf(`
SELECT epoch / %d AS bucket,
COUNT(*), AVG(duration_ms),
SUM(CASE WHEN source='local' THEN 1 ELSE 0 END),
SUM(CASE WHEN source='cache' THEN 1 ELSE 0 END),
SUM(CASE WHEN source='upstream' THEN 1 ELSE 0 END)
FROM queries
WHERE epoch >= ? AND epoch < ?
GROUP BY bucket ORDER BY bucket`, int(width.Seconds())),
		from.Unix(), to.Unix())
	if err != nil {
		return err
	}
	defer rows.Close()
	firstBucket := gridStart.Unix() / int64(width.Seconds())
	for rows.Next() {
		var bucket int64
		var p TrendPoint
		if err := rows.Scan(&bucket, &p.Queries, &p.AverageLatencyMS, &p.Local, &p.Cache, &p.Upstream); err != nil {
			return err
		}
		index := bucket - firstBucket
		if index >= 0 && index < int64(len(out)) {
			p.Time = out[index].Time
			out[index] = p
		}
	}
	return rows.Err()
}

// fillTrendFromRing buckets the in-memory ring when no history database is open.
func fillTrendFromRing(c *Collector, start time.Time, width time.Duration, points []TrendPoint) []TrendPoint {
	sums := make([]float64, len(points))
	for _, e := range c.Events(c.max) {
		index := int(e.Time.UTC().Sub(start) / width)
		if e.Time.Before(start) || index < 0 || index >= len(points) {
			continue
		}
		p := &points[index]
		p.Queries++
		sums[index] += e.DurationMS
		switch e.Source {
		case "local":
			p.Local++
		case "cache":
			p.Cache++
		case "upstream":
			p.Upstream++
		}
	}
	for i := range points {
		if points[i].Queries > 0 {
			points[i].AverageLatencyMS = sums[i] / float64(points[i].Queries)
		}
	}
	return points
}
