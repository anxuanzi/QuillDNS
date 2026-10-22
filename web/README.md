# Web management application

Vue 3 + TypeScript SPA compiled by Vite and styled with Nuxt UI v4. Pinia owns small application/UI state; Pinia Colada manages server-state queries and invalidation. No React or Nuxt runtime.

## Development
1. `npm install`
2. `npm run dev` (Vite dev server at localhost:5173 with API proxy to localhost:8080)
3. Run the Go server independently: `go run ./cmd/quilldns`.
4. `npm run build` writes `../webassets/dist`, which `go:embed` includes in the final binary.

The committed `webassets/dist/index.html` is a deliberate development placeholder. A release must build the frontend before compiling Go.

## Boundaries
`src/features/records` owns DNS record views, `src/features/overview` owns operational views, and `src/lib` owns HTTP transport helpers. Never store a second authoritative copy of records in Pinia; queries are managed by Pinia Colada.
