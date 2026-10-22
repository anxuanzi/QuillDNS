# Integrated observability

Every DNS query creates a small in-memory event and contributes to cumulative process-lifetime metrics. An optional bounded queue writes batches to an embedded SQLite database; the DNS hot path never waits on the database. There are no mandatory Prometheus, Grafana, or broker services.

- Persistent queries use a dedicated `queries.sqlite` file in WAL mode.
- Events are flushed once per second or at 256 events, whichever happens first.
- Memory events are bounded and counters are process-lifetime.
- Query history retains up to 100,000 rows, with seven-day pruning every hour.
- During overload, log events may be dropped instead of delaying DNS; `dropped_logs` reports that condition.
- Client IP and hostname are sensitive operational data. Restrict access and backups appropriately.
- Historical aggregations, configurable retention, privacy controls and detailed upstream timings need further work.

## Native performance analytics

The summary exposes process-lifetime average latency, rolling p95/p99 latencies from the bounded in-memory event ring, and per-resolution-source mean latencies (local, cache, upstream). Percentiles are **not lifetime percentiles**. The trend endpoint groups persistent events into twelve five-minute buckets for the past hour, including zero-query intervals. When configured with a memory-only collector it uses the ring, which may omit older events.

REST: `GET /api/v1/metrics/summary`, `GET /api/v1/metrics/trend`, and `GET /api/v1/queries`. The DNS lookup-testing endpoint also includes `duration_ms` for each requested test.
