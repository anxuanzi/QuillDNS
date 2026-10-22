# Contributing to QuillDNS

Thank you for helping make reliable DNS infrastructure accessible.

## Engineering principles

1. Prefer readable code to novel abstractions. Each package should have one clear domain purpose and a local README.
2. Use domain-driven boundaries: DNS rules stay in the record/resolution domain, persistence remains behind domain APIs, and Vue never duplicates backend invariants.
3. Follow test-driven development when adding behavior. Start with a failing case, make the smallest correct change, then refactor and document.
4. Do not add a mandatory external service or runtime dependency for caching, analytics, DNS configuration, or frontend hosting.
5. Keep DNS hot paths allocation-conscious and independent of disk or network management APIs.
6. Never introduce a feature flag that silently disables client access control or management authentication restrictions.
7. Document operational failure cases and security assumptions before publishing new configuration surface area.

## Local checks

```sh
npm ci --prefix web
npm run build --prefix web
go mod download
gofmt -w cmd internal webassets
go test -race ./...
go vet ./...
```

For integration tests use ephemeral loopback UDP/TCP sockets and deterministic simulated upstream responses, not public DNS network access.

When making UI changes, keep accessible labels, keyboard navigation, coherent loading and error states, responsive breakpoints, and a clear visual hierarchy. Reuse Nuxt UI and the established QuillDNS design tokens rather than introducing another component system.

Avoid claiming benchmarks without captured hardware, workloads, and repeatable scripts. Release changes must pass the CI checks and the release readiness checklist documented in `docs/architecture.md`.
