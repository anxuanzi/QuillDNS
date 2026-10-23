package observability

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// QueryFilter selects one page of persisted query history. Every field is a
// matchers value bound as a SQL parameter; nothing is interpolated.
type QueryFilter struct {
	Q      string // substring matched against the queried name or client address
	Name   string // substring matched against the queried name
	Client string // prefix matched against the client address
	Source string
	Rcode  string
	Type   string
	Since  time.Time
	Until  time.Time
	Cursor int64  // exclusive id bound: pages continue strictly past this row
	Sort   string // "asc" walks oldest first; anything else is newest first
	Limit  int
}

// MaxPageSize bounds the page size accepted from callers.
const MaxPageSize = 500

// exportRowCap bounds CSV exports; downloads beyond this need narrower filters.
const exportRowCap = 50_000

// order returns the SQL sort direction for keyset pagination.
func (f QueryFilter) order() string {
	if f.Sort == "asc" {
		return "ASC"
	}
	return "DESC"
}

// likePattern escapes LIKE wildcards and wraps the value for substring search.
func likePattern(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('%')
	for _, r := range value {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// match applies the filter to one event; it backs the in-memory fallback and
// mirrors the SQL conditions below.
func (f QueryFilter) match(e Event) bool {
	if f.Q != "" {
		needle := strings.TrimSuffix(f.Q, ".")
		if !containsFold(e.Name, needle) && !containsFold(e.Client, needle) {
			return false
		}
	}
	if f.Name != "" && !containsFold(e.Name, strings.TrimSuffix(f.Name, ".")) {
		return false
	}
	if f.Client != "" && !strings.HasPrefix(e.Client, f.Client) {
		return false
	}
	if f.Source != "" && e.Source != f.Source {
		return false
	}
	if f.Rcode != "" && e.Rcode != f.Rcode {
		return false
	}
	if f.Type != "" && e.Type != f.Type {
		return false
	}
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && !e.Time.Before(f.Until) {
		return false
	}
	return true
}

// conditions builds the parameterized WHERE fragment shared by Search and
// Count, without the pagination cursor so Count totals a whole filter.
func (f QueryFilter) conditions() (string, []any) {
	var conds []string
	var args []any
	if f.Q != "" {
		pattern := likePattern(strings.TrimSuffix(f.Q, "."))
		conds = append(conds, "(name LIKE ? ESCAPE '\\' OR client LIKE ? ESCAPE '\\')")
		args = append(args, pattern, pattern)
	}
	if f.Name != "" {
		conds = append(conds, "name LIKE ? ESCAPE '\\'")
		args = append(args, likePattern(strings.TrimSuffix(f.Name, ".")))
	}
	if f.Client != "" {
		conds = append(conds, "client LIKE ? ESCAPE '\\'")
		args = append(args, likePattern(f.Client))
	}
	if f.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, f.Source)
	}
	if f.Rcode != "" {
		conds = append(conds, "rcode = ?")
		args = append(args, f.Rcode)
	}
	if f.Type != "" {
		conds = append(conds, "type = ?")
		args = append(args, f.Type)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "time >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "time < ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// where adds the exclusive cursor bound on top of conditions().
func (f QueryFilter) where() (string, []any) {
	where, args := f.conditions()
	if f.Cursor > 0 {
		comparison := "<"
		if f.Sort == "asc" {
			comparison = ">"
		}
		if where == "" {
			where = " WHERE id " + comparison + " ?"
		} else {
			where += " AND id " + comparison + " ?"
		}
		args = append(args, f.Cursor)
	}
	return where, args
}

// cacheKey serializes the filter for the short-lived count cache.
func (f QueryFilter) cacheKey() string {
	var b strings.Builder
	for _, part := range []string{f.Q, f.Name, f.Client, f.Source, f.Rcode, f.Type, f.Sort} {
		b.WriteString(part)
		b.WriteByte(0)
	}
	b.WriteString(f.Since.UTC().Format(time.RFC3339Nano))
	b.WriteByte(0)
	b.WriteString(f.Until.UTC().Format(time.RFC3339Nano))
	return b.String()
}

const countCacheTTL = 2 * time.Second

type countEntry struct {
	count   int64
	expires time.Time
}

// Search returns one page of query history, newest first unless Sort is
// "asc", with hasMore reporting whether another page follows.
func (c *Collector) Search(ctx context.Context, filter QueryFilter) ([]Event, bool, error) {
	if filter.Limit <= 0 || filter.Limit > MaxPageSize {
		filter.Limit = 100
	}
	if c.dbRead == nil {
		return c.searchRing(filter), false, nil
	}
	where, args := filter.where()
	rows, err := c.dbRead.QueryContext(ctx,
		"SELECT id,time,name,type,client,source,rcode,duration_ms FROM queries"+where+
			" ORDER BY id "+filter.order()+" LIMIT ?",
		append(args, filter.Limit+1)...)
	if err != nil {
		return nil, false, err
	}
	events, err := scanEvents(rows)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(events) > filter.Limit
	if hasMore {
		events = events[:filter.Limit]
	}
	return events, hasMore, nil
}

// Export returns the full filtered history in chronological order for CSV
// download, hard-bounded by maxRows.
func (c *Collector) Export(ctx context.Context, filter QueryFilter, maxRows int) ([]Event, error) {
	if maxRows <= 0 || maxRows > exportRowCap {
		maxRows = exportRowCap
	}
	filter.Limit = maxRows
	filter.Cursor = 0
	if c.dbRead == nil {
		filter.Sort = "asc"
		return c.searchRing(filter), nil
	}
	where, args := filter.where()
	rows, err := c.dbRead.QueryContext(ctx,
		"SELECT id,time,name,type,client,source,rcode,duration_ms FROM queries"+where+
			" ORDER BY id ASC LIMIT ?", append(args, maxRows)...)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Count totals every row matching the filter. Identical filters are served
// from a short-lived cache so paging a large history never re-scans the
// table per page just to render a total.
func (c *Collector) Count(ctx context.Context, filter QueryFilter) (int64, error) {
	key := filter.cacheKey()
	now := time.Now()
	c.countMu.Lock()
	entry, cached := c.countCache[key]
	c.countMu.Unlock()
	if cached && now.Before(entry.expires) {
		return entry.count, nil
	}
	var count int64
	if c.dbRead == nil {
		for _, e := range c.Events(c.max) {
			if filter.match(e) {
				count++
			}
		}
	} else {
		where, args := filter.conditions()
		row := c.dbRead.QueryRowContext(ctx, "SELECT COUNT(*) FROM queries"+where, args...)
		if err := row.Scan(&count); err != nil {
			return 0, err
		}
	}
	c.countMu.Lock()
	if c.countCache == nil {
		c.countCache = make(map[string]countEntry)
	}
	if len(c.countCache) >= 64 {
		c.countCache = make(map[string]countEntry)
	}
	c.countCache[key] = countEntry{count: count, expires: now.Add(countCacheTTL)}
	c.countMu.Unlock()
	return count, nil
}

// TopEntry is one ranked aggregation row.
type TopEntry struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Tops ranks the most queried names, busiest clients, and most common record
// types and response codes inside a time window.
type Tops struct {
	Domains []TopEntry `json:"domains"`
	Clients []TopEntry `json:"clients"`
	Types   []TopEntry `json:"types"`
	Rcodes  []TopEntry `json:"rcodes"`
}

const topsCacheTTL = 30 * time.Second

type cachedTops struct {
	tops    Tops
	expires time.Time
}

// Tops returns top-N aggregations for queries logged since the given time.
// Persisted aggregation is a single table scan folded in Go, then cached
// briefly so repeated dashboard or scraper requests stay cheap.
func (c *Collector) Tops(ctx context.Context, since time.Time, limit int) (Tops, error) {
	if limit <= 0 {
		limit = 10
	}
	cacheKey := since.UTC().Format(time.RFC3339Nano) + "|" + itoaForCache(limit)
	if c.dbRead != nil {
		c.topsMu.Lock()
		entry, fresh := c.topsCache[cacheKey]
		c.topsMu.Unlock()
		if fresh && time.Now().Before(entry.expires) {
			return capTops(entry.tops, limit), nil
		}
	}
	tops, err := c.computeTops(ctx, since, limit)
	if err != nil {
		return Tops{}, err
	}
	if c.dbRead != nil {
		c.topsMu.Lock()
		if c.topsCache == nil {
			c.topsCache = make(map[string]cachedTops)
		}
		if len(c.topsCache) >= 16 {
			c.topsCache = make(map[string]cachedTops)
		}
		c.topsCache[cacheKey] = cachedTops{tops: tops, expires: time.Now().Add(topsCacheTTL)}
		c.topsMu.Unlock()
	}
	return capTops(tops, limit), nil
}

// capTops trims cached results down to a smaller requested limit.
func capTops(tops Tops, limit int) Tops {
	return Tops{
		Domains: tops.Domains[:min(len(tops.Domains), limit)],
		Clients: tops.Clients[:min(len(tops.Clients), limit)],
		Types:   tops.Types[:min(len(tops.Types), limit)],
		Rcodes:  tops.Rcodes[:min(len(tops.Rcodes), limit)],
	}
}

func (c *Collector) computeTops(ctx context.Context, since time.Time, limit int) (Tops, error) {
	if c.dbRead == nil {
		return c.topsRing(since, max(limit, 50)), nil
	}
	rows, err := c.dbRead.QueryContext(ctx,
		"SELECT name, client, type, rcode FROM queries WHERE epoch >= ?",
		since.UTC().Unix())
	if err != nil {
		return Tops{}, err
	}
	defer rows.Close()
	domainCounts := make(map[string]int64)
	clientCounts := make(map[string]int64)
	typeCounts := make(map[string]int64)
	rcodeCounts := make(map[string]int64)
	for rows.Next() {
		var name, client, recordType, rcode string
		if err := rows.Scan(&name, &client, &recordType, &rcode); err != nil {
			return Tops{}, err
		}
		domainCounts[name]++
		clientCounts[client]++
		typeCounts[recordType]++
		rcodeCounts[rcode]++
	}
	if err := rows.Err(); err != nil {
		return Tops{}, err
	}
	return Tops{
		Domains: topOf(domainCounts, max(limit, 50)),
		Clients: topOf(clientCounts, max(limit, 50)),
		Types:   topOf(typeCounts, max(limit, 50)),
		Rcodes:  topOf(rcodeCounts, max(limit, 50)),
	}, nil
}

func itoaForCache(value int) string {
	return fmt.Sprintf("%d", value)
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	defer rows.Close()
	out := make([]Event, 0, 100)
	for rows.Next() {
		var e Event
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Name, &e.Type, &e.Client, &e.Source, &e.Rcode, &e.DurationMS); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, err
		}
		e.Time = parsed
		out = append(out, e)
	}
	return out, rows.Err()
}

// searchRing applies the filter over the in-memory ring when no history
// database is open, emulating the keyset ordering of the SQLite path.
func (c *Collector) searchRing(filter QueryFilter) []Event {
	events := c.Events(c.max)
	matched := make([]Event, 0, len(events))
	for _, e := range events {
		if filter.match(e) {
			matched = append(matched, e)
		}
	}
	if filter.Sort == "asc" {
		slices.Reverse(matched)
	}
	if filter.Cursor > 0 {
		start := 0
		for start < len(matched) {
			if filter.Sort == "asc" {
				if matched[start].ID > filter.Cursor {
					break
				}
			} else if matched[start].ID < filter.Cursor {
				break
			}
			start++
		}
		matched = matched[start:]
	}
	limit := filter.Limit
	if limit <= 0 || limit > MaxPageSize {
		limit = 100
	}
	return matched[:min(len(matched), limit)]
}

func (c *Collector) topsRing(since time.Time, limit int) Tops {
	domainCounts := make(map[string]int64)
	clientCounts := make(map[string]int64)
	typeCounts := make(map[string]int64)
	rcodeCounts := make(map[string]int64)
	for _, e := range c.Events(c.max) {
		if e.Time.Before(since) {
			continue
		}
		domainCounts[e.Name]++
		clientCounts[e.Client]++
		typeCounts[e.Type]++
		rcodeCounts[e.Rcode]++
	}
	return Tops{
		Domains: topOf(domainCounts, limit),
		Clients: topOf(clientCounts, limit),
		Types:   topOf(typeCounts, limit),
		Rcodes:  topOf(rcodeCounts, limit),
	}
}

// topOf ranks counts by descending volume with the value as a stable tiebreak.
func topOf(counts map[string]int64, limit int) []TopEntry {
	entries := make([]TopEntry, 0, len(counts))
	for value, count := range counts {
		entries = append(entries, TopEntry{Value: value, Count: count})
	}
	slices.SortFunc(entries, func(a, b TopEntry) int {
		if a.Count != b.Count {
			return int(b.Count - a.Count)
		}
		return strings.Compare(a.Value, b.Value)
	})
	return entries[:min(len(entries), limit)]
}
