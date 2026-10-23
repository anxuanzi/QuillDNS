# Integrated observability

Every DNS query creates a small in-memory event and contributes to cumulative process-lifetime metrics. An optional bounded queue writes batches to an embedded SQLite database; the DNS hot path never waits on the database. There are no mandatory Prometheus, Grafana, or broker services.

- Persistent queries use a dedicated `queries.sqlite` file in WAL mode, written by a single connection; management reads run on a separate read pool so heavy analytics never delay log flushes.
- Events are flushed once per second or at 512 events, whichever happens first.
- Memory events are bounded and counters are process-lifetime.
- Retention is configurable: `QUILLDNS_LOG_RETENTION_DAYS` (default 7, 1–365) and `QUILLDNS_LOG_MAX_ROWS` (default 100,000, 1,000–5,000,000). Both are enforced by an hourly prune, so logging can never grow without bound.
- During overload, log events may be dropped instead of delaying DNS; `dropped_logs` reports that condition.
- Client IP and hostname are sensitive operational data. Restrict access and backups appropriately.
- Privacy controls for query-name logging need further work.

## Native performance analytics

The summary exposes process-lifetime average latency, rolling p95/p99 latencies from the bounded in-memory event ring, and per-resolution-source mean latencies (local, cache, upstream). Percentiles are **not lifetime percentiles**.

Trends group persistent events into fixed-width buckets for a selectable window — `1h` (12×5min), `24h` (24×1h) or `7d` (28×6h) — including zero-query intervals. Wider windows widen buckets so aggregation cost stays flat as history grows.

Rankings (`/api/v1/metrics/tops`) aggregate the most queried names, busiest clients, record types and response codes over the same windows.

Resolver state (`/api/v1/metrics/resolver`) snapshots the response cache (entries, hit/miss/eviction counters) and per-upstream forwarder health: request and error counts, cumulative and average latency, last error, and breaker state. The upstream breaker skips a forwarder after three consecutive exchange failures for 30 seconds, so a dead upstream stops taxing client queries with its full timeout.

Query history (`/api/v1/queries`) is server-side paginated with keyset cursors and filterable by name/client substring (`q`, `name`, `client`), source, response code, record type and RFC3339 time range (`since`, `until`), in either sort direction. Page sizes are capped at 500; `count=1` adds the total matching rows. The same filters drive bounded CSV downloads via `/api/v1/queries/export` (50,000-row cap). All filters are parameterized SQL; search input is LIKE-escaped.

## Prometheus endpoint

`GET /metrics` (guarded like the API when OIDC is on) exposes the same telemetry in Prometheus text format, implemented with the standard library only — no client library, no extra services.

| Metric | Type | Meaning |
|:--|:--|:--|
| `quilldns_queries_total{source}` | counter | Queries answered, by `local` / `cache` / `upstream` |
| `quilldns_responses_total{rcode}` | counter | Responses by response code |
| `quilldns_query_errors_total` | counter | Non-NOERROR responses |
| `quilldns_query_duration_seconds` | histogram | Cumulative latency, 14 sub-millisecond-to-10s bounds |
| `quilldns_cache_{hits,misses,evictions}_total` | counter | Response-cache activity |
| `quilldns_cache_entries` / `_max_entries` | gauge | Cache occupancy and capacity |
| `quilldns_upstream_requests_total{upstream,result}` | counter | Forwarded exchanges, `result="ok"\|"error"` |
| `quilldns_upstream_latency_seconds_{sum,count}{upstream}` | counter | Cumulative upstream exchange latency |
| `quilldns_log_events_dropped_total` | counter | History events dropped under overload |
| `quilldns_uptime_seconds` / `quilldns_build_info` | gauge | Process age and build metadata |

REST: `GET /api/v1/metrics/summary`, `GET /api/v1/metrics/trend?window=1h|24h|7d`, `GET /api/v1/metrics/tops?window=…&limit=…`, `GET /api/v1/metrics/resolver`, `GET /api/v1/queries`, `GET /api/v1/queries/export`, `GET /metrics`. The DNS lookup-testing endpoint also includes `duration_ms` for each requested test.
