# HTTP management API

The HTTP transport validates JSON and orchestrates Records, Settings, Resolution and Observability. It does not implement DNS business invariants.

| Method | Endpoint | Purpose |
|---|---|---|
| GET | /api/v1/health | Liveness |
| GET/POST | /api/v1/records | List/create records |
| PUT/DELETE | /api/v1/records/{id} | Edit/delete records |
| GET/PUT | /api/v1/upstreams | View and change live upstream resolvers |
| GET | /api/v1/queries | Built-in persistent query history |
| GET | /api/v1/metrics/summary | Built-in process counters |
| POST | /api/v1/dns/resolve | Test active DNS resolution |
| GET | /api/v1/session | Authentication status |

Management operates only on loopback unless OIDC is configured. All mutating endpoints require same-origin browser requests, and settings are written to SQLite before being applied.
