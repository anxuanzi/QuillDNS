# Bun-first frontend workflow

QuillDNS uses **Bun** for frontend package installation, scripts and lockfile management. Vue 3, Vite, Nuxt UI, Pinia and Pinia Colada remain the frontend stack. The application remains a static SPA embedded in the production Go binary.

## Working on the frontend

```sh
cd web
bun install --frozen-lockfile
bun run dev
bun run typecheck
bun run build
```

Commit `web/bun.lock` whenever frontend dependencies change. Do not commit `package-lock.json` or use npm to regenerate the dependency tree. CI and Docker both use `bun install --frozen-lockfile`.

## Release contract

1. Install dependencies from the committed Bun lockfile.
2. Typecheck Vue with `vue-tsc` and build the optimized SPA using Vite.
3. Include generated assets in `webassets/dist` before `go build`.
4. Run browser regression tests against the compiled binary.
5. Do not publish any release that embeds the development placeholder index.
