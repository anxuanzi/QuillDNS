# QuillDNS architecture and engineering guide

## Principles
- Business behavior lives in domain packages; transport handlers depend on domain interfaces and never own DNS invariants.
- A DNS lookup must not depend on a database or frontend request.
- Never introduce a required external service for metrics, caching, configuration, or query storage.
- Prefer small, reviewable components; document important decisions, test the observable behavior, and avoid abstraction without a use case.
- Version API boundaries. Validate DNS data server-side.
- Deny unsafe public access by default.

## Bounded contexts
| Domain | Ownership | Current persistence |
|---|---|---|
| Records | normalization, RR validation, conflict rules, immutable snapshots | atomic JSON file |
| Resolution | local-first DNS, upstreams, response cache | in-memory |
| Observability | query events and aggregate counters | in-memory bounded ring |
| Management | HTTP orchestration / DTOs | delegates to domains |
| Web | operational workflows | browser-only UI state |

The initial modules are separated by Go package and documented with local README files. Keep interfaces near consumers rather than creating a generic services framework.

## Testing contract
- Unit tests: normalization, RR conversion, type-specific fallback, CNAME conflicts, persistence.
- API tests: valid/invalid mutations, response shape, cross-origin denial.
- DNS tests: local answers, upstream delegation, positive cache.
- Upcoming: RFC edge cases, EDNS and DNSSEC pass-through, truncation, client controls, concurrency race tests, browser automation, performance benchmarks.
- CI must execute `go test ./...`, `go vet ./...`, frontend typechecking and frontend build.

## Known compromises in foundation
1. JSON persistence is intentional for a working vertical slice; SQLite with transactional migrations is the planned embedded storage layer.
2. A response cache is included but is not yet a hardened DNS cache. EDNS requests bypass it and positive-cache response TTLs need thorough protocol tests.
3. The current query history is memory-only, with a fixed maximum size. SQLite-backed retention and rolling metrics are planned.
4. OIDC is **not** currently available. Bind management to loopback until an authentication implementation passes security review.
5. Restrict DNS listeners to private interfaces. Network ACLs, rate limiting, and abuse protections are not complete.
6. The frontend ships embedded only after `npm run build`.

## Milestones
1. DNS protocol correctness and resilient upstream behavior.
2. Transactional embedded storage and dynamic settings.
3. OIDC provider integration, sessions and CSRF protections.
4. Durable in-product query telemetry and latency histograms.
5. Deployment, packaging, integration tests and load benchmarks.

## Record fallback contract
QuillDNS is an **override proxy**, not an authoritative DNS zone by default. Only a matching local RRset intercepts the request; other types at the same name forward to public upstreams. CNAME acts as an alias override and has special conflict constraints. DNSSEC cannot truthfully authenticate synthetic records; design explicit AD flag handling and downstream expectations before general use.
