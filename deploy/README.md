# Deploy QuillDNS with Docker

The published image includes **Go DNS resolver + Vue web UI + SQLite + observability**. It does not need Redis, a database container, Node.js, Bun or an external metrics server.

> **Status:** QuillDNS remains pre-release. Deploy only on trusted networks until protocol/security testing is complete. The public image should be used with the same caution.

## Quick start (Linux)

No Go/Bun checkout or Docker build is needed. The default binds DNS and the management UI to **127.0.0.1**, not to the public internet.

```bash
mkdir quilldns && cd quilldns
curl -fsSLo compose.yaml https://raw.githubusercontent.com/anxuanzi/QuillDNS/main/deploy/compose.yaml
docker compose up -d
```

Open **http://127.0.0.1:8080** on the Docker host and use `dig @127.0.0.1 -p 1053 example.org`.

The image is `ghcr.io/anxuanzi/quilldns:edge` (newest verified main build). Versioned tags appear when a `vX.Y.Z` release is published. If the package cannot be pulled anonymously, its GHCR package owner must set **Package settings → Change visibility → Public**. GitHub currently creates new packages as private unless made public.

### Docker run alternative

```bash
docker run -d --name quilldns --restart unless-stopped \
  --network host \
  --read-only --security-opt no-new-privileges:true --cap-drop ALL \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -v quilldns_data:/data \
  ghcr.io/anxuanzi/quilldns:edge
```

Because the image contains defaults for `QUILLDNS_DATA_PATH`, DNS, UI and upstreams, no environment flags are necessary for the safe local setup. On Docker Desktop, host networking depends on the Docker Desktop version and settings; this example targets Linux hosts.

## Environment variables

All DNS server settings accept environment variables. **Explicit CLI flags win over environment variables**, followed by safe built-in defaults. This keeps existing `quilldns -dns ...` usage working.

| Variable | Container default | Description |
| --- | --- | --- |
| `QUILLDNS_DATA_PATH` | `/data/quilldns.db` | SQLite config database (query logs live alongside it) |
| `QUILLDNS_DNS_LISTEN` | `127.0.0.1:1053` | DNS UDP/TCP bind address |
| `QUILLDNS_HTTP_LISTEN` | `127.0.0.1:8080` | Embedded Vue UI + REST API bind address |
| `QUILLDNS_UPSTREAMS` | `1.1.1.1:53,8.8.8.8:53` | Comma-separated public DNS upstreams |
| `QUILLDNS_ALLOW_NETWORKS` | `127.0.0.0/8,::1/128` | Allowed DNS client CIDRs |

Optional OIDC (all values must be configured consistently):

- `QUILLDNS_OIDC_ISSUER`
- `QUILLDNS_OIDC_CLIENT_ID`
- `QUILLDNS_OIDC_CLIENT_SECRET`
- `QUILLDNS_PUBLIC_URL`
- `QUILLDNS_OIDC_ALLOWED_EMAILS`

`QUILLDNS_IMAGE_TAG` is a **Docker Compose interpolation variable**, not a QuillDNS server setting. It controls the image tag to pull.

Copy [`.env.example`](.env.example) to `deploy/.env` for changes, then use `docker compose -f deploy/compose.yaml up -d` from the repository root, or `cd deploy && docker compose up -d`. Do not commit real secrets. Docker Compose automatically reads the local `.env` from its project directory.

### LAN DNS

Bind only to a trusted private interface and allow only your clients:

```dotenv
QUILLDNS_DNS_LISTEN=192.168.10.10:1053
QUILLDNS_ALLOW_NETWORKS=192.168.10.0/24,127.0.0.0/8,::1/128
```

Host networking exposes the listener on the **host** address you select. Add host firewall rules. For standard DNS port 53, a non-root process may require an explicitly granted `NET_BIND_SERVICE` capability or a firewall/port-forwarding rule from 53 to 1053; do not grant privileged execution or turn on unrestricted client access just to use port 53.

### Remote management / OIDC

QuillDNS deliberately rejects an unauthenticated management listener bound to `0.0.0.0` or another non-loopback interface. For remote admin access, configure OIDC, put a trusted HTTPS reverse proxy in front of the management API, and use an allowlist of permitted identities. See [authentication](../internal/auth/README.md).

## Upgrades and rollback

```bash
docker compose pull
docker compose up -d
docker compose logs -f
```

For predictable upgrades, use an explicit image tag (for example `QUILLDNS_IMAGE_TAG=1.0.0`) or pin a verified digest rather than relying on `edge`. A named volume `quilldns_data` retains SQLite databases across restarts and upgrades. The **database schema and app compatibility** still need to be checked before downgrading; take a consistent backup before major version changes.

Do not copy a running SQLite database file alone without its WAL state. Use SQLite's backup facility or stop the container before backing up the named volume.

## Image publishing

CI builds and tests the Go binary, embedded Vue UI, Docker runtime and live browser, then uses GitHub's `GITHUB_TOKEN` to push a multi-platform image to GHCR:

| Git ref | Image tags |
| --- | --- |
| `main` | `edge`, `sha-<full commit SHA>` |
| `v1.2.3` | `1.2.3`, `1.2`, `sha-<full commit SHA>`, `latest` |
| `v1.2.3-rc.1` | `1.2.3-rc.1`, `sha-<full commit SHA>` (no `latest`) |

Architectures: `linux/amd64` and `linux/arm64`. A GitHub package's public visibility is a package-owner setting; package publishing cannot guarantee that anonymous pulls are enabled. This is an infrastructure packaging feature, not a claim that QuillDNS has completed every production-readiness gate.

### Cross-platform build details

The Dockerfile runs Bun and the Go compiler on the native Buildx **build platform**, then cross-compiles the CGO-free Go binary for each **target platform**. The same workflow publishes Linux AMD64 and ARM64 images without executing build tools under QEMU. The OCI manifest is inspected after pushing.
