# Optional OpenID Connect authentication

This package is the control-plane authorization boundary. It is independent of DNS UDP/TCP resolution.

When all OIDC settings are configured, QuillDNS discovers the identity provider, uses Authorization Code + PKCE (S256), validates ID tokens including issuer/audience/nonce, and creates an opaque, HttpOnly, SameSite=Lax browser session with an eight-hour expiration. Cookies are Secure on HTTPS. Session secrets are stored only as SHA-256 digests in memory. Login state and sessions expire and are never written to disk.

If OIDC is disabled, the management listener must bind to loopback; no public unauthenticated GUI is permitted. For remote administration, enable OIDC and terminate HTTPS at a trusted reverse proxy. The public URL must match the externally visible HTTPS hostname.

Set all four environment variables: `QUILLDNS_OIDC_ISSUER`, `QUILLDNS_OIDC_CLIENT_ID`, `QUILLDNS_OIDC_CLIENT_SECRET`, and `QUILLDNS_PUBLIC_URL`. Set `QUILLDNS_OIDC_ALLOWED_EMAILS` to a comma-separated, explicitly permitted list if your provider has other users. If the allowlist is empty, any identity accepted by the configured provider can sign in.

**Known limitations:** Sessions are in-memory and are lost on restart; a restart requires login again. There is no RBAC, refresh-token persistence, session management UI, or external authorization policy. A trusted reverse proxy must enforce HTTPS and route the correct Host header. Keep the management endpoint private until deployment-specific security review is complete.
