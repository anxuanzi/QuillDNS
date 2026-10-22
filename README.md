# QuillDNS

**DNS, under your control.**

QuillDNS is an open-source, self-hosted Go DNS proxy with local record overrides, public DNS forwarding, a responsive Vue 3 management interface, and first-party observability. This repository currently contains an **early development foundation**, not a production release.

## Architecture

- **DNS** — `miekg/dns` UDP/TCP server; exact local RRsets, positive cache, ordered upstream fallback.
- **Records** — validated A, AAAA, CNAME, TXT, MX, SRV and PTR records; immutable in-memory snapshots; embedded JSON persistence.
- **API** — Go `net/http` management endpoints with no mandatory service dependency.
- **Observability** — integrated in-memory query history and counters, not Prometheus or Grafana.
- **UI** — Vue 3 + Vite + Nuxt UI + Pinia + Pinia Colada; compiled into Go via `go:embed`.

Documentation: [DNS](internal/dnsserver/README.md), [Records](internal/records/README.md), [HTTP API](internal/httpapi/README.md), [Observability](internal/observability/README.md), [Frontend](web/README.md), [Architecture](docs/architecture.md).

## Development

Prerequisites: Go 1.23+, Node.js 22.12+ (or Node 20.19+), npm.

```bash
cd web
npm install
npm run build
cd ..
go mod tidy
go test ./...
go run ./cmd/quilldns
```

The API and embedded UI default to `http://127.0.0.1:8080`. DNS defaults to `127.0.0.1:1053` on both UDP and TCP, avoiding privileged ports during development.

```bash
dig @127.0.0.1 -p 1053 example.org A
curl http://127.0.0.1:8080/api/v1/health
```

Production builds **must** compile the frontend first. A placeholder embedded index exists to keep backend tests buildable without Node dependencies.

## Example API

```bash
curl -X POST http://127.0.0.1:8080/api/v1/records \
  -H 'Content-Type: application/json' \
  -d '{"name":"api.internal.test","type":"A","value":"10.0.0.20","ttl":300}'

dig @127.0.0.1 -p 1053 api.internal.test A
```

## Security and release status

**Do not deploy this foundation as an unrestricted public resolver.** OIDC is not implemented; management binds to loopback by default, DNS also binds to loopback, and client access control is not implemented. Use this version only for local development or within secured environments.

Further work includes OIDC, persistent query history, comprehensive protocol tests, EDNS/DNSSEC semantics, wildcard overrides, encrypted upstreams, health-aware upstream failover, query-cache hardening, and end-to-end browser tests. See [architecture and roadmap](docs/architecture.md).

Licensed under Apache-2.0.
