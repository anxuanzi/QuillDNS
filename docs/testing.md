# DNS testing and quality gates

Go unit tests verify record normalization, persistent snapshots, response source selection, cache behavior, access policies, API validation, OIDC middleware and SQLite query history.

The UDP/TCP integration test starts real DNS listeners and a controlled upstream on ephemeral loopback ports. It checks actual DNS packets for local A, local TXT, type-specific fallback and cached public answers.

Run:

```sh
go test -race ./...
go test ./internal/dnsserver -run TestUDPAndTCPResolutionEndToEnd -v
go test ./internal/records -run '^$' -bench BenchmarkSnapshotLocalLookup -benchmem
```

CI also builds the Vue SPA and checks Go formatting, vet, and tidy dependency metadata. A passing test suite is a prerequisite, **not a production security certification**.

Before a production release, add RFC conformance vectors, fuzz testing, stress tests over IPv4/IPv6, DNSSEC integration, denial-of-service analysis, browser-based E2E tests, and backup/recovery drills.


## Real browser verification

See [Playwright E2E tests and screenshot capture](../tests/e2e/README.md). The CI workflow publishes real Chromium screenshots of the embedded Vue SPA for desktop and mobile review.
