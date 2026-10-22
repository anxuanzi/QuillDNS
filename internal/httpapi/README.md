# HTTP management API

The API owns request orchestration and error translation but delegates DNS rules to the Records and DNS domains.

## Endpoints
- `GET /api/v1/health`
- `GET, POST /api/v1/records`
- `PUT, DELETE /api/v1/records/{id}`
- `GET /api/v1/queries` — recent persistent history where configured
- `GET /api/v1/metrics/summary` — built-in runtime counters
- `POST /api/v1/dns/resolve` — in-process DNS test

No external services are required. Record IDs are generated with cryptographic randomness. Request JSON has strict unknown-field and trailing-content checks. Basic same-origin and browser framing restrictions are enforced.

**OIDC is not yet implemented.** The HTTP management service refuses non-loopback binding until a proper authentication flow exists.
