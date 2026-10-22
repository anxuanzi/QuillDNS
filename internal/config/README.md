# Process configuration

The `config` package defines QuillDNS's environment variables and compatible command-line flags. It is kept independent from DNS and HTTP domain packages so parsing can be tested without starting listeners or opening SQLite.

**Order of precedence:** an explicitly supplied CLI flag overrides the environment variable; otherwise a safe built-in default applies. An empty supplied value and invalid network address cause startup to fail with a clear error rather than starting with an unsafe or partial configuration.

| Environment variable | CLI flag | Default |
| --- | --- | --- |
| `QUILLDNS_DATA_PATH` | `-data` | `./data/quilldns.db` |
| `QUILLDNS_DNS_LISTEN` | `-dns` | `127.0.0.1:1053` |
| `QUILLDNS_HTTP_LISTEN` | `-http` | `127.0.0.1:8080` |
| `QUILLDNS_UPSTREAMS` | `-upstreams` | `1.1.1.1:53,8.8.8.8:53` |
| `QUILLDNS_ALLOW_NETWORKS` | `-allow-networks` | `127.0.0.0/8,::1/128` |

OIDC settings are deliberately separate secrets read by the authentication subsystem: `QUILLDNS_OIDC_ISSUER`, `QUILLDNS_OIDC_CLIENT_ID`, `QUILLDNS_OIDC_CLIENT_SECRET`, `QUILLDNS_PUBLIC_URL`, and `QUILLDNS_OIDC_ALLOWED_EMAILS`.

**Security:** Unauthenticated management may bind only to loopback addresses, regardless of flags or env values. Public DNS recursion is not enabled by default. Expanding DNS client networks must be intentional; firewall rules remain recommended. The default ports are unprivileged for simple non-root containers.

The runtime image overrides only `QUILLDNS_DATA_PATH` (to `/data/quilldns.db`) and otherwise preserves the same safe defaults.
