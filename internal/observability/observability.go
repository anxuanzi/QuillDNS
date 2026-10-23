// Package observability provides integrated bounded analytics and persistent query history.
package observability

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	ID         int64     `json:"id,omitempty"`
	Time       time.Time `json:"time"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Client     string    `json:"client"`
	Source     string    `json:"source"`
	Rcode      string    `json:"rcode"`
	DurationMS float64   `json:"duration_ms"`
}
type Summary struct {
	Queries              uint64  `json:"queries"`
	Local                uint64  `json:"local"`
	Cache                uint64  `json:"cache"`
	Upstream             uint64  `json:"upstream"`
	Errors               uint64  `json:"errors"`
	Dropped              uint64  `json:"dropped_logs"`
	AverageLatencyMS     float64 `json:"average_latency_ms"`
	P95LatencyMS         float64 `json:"p95_latency_ms"`
	P99LatencyMS         float64 `json:"p99_latency_ms"`
	LocalAvgLatencyMS    float64 `json:"local_avg_latency_ms"`
	CacheAvgLatencyMS    float64 `json:"cache_avg_latency_ms"`
	UpstreamAvgLatencyMS float64 `json:"upstream_avg_latency_ms"`
}
type Collector struct {
	mu              sync.RWMutex
	events          []Event
	max             int
	next            int
	full            bool
	summary         Summary
	totalLatency    float64
	localLatency    float64
	cacheLatency    float64
	upstreamLatency float64
	db              *sql.DB
	dbRead          *sql.DB
	retention       time.Duration
	maxRows         int
	queue           chan Event
	done            chan struct{}
	once            sync.Once
	dropped         atomic.Uint64
	histogram       [len(latencyBucketsSec) + 1]atomic.Uint64
	latencySumUS    atomic.Int64
	rcodeCounts     sync.Map
	countMu         sync.Mutex
	countCache      map[string]countEntry
	trendMu         sync.Mutex
	trendCache      map[string]cachedTrend
	topsMu          sync.Mutex
	topsCache       map[string]cachedTops
}

func New(max int) *Collector {
	if max < 1 {
		max = 1000
	}
	return &Collector{events: make([]Event, max), max: max}
}

// Open creates a bounded in-memory collector plus a separate SQLite query-history database.
// retentionDays and maxRows bound the persisted history so logging can never
// grow without limit; both come from the operator's config.
func Open(path string, ringSize, retentionDays, maxRows int) (*Collector, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=5000",
		"CREATE TABLE IF NOT EXISTS queries(id INTEGER PRIMARY KEY AUTOINCREMENT, time TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL, client TEXT NOT NULL, source TEXT NOT NULL, rcode TEXT NOT NULL, duration_ms REAL NOT NULL)",
		"CREATE INDEX IF NOT EXISTS queries_time ON queries(time DESC)",
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize query history: %w", err)
		}
	}
	// Older databases lack the epoch column analytics aggregate on; add and
	// backfill it once, before any reader can touch the table.
	var epochColumns int
	if err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('queries') WHERE name='epoch'").Scan(&epochColumns); err != nil {
		db.Close()
		return nil, err
	}
	if epochColumns == 0 {
		if _, err = db.Exec("ALTER TABLE queries ADD COLUMN epoch INTEGER"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err = db.Exec("UPDATE queries SET epoch = CAST(strftime('%s', time) AS INTEGER) WHERE epoch IS NULL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill query epochs: %w", err)
	}
	read, err := openReadPool(path)
	if err != nil {
		db.Close()
		return nil, err
	}
	c := New(ringSize)
	c.db = db
	c.dbRead = read
	c.retention = time.Duration(max(retentionDays, 1)) * 24 * time.Hour
	c.maxRows = max(maxRows, 1)
	c.queue = make(chan Event, 8192)
	c.done = make(chan struct{})
	go c.worker()
	return c, nil
}

// openReadPool returns a dedicated connection pool for management reads.
// In WAL mode readers run alongside the single log writer, so heavy analytic
// queries never delay query-log flushes on the write connection.
func openReadPool(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(absolute),
		RawQuery: url.Values{"_pragma": {"busy_timeout(5000)"}}.Encode(),
	}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	return db, nil
}
func (c *Collector) Record(e Event) {
	c.mu.Lock()
	c.events[c.next] = e
	c.next = (c.next + 1) % c.max
	if c.next == 0 {
		c.full = true
	}
	c.summary.Queries++
	switch e.Source {
	case "local":
		c.summary.Local++
		c.localLatency += e.DurationMS
	case "cache":
		c.summary.Cache++
		c.cacheLatency += e.DurationMS
	case "upstream":
		c.summary.Upstream++
		c.upstreamLatency += e.DurationMS
	}
	if e.Rcode != "NOERROR" {
		c.summary.Errors++
	}
	c.totalLatency += e.DurationMS
	c.summary.AverageLatencyMS = c.totalLatency / float64(c.summary.Queries)
	c.mu.Unlock()
	// Lifetime latency and rcode telemetry runs lock-free so the DNS hot path
	// never contends on the ring-buffer mutex.
	c.recordLatency(e.DurationMS)
	if counter, _ := c.rcodeCounts.LoadOrStore(e.Rcode, &atomic.Uint64{}); counter != nil {
		counter.(*atomic.Uint64).Add(1)
	}
	if c.queue != nil {
		select {
		case c.queue <- e:
		default:
			c.dropped.Add(1)
		}
	}
}

// latencyBucketsSec holds the finite histogram bounds in seconds; the final
// +Inf bucket is implicit. Bounds match common DNS behavior: sub-millisecond
// local and cache answers through multi-second upstream timeouts.
var latencyBucketsSec = [...]float64{0.0005, 0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10}

func (c *Collector) recordLatency(durationMS float64) {
	seconds := durationMS / 1000
	// The sum is tracked as integer microseconds: an atomic float type is
	// unavailable and per-event truncation at microsecond scale is negligible.
	c.latencySumUS.Add(int64(durationMS * 1000))
	c.histogram[sort.SearchFloat64s(latencyBucketsSec[:], seconds)].Add(1)
}

// RcodeCounts returns lifetime response-code counters keyed by rcode name.
func (c *Collector) RcodeCounts() map[string]uint64 {
	out := make(map[string]uint64)
	c.rcodeCounts.Range(func(key, value any) bool {
		out[key.(string)] = value.(*atomic.Uint64).Load()
		return true
	})
	return out
}

// LatencyHistogram returns cumulative per-bound counts using Prometheus le
// semantics, plus the total sum and count in seconds.
func (c *Collector) LatencyHistogram() (bounds []float64, counts []uint64, sumSeconds float64) {
	counts = make([]uint64, len(c.histogram))
	var cumulative uint64
	for i := range c.histogram {
		cumulative += c.histogram[i].Load()
		counts[i] = cumulative
	}
	return latencyBucketsSec[:], counts, float64(c.latencySumUS.Load()) / 1e6
}
func (c *Collector) Summary() Summary {
	c.mu.RLock()
	out := c.summary
	if out.Local > 0 {
		out.LocalAvgLatencyMS = c.localLatency / float64(out.Local)
	}
	if out.Cache > 0 {
		out.CacheAvgLatencyMS = c.cacheLatency / float64(out.Cache)
	}
	if out.Upstream > 0 {
		out.UpstreamAvgLatencyMS = c.upstreamLatency / float64(out.Upstream)
	}
	// Percentiles reflect only the bounded in-memory window, not lifetime history.
	n := c.next
	if c.full {
		n = c.max
	}
	latencies := make([]float64, n)
	for i := 0; i < n; i++ {
		latencies[i] = c.events[i].DurationMS
	}
	c.mu.RUnlock()
	if n > 0 {
		sort.Float64s(latencies)
		out.P95LatencyMS = latencies[(95*n+99)/100-1]
		out.P99LatencyMS = latencies[(99*n+99)/100-1]
	}
	out.Dropped = c.dropped.Load()
	return out
}
func (c *Collector) Events(limit int) []Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	n := c.next
	if c.full {
		n = c.max
	}
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Event, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (c.next - 1 - i + c.max) % c.max
		out = append(out, c.events[idx])
	}
	return out
}
func (c *Collector) Recent(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if c.db == nil {
		return c.Events(limit), nil
	}
	rows, err := c.db.QueryContext(ctx, "SELECT time,name,type,client,source,rcode,duration_ms FROM queries ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		var ts string
		if err = rows.Scan(&ts, &e.Name, &e.Type, &e.Client, &e.Source, &e.Rcode, &e.DurationMS); err != nil {
			return nil, err
		}
		e.Time, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (c *Collector) worker() {
	defer close(c.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	batch := make([]Event, 0, 256)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		tx, err := c.db.Begin()
		if err != nil {
			c.dropped.Add(uint64(len(batch)))
			batch = batch[:0]
			return
		}
		stmt, err := tx.Prepare("INSERT INTO queries(time,epoch,name,type,client,source,rcode,duration_ms) VALUES(?,?,?,?,?,?,?,?)")
		if err == nil {
			for _, e := range batch {
				start := e.Time.UTC()
				if _, err = stmt.Exec(start.Format(time.RFC3339Nano), start.Unix(), e.Name, e.Type, e.Client, e.Source, e.Rcode, e.DurationMS); err != nil {
					break
				}
			}
			stmt.Close()
		}
		if err != nil {
			_ = tx.Rollback()
			c.dropped.Add(uint64(len(batch)))
		} else if err = tx.Commit(); err != nil {
			c.dropped.Add(uint64(len(batch)))
		}
		batch = batch[:0]
	}
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case e, ok := <-c.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 512 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-prune.C:
			// Configurable time retention plus an absolute row budget.
			_, _ = c.db.Exec("DELETE FROM queries WHERE time < ?", time.Now().UTC().Add(-c.retention).Format(time.RFC3339Nano))
			_, _ = c.db.Exec("DELETE FROM queries WHERE id NOT IN (SELECT id FROM queries ORDER BY id DESC LIMIT ?)", c.maxRows)
		}
	}
}

func (c *Collector) Close(ctx context.Context) error {
	if c.db == nil {
		return nil
	}
	c.once.Do(func() { close(c.queue) })
	select {
	case <-c.done:
		err := c.db.Close()
		if c.dbRead != nil {
			err = errors.Join(err, c.dbRead.Close())
		}
		return err
	case <-ctx.Done():
		return errors.Join(ctx.Err(), fmt.Errorf("query log flush incomplete"))
	}
}
