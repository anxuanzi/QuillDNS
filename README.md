<div align="center">
  <a href="https://github.com/anxuanzi/QuillDNS">
    <img src="assets/brand/quilldns-wordmark.svg" width="460" alt="QuillDNS — DNS, under your control" />
  </a>
  <p><strong>Your DNS. Your rules. Your visibility.</strong></p>
  <p>A fast, self-hosted DNS proxy with local overrides, a beautiful web console,<br />and all the observability you need — built into one Go application.</p>
  <p>
    <a href="https://github.com/anxuanzi/QuillDNS/actions/workflows/ci.yml"><img src="https://github.com/anxuanzi/QuillDNS/actions/workflows/ci.yml/badge.svg" alt="Go and build CI" /></a>
    <a href="https://github.com/anxuanzi/QuillDNS/actions/workflows/browser-e2e.yml"><img src="https://github.com/anxuanzi/QuillDNS/actions/workflows/browser-e2e.yml/badge.svg" alt="Browser acceptance tests" /></a>
    <img src="https://img.shields.io/badge/Go-backend-00ADD8?logo=go&logoColor=white" alt="Go backend" />
    <img src="https://img.shields.io/badge/Vue_3-frontend-42b883?logo=vuedotjs&logoColor=white" alt="Vue 3 frontend" />
    <img src="https://img.shields.io/badge/Bun-tooling-f4e9d8?logo=bun&logoColor=171717" alt="Bun frontend tooling" />
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-64748b" alt="Apache 2.0 license" /></a>
  </p>
  <p>
    <a href="#quick-start">Quick start</a> ·
    <a href="#how-it-works">How it works</a> ·
    <a href="#configuration--security">Configuration</a> ·
    <a href="#documentation">Documentation</a> ·
    <a href="#quality--release-status">Release status</a>
  </p>
</div>

---

> [!IMPORTANT]
> **QuillDNS is a pre-release project.** It is intended for development and trusted private networks while DNS protocol conformance, security review, browser testing and recovery drills are completed. **Do not expose it as an unrestricted public recursive resolver.**

## Why QuillDNS?

Most DNS tools make you choose between easy local rewrites, powerful record management, and useful operational visibility. QuillDNS brings them together without requiring a separate metrics stack, database server, or management service.

<table>
<tr>
<td width="50%" valign="top">

### ⚡ Fast by design
Go DNS server with immutable in-memory record snapshots, UDP/TCP resolution, caching, and upstream fallback.

### 🧭 Control every override
Add, edit, and remove **A, AAAA, CNAME, TXT, MX, SRV, PTR, and CAA** records from the browser or API.

</td>
<td width="50%" valign="top">

### 📈 Observe everything
Query history, resolution sources, counters, cache activity, and latency — in the built-in dashboard.

### 📦 One deployable service
An embedded Vue 3 SPA, SQLite storage, optional OIDC login, and no external runtime services.

</td>
</tr>
</table>

## Quick start

The simplest way to explore QuillDNS is on your own machine, with DNS and the management UI both bound to loopback.

### Option A — Pull the prebuilt Docker image (Linux)

No Go or Bun installation is required. Docker Compose pulls QuillDNS from **GitHub Container Registry**:

```bash
mkdir quilldns && cd quilldns
curl -fsSLo compose.yaml https://raw.githubusercontent.com/anxuanzi/QuillDNS/main/deploy/compose.yaml
docker compose up -d
```

Open **http://127.0.0.1:8080** on the Docker host. DNS listens on **127.0.0.1:1053** (UDP/TCP). All settings and query history persist in the named Docker volume.

```bash
docker compose pull             # fetch a newer image
docker compose up -d            # restart with persistent data
docker compose logs -f          # inspect resolver logs
```

<sub>Default image: `ghcr.io/anxuanzi/quilldns:edge` (tested main-branch builds). Stable `:latest` and semver tags are reserved for versioned releases. GHCR packages may initially be private; package owners must enable Public visibility for anonymous pulls.</sub>

> **Safety:** The default Linux host-network deployment is loopback-only, with no public recursive DNS or remote management. To serve your LAN, configure explicit client CIDRs and a private-interface listener. See [Docker deployment and environment variables](deploy/README.md).

### Option B — Go + Bun

Prerequisites: **Go 1.23+** and **Bun 1.3.5+**.

```bash
git clone https://github.com/anxuanzi/QuillDNS.git
cd QuillDNS

cd web
bun install --frozen-lockfile
bun run build
cd ..

go run ./cmd/quilldns
```

Open **http://127.0.0.1:8080** and create your first local DNS record.

### Try a local override

The dashboard provides a record editor, or you can use the REST API:

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/v1/records \
  -H 'Content-Type: application/json' \
  -d '{"name":"api.internal.test","type":"A","value":"10.0.0.20","ttl":300}'
```

Confirm it resolves locally:

```bash
dig @127.0.0.1 -p 1053 api.internal.test A
```

Expected local answer: `10.0.0.20`. Requests without a matching local override are forwarded to configured upstream resolvers.

## How it works

```text
                    ┌─────────────────────────────┐
                    │ DNS clients · UDP and TCP    │
                    └──────────────┬──────────────┘
                                   │
                          ┌────────▼────────┐
                          │ QuillDNS resolver│
                          └────────┬────────┘
                                   │
                        Local name + type?
                         /               \
                       Yes                No
                        │                  │
                ┌───────▼──────┐   ┌───────▼─────────┐
                │ Local record │   │ Response cache  │
                └──────────────┘   └───────┬─────────┘
                                           │ cache miss
                                    ┌──────▼─────────┐
                                    │ DNS upstreams  │
                                    └────────────────┘

     Vue 3 dashboard ── REST API ── SQLite settings & query history
                         All inside one self-hosted Go application
```

**Record-level fallback is intentional:** if you override `api.example.com A` but not `api.example.com TXT`, its TXT query can still resolve through your public upstream. QuillDNS is an override proxy, **not** automatically an authoritative DNS server for the entire domain.

## The management console

A Vue 3 + Nuxt UI single-page application, built with **Vite and Bun** and embedded directly into the final Go executable.

| Area | What you can do |
|:--|:--|
| **Overview** | Inspect resolver activity, local/cache/upstream responses, and latency |
| **DNS Records** | Search, add, edit, enable/disable, and remove local records |
| **Query Logs** | Explore recent DNS activity and response sources |
| **Settings** | Manage DNS upstreams and inspect authentication settings |
| **DNS Lookup** | Test DNS resolution from the management UI |

**Actual interface captures:** the [CI workflow](https://github.com/anxuanzi/QuillDNS/actions/workflows/ci.yml) invokes real Chromium tests against the compiled Go binary and uploads desktop/mobile screenshots. These are real captures, not generated mockups. Open a successful CI run's `browser / chromium` job and download its `quilldns-ui-screenshots` artifact.

## Docker environment configuration

The Go server now supports `QUILLDNS_*` environment variables for containerized deployments. CLI flags remain supported and **override** env values when both are supplied.

| Environment variable | Image default | Purpose |
|:--|:--|:--|
| `QUILLDNS_DATA_PATH` | `/data/quilldns.db` | SQLite configuration and query-history storage |
| `QUILLDNS_DNS_LISTEN` | `127.0.0.1:1053` | DNS UDP/TCP listener |
| `QUILLDNS_HTTP_LISTEN` | `127.0.0.1:8080` | Embedded management UI + API |
| `QUILLDNS_ALLOW_NETWORKS` | `127.0.0.0/8,::1/128` | Permitted DNS client subnets |
| `QUILLDNS_UPSTREAMS` | `1.1.1.1:53,8.8.8.8:53` | Comma-separated fallback resolvers |

The image also accepts the optional `QUILLDNS_OIDC_*` authentication variables. Copy [the Docker Compose environment example](deploy/.env.example) to customize an installation. Run `cd deploy && docker compose up -d` from a cloned repository, or keep a downloaded `compose.yaml` alongside your `.env` file.

## Automated Docker images

The [CI workflow](https://github.com/anxuanzi/QuillDNS/actions/workflows/ci.yml) tests the Go resolver, Vue UI in Chrome, and the container before publishing a multi-architecture image to **[GHCR](https://github.com/anxuanzi/QuillDNS/pkgs/container/quilldns)**. `main` publishes `:edge` and an immutable commit-SHA tag; `vX.Y.Z` tags publish semver tags and, for stable releases, `:latest`. Full details are in [the deployment guide](deploy/README.md).

## Configuration & security

### Using QuillDNS on a trusted LAN

The default DNS listener is loopback-only. To answer requests from a private subnet, specify an interface **and** a client CIDR allowlist:

```bash
./quilldns \
  -dns 192.168.10.10:53 \
  -allow-networks 192.168.10.0/24,127.0.0.0/8,::1/128
```

Firewall filtering is recommended in addition to the built-in CIDR policy. Never permit unrestricted recursion from the public internet.

### Optional OIDC login

Without OIDC, the management server intentionally remains **loopback-only**. To enable remote management, configure an OIDC provider and place QuillDNS behind a trusted HTTPS reverse proxy:

```bash
export QUILLDNS_OIDC_ISSUER='https://id.example.com/realms/home'
export QUILLDNS_OIDC_CLIENT_ID='quilldns'
export QUILLDNS_OIDC_CLIENT_SECRET='set-from-a-secret-manager'
export QUILLDNS_PUBLIC_URL='https://dns.example.com'
export QUILLDNS_OIDC_ALLOWED_EMAILS='admin@example.com'

./quilldns -http 0.0.0.0:8080
```

Register `https://dns.example.com/auth/callback` with the identity provider. The example is illustrative; see [authentication details](internal/auth/README.md) and [security guidance](SECURITY.md) before making the management UI remotely accessible.

## Build & test

```bash
make build        # Bun + Vite frontend, embedded in one Go binary
make test         # Go race-enabled unit and integration tests
make vet          # Go static analysis
make verify       # Build and verify formatting and dependencies
```

Frontend development:

```bash
cd web
bun install --frozen-lockfile
bun run dev
```

The [CI workflow](https://github.com/anxuanzi/QuillDNS/actions/workflows/ci.yml) checks formatting, race tests, static analysis, Bun/Vite compilation, Docker packaging and a restart-persistence smoke test. It also runs the reusable [Chromium acceptance suite](tests/e2e/README.md), which captures real UI screenshots, **before** allowing a GHCR image publication.

## Documentation

| Guide | Scope |
|:--|:--|
| [Architecture](docs/architecture.md) | Domain boundaries, DNS semantics, design decisions, and current limitations |
| [DNS resolution](internal/dnsserver/README.md) | Resolver, upstreams, cache, and transport |
| [DNS records](internal/records/README.md) | Record types, validation, SQLite persistence, and fallback |
| [API](internal/httpapi/README.md) | REST endpoints and transport contract |
| [Authentication](internal/auth/README.md) | Optional OIDC and session handling |
| [Observability](internal/observability/README.md) | Local metrics, query history, and retention |
| [Frontend](web/README.md) | Vue 3, Nuxt UI, Pinia, Pinia Colada, and Bun |
| [Deployment](deploy/README.md) | Docker and operational considerations |
| [Testing](docs/testing.md) | Quality gates, DNS integration tests, and browser E2E |
| [Contributing](CONTRIBUTING.md) | Coding standards, TDD, DDD, and review expectations |

## Quality & release status

QuillDNS is designed for long-term maintainability with clear Go domain packages, module READMEs, Vue feature boundaries, and continuous testing.

**Implemented:** record management, configurable upstreams, first-party analytics, embedded GUI, SQLite persistence, optional OIDC and access restrictions.

**Before a stable production release:** full DNSSEC/EDNS conformance review, security and OIDC integration review, sustained load/fuzz testing, recovery drills, and reproducible versioned release artifacts. Track current CI results using the badges above; a successful build does not substitute for a security assessment.

---

<div align="center">
  <img src="assets/brand/quilldns-mark.svg" width="48" alt="" />
  <p><strong>QuillDNS</strong> · DNS, under your control.</p>
  <p>Apache-2.0 licensed · Made for infrastructure you own.</p>
</div>
