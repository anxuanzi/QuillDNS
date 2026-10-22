# Security policy

QuillDNS is infrastructure software. Treat DNS records, client request history, and management credentials as sensitive.

## Supported versions

QuillDNS is **pre-release** and has no designated security-supported stable version yet. Do not deploy an unaudited build as an unrestricted public recursive resolver.

## Reporting vulnerabilities

Please use [GitHub's private vulnerability reporting](https://github.com/anxuanzi/QuillDNS/security/advisories) if available for this repository. Avoid filing public issues containing exploit details, access tokens, IP addresses, user records, or other confidential information.

## Deployment security

- Management without OIDC must remain bound to loopback. Access remotely through trusted private tunnels.
- When OIDC is enabled, configure HTTPS termination, a matching public URL, a trusted identity provider, and preferably an email allowlist.
- DNS client access must be restricted to trusted networks using both `--allow-networks` and firewall policies.
- Never use `0.0.0.0/0` or `::/0` on an internet-reachable recursive resolver.
- Protect SQLite database files and backups. Query history contains client identifiers and queried hostnames.
- Keep dependencies updated and review `go.mod`, `go.sum`, and the frontend lockfile when upgrading.
- Do not treat the current positive cache as a validating DNSSEC resolver.

Security hardening, identity-provider integration tests, and protocol fuzzing are required before declaring a stable public release.
