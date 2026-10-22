# Web management application

Vue 3 + TypeScript SPA compiled by Vite and styled with Nuxt UI v4. Pinia owns small application/UI state; Pinia Colada manages server-state queries and invalidation. No React or Nuxt runtime.

## Development
1. `bun install --frozen-lockfile`
2. `bun run dev` (Vite dev server at localhost:5173 with API proxy to localhost:8080)
3. Run the Go server independently: `go run ./cmd/quilldns`.
4. `bun run build` writes `../webassets/dist`, which `go:embed` includes in the final binary.

The committed `webassets/dist/index.html` is a deliberate development placeholder. A release must build the frontend before compiling Go.

## Boundaries
`src/features/records` owns DNS record views, `src/features/overview` owns operational views, and `src/lib` owns HTTP transport helpers. Never store a second authoritative copy of records in Pinia; queries are managed by Pinia Colada.

## Color modes and performance views

QuillDNS defaults to a light theme and provides a dark-mode button in the header. The preference is retained locally in the browser; no external theme or settings service is required. Both modes use the same design tokens in `src/style.css`.

The overview plots **actual** five-minute query-volume and processing-time samples from the Go API using Nuxt UI's `UChart` component and its TanStack Charts peer. Mean, rolling p95/p99, per-source latencies and DNS lookup processing time are labeled with their measurement scopes. Empty periods are represented by zero-query buckets rather than invented values.

Record and upstream save buttons use local async `try/finally` state (not a nested mutation-status ref) and recover correctly from server errors. Browser acceptance tests verify the idle, successful, and rejected-save states.
