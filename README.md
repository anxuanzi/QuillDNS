<div align="center">
  <img src="assets/brand/quilldns-wordmark.svg" width="610" alt="QuillDNS — DNS, under your control" />
</div>

<h1 align="center">QuillDNS</h1>

<p align="center"><strong>DNS, under your control.</strong> High-performance, self-hosted DNS with built-in management and observability.</p>

QuillDNS is a lightweight, open-source Go DNS override proxy with a self-contained Vue 3 management SPA. Local records take precedence over public DNS, and unmatched name/type lookups use configurable public upstream resolvers. It includes an in-process TTL-aware response cache, transactional embedded SQLite record storage, optional OpenID Connect sign-in, and first-party query analytics. **Pre-release: not yet certified for exposed production networks.**

## Features

- Bun-based reproducible frontend builds using a committed `bun.lock`; see [frontend tooling](docs/bun.md).

- DNS over UDP and TCP with local-first A, AAAA, CNAME, TXT, MX, SRV, PTR and CAA responses.
- Type-specific fallback to DNS upstreams, ordered failover and UDP→TCP retries for truncated replies.
- Strict DNS client CIDR policy; default loopback-only DNS and management listeners.
- Built-in query event collection, durable seven-day SQLite query history, runtime counters and drop metrics.
- Embedded Vue 3 SPA built using Vite, Nuxt UI, Pinia and Pinia Colada.
- Optional OIDC (authorization code + PKCE), protected HTTP-only sessions and email allowlist.
- Embedded SQLite persistence; no Redis, PostgreSQL, Prometheus, Grafana, or other required daemon.

## Quick start (developer)

Prerequisites: Go 1.23+ and Bun 1.3.5+ (pinned in CI and Docker).

```sh
cd web && bun install --frozen-lockfile && cd ..
cd web && bun run build && cd ..
go mod download
go test -race ./...
go run ./cmd/quilldns
```

Open http://127.0.0.1:8080; DNS listens on port 1053 over UDP and TCP on loopback.

```sh
curl -s http://127.0.0.1:8080/api/v1/health
curl -s -X POST http://127.0.0.1:8080/api/v1/records \
  -H 'Content-Type: application/json' \
  --data '{"name":"api.internal.test","type":"A","value":"10.0.0.20","ttl":60}'
dig @127.0.0.1 -p 1053 api.internal.test A
```

## Running as a DNS resolver

By default QuillDNS binds only to loopback: this is deliberate because open recursive resolvers are frequently abused. To serve a trusted subnet, bind DNS to a private address and explicitly specify accepted clients:

```sh
./quilldns -dns 192.168.10.10:53 -allow-networks 192.168.10.0/24,127.0.0.0/8,::1/128
```

Do not expose unrestricted public DNS recursion. Set firewall rules too. A zero-trust network configuration and DNSSEC correctness review are still required before critical production use.

## Authentication

When OIDC is not configured, the management UI has **no authentication** and binds exclusively to loopback. For remote management, configure an OIDC provider, a public HTTPS URL, and a reverse proxy with TLS:

```sh
export QUILLDNS_OIDC_ISSUER='https://id.example.com/realms/home'
export QUILLDNS_OIDC_CLIENT_ID='quilldns'
export QUILLDNS_OIDC_CLIENT_SECRET='your-secret-from-a-secure-store'
export QUILLDNS_PUBLIC_URL='https://dns.example.com'
export QUILLDNS_OIDC_ALLOWED_EMAILS='admin@example.com'
./quilldns -http 0.0.0.0:8080
```

Register `https://dns.example.com/auth/callback` as your provider's redirect URI. The proxy must preserve the public `Host` and enforce TLS. Empty allowed-email list admits any identity accepted by the configured provider, so set an allowlist for shared identity providers. All OIDC settings must be present together; partial configuration fails startup.

## Build a single release binary

```sh
cd web && bun install --frozen-lockfile && cd ..
cd web && bun run build && cd ..
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o quilldns ./cmd/quilldns
```

The Go `webassets` package embeds `webassets/dist` via `go:embed`. **Always build the frontend before building the release binary.** Without it, a placeholder development index may be embedded.

## Repository layout

The visual identity is maintained in [the branding guide](docs/brand/README.md).


| Domain | Documentation |
|---|---|
| DNS and caching | [internal/dnsserver/README.md](internal/dnsserver/README.md) |
| Record management | [internal/records/README.md](internal/records/README.md) |
| HTTP management API | [internal/httpapi/README.md](internal/httpapi/README.md) |
| OIDC authentication | [internal/auth/README.md](internal/auth/README.md) |
| Built-in observability | [internal/observability/README.md](internal/observability/README.md) |
| Vue SPA | [web/README.md](web/README.md) |
| Design decisions and roadmap | [docs/architecture.md](docs/architecture.md) |

## Interface previews

The [Browser acceptance and screenshots workflow](https://github.com/anxuanzi/QuillDNS/actions/workflows/browser-e2e.yml) tests the actual embedded Vue app and attaches downloadable PNG screenshots of Overview, Records, Add Record, Lookup, Query Logs, Settings, and mobile Overview. These are captured from a disposable test instance, rather than simulated product mockups.

## Release maturity

The current implementation is an actively tested foundation, **not a final production release**. Remaining high-priority verification includes DNS fuzzing, full protocol/EDNS/DNSSEC handling, session/redirect security review, configuration migration, upstream health/failover tests, E2E browser tests, backups and restore drills, and reproducible release artifacts. See [architecture](docs/architecture.md).

Apache-2.0 licensed.
