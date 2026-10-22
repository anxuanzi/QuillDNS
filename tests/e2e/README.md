# Browser acceptance tests

Playwright runs headless Chromium against the **built and embedded Vue 3 SPA** from the Go binary, using an isolated SQLite database. Tests exercise dashboard data, DNS record creation through the UI, lookup verification, query logs, settings and a mobile viewport.

## Run locally

```sh
cd web && bun install --frozen-lockfile && bun run build && cd ..
go build -o quilldns ./cmd/quilldns
./quilldns -dns 127.0.0.1:11053 -http 127.0.0.1:18080 -data /tmp/quilldns-e2e.sqlite
```

In another terminal:

```sh
python -m pip install playwright==1.56.0
python -m playwright install chromium
python tests/e2e/test_console.py
```

**Only run against a disposable database**: tests create DNS records. CI uploads actual browser screenshots as `quilldns-ui-screenshots`. These are not promotional mockups and do not contain production user data.

When modifying the UI, review both the automated assertions and the resulting 1440px/390px screenshots for clipping, hierarchy, contrast, focus states and responsive layout.
