# Settings management

The settings page edits ordered public DNS upstreams and displays optional OIDC authentication status. Changes are validated, saved in SQLite and hot-applied to the Go resolver with response-cache invalidation.

OIDC is configured using server environment variables, never stored in browser local storage. When OIDC is disabled, the management service remains loopback-only.
