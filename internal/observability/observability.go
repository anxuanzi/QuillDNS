// Package observability provides integrated bounded analytics and persistent query history.
package observability

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
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
	queue           chan Event
	done            chan struct{}
	once            sync.Once
	dropped         atomic.Uint64
}

func New(max int) *Collector {
	if max < 1 {
		max = 1000
	}
	return &Collector{events: make([]Event, max), max: max}
}

// Open creates a bounded in-memory collector plus a separate SQLite query-history database.
func Open(path string, max int) (*Collector, error) {
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
	c := New(max)
	c.db = db
	c.queue = make(chan Event, 4096)
	c.done = make(chan struct{})
	go c.worker()
	return c, nil
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
	if c.queue != nil {
		select {
		case c.queue <- e:
		default:
			c.dropped.Add(1)
		}
	}
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
		stmt, err := tx.Prepare("INSERT INTO queries(time,name,type,client,source,rcode,duration_ms) VALUES(?,?,?,?,?,?,?)")
		if err == nil {
			for _, e := range batch {
				if _, err = stmt.Exec(e.Time.UTC().Format(time.RFC3339Nano), e.Name, e.Type, e.Client, e.Source, e.Rcode, e.DurationMS); err != nil {
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
			if len(batch) >= 256 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-prune.C:
			// Seven-day retention and a fixed maximum row budget.
			_, _ = c.db.Exec("DELETE FROM queries WHERE time < ?", time.Now().UTC().Add(-7*24*time.Hour).Format(time.RFC3339Nano))
			_, _ = c.db.Exec("DELETE FROM queries WHERE id NOT IN (SELECT id FROM queries ORDER BY id DESC LIMIT 100000)")
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
		return c.db.Close()
	case <-ctx.Done():
		return errors.Join(ctx.Err(), fmt.Errorf("query log flush incomplete"))
	}
}
