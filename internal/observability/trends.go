package observability

import (
	"context"
	"time"
)

// TrendPoint is a five-minute bucket of observed DNS query volume and processing latency.
type TrendPoint struct {
	Time time.Time `json:"time"`
	Queries int64 `json:"queries"`
	AverageLatencyMS float64 `json:"average_latency_ms"`
	Local int64 `json:"local"`
	Cache int64 `json:"cache"`
	Upstream int64 `json:"upstream"`
}

// Trend returns the last hour in twelve fixed five-minute buckets, including empty intervals.
// It reports stored query history when available; the in-memory fallback is limited to the ring.
func (c *Collector) Trend(ctx context.Context, now time.Time) ([]TrendPoint, error) {
	const width = 5 * time.Minute
	end := now.UTC().Truncate(width)
	start := end.Add(-11 * width)
	points := make([]TrendPoint, 12)
	for i := range points {
		points[i].Time = start.Add(time.Duration(i) * width)
	}
	if c.db == nil {
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
		return points, nil
	}
	rows, err := c.db.QueryContext(ctx, `
SELECT CAST(strftime('%s', time) AS INTEGER) / 300 AS bucket,
COUNT(*), AVG(duration_ms),
SUM(CASE WHEN source='local' THEN 1 ELSE 0 END),
SUM(CASE WHEN source='cache' THEN 1 ELSE 0 END),
SUM(CASE WHEN source='upstream' THEN 1 ELSE 0 END)
FROM queries
WHERE time >= ? AND time < ?
GROUP BY bucket ORDER BY bucket`,
		start.Format(time.RFC3339Nano), end.Add(width).Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	firstBucket := start.Unix() / 300
	for rows.Next() {
		var bucket int64
		var p TrendPoint
		if err := rows.Scan(&bucket, &p.Queries, &p.AverageLatencyMS, &p.Local, &p.Cache, &p.Upstream); err != nil {
			return nil, err
		}
		index := bucket - firstBucket
		if index >= 0 && index < int64(len(points)) {
			p.Time = points[index].Time
			points[index] = p
		}
	}
	return points, rows.Err()
}
