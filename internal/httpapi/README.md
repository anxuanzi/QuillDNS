# HTTP management API

The HTTP transport validates JSON and orchestrates Records, Settings, Resolution and Observability. It does not implement DNS business invariants.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | /api/v1/health | Liveness |
| GET/POST | /api/v1/records | List/create records |
| PUT/DELETE | /api/v1/records/{id} | Edit/delete records |
| GET/PUT | /api/v1/upstreams | View and change live upstream resolvers |
| GET | /api/v1/queries | Paginated, filterable persistent query history |
| GET | /api/v1/queries/export | CSV download of filtered history (50,000-row cap) |
| GET | /api/v1/metrics/summary | Built-in process counters |
| GET | /api/v1/metrics/trend | Query volume/latency buckets (`window=1h` default, `24h`, `7d`) |
| GET | /api/v1/metrics/tops | Top queried domains, clients, types, response codes |
| GET | /api/v1/metrics/resolver | Cache stats and per-upstream forwarder health |
| POST | /api/v1/dns/resolve | Test active DNS resolution |
| GET | /api/v1/session | Authentication status |
| GET | /metrics | Prometheus text exposition of the same telemetry |

Query-history parameters: `q` (name or client substring), `name`, `client`, `source`, `rcode`, `type`, `since`/`until` (RFC3339), `sort` (`asc`/`desc`, default newest), `cursor` (keyset continuation id), `limit` (1–500, default 100), `count=1` (add total). Responses are `{data, next_cursor?, total?}`; an absent `next_cursor` means the end of the walk. Invalid parameters are rejected with 400.

```text
GET /api/v1/queries?rcode=NXDOMAIN&since=2026-10-22T00:00:00Z&limit=2

{"data":[
  {"id":9842,"time":"2026-10-22T09:41:03.114Z","name":"missing.example.org.",
   "type":"A","client":"192.168.1.24","source":"upstream","rcode":"NXDOMAIN",
   "duration_ms":41.2},
  …
 ],
 "next_cursor":9841,"total":37}
```

Pass `cursor=9841` on the next request (with identical filters) to continue the walk. Keyset pagination keeps deep pages as fast as the first one, and new events arriving mid-walk are never revisited.

Management operates only on loopback unless OIDC is configured. All mutating endpoints require same-origin browser requests, and settings are written to SQLite before being applied.
