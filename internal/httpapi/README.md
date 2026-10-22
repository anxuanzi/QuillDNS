# Management API

HTTP transport and request validation. The API orchestrates record persistence and DNS testing but does not implement record semantics.

## Routes
- `GET /api/v1/health`
- `GET, POST /api/v1/records`
- `DELETE /api/v1/records/{id}`
- `GET /api/v1/queries`
- `GET /api/v1/metrics/summary`
- `POST /api/v1/dns/resolve`

JSON responses are intentionally small and versioned. No unauthenticated remote management is safe: the default HTTP listen address is loopback. OIDC is planned, **not implemented**. Restrict access with SSH forwarding or private network boundaries until authentication ships.
