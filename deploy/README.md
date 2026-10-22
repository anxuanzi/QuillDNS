# Packaging and operations

The multi-stage Dockerfile builds the Vite SPA, then compiles a CGO-free Go executable containing all UI assets. The runtime image uses a non-root distroless base.

The Compose example uses host networking on Linux and is intentionally loopback-only. To serve a LAN, choose an appropriate private interface for `-dns`, list specific client CIDRs, and allow UDP/TCP port 53 through the host firewall. For remote UI access, configure OIDC, terminate HTTPS at a trusted reverse proxy, and change the HTTP listener only after validating the authentication flow.

Before production:
- Pin container base images by digest and generate signed release artifacts.
- Test DNS and HTTPS proxy networking in the target environment.
- Monitor persistent SQLite storage and log drop counters.
- Back up databases consistently with a SQLite backup operation or a stopped service, including WAL state.
- Do not rely on the local example as a hardened production deployment.
