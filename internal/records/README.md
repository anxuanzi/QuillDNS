# Records domain

This domain owns override validation, RR construction, conflict detection, persistence, and immutable indexed snapshots. Neither the DNS server nor the HTTP API can mutate its internal state directly.

## Semantics
- Lookups are normalized to absolute, lowercase DNS names.
- Matching is type-specific. If a hostname has a local A but no TXT, TXT falls back to upstream.
- An existing CNAME is returned for requests for other types; it cannot coexist with any other RR at its owner name.
- Multiple A/AAAA/MX/TXT/SRV records may coexist at one name.
- Every mutation validates a complete snapshot and persists it via same-directory atomic rename before publication.
- Restart loads the last persisted valid snapshot. The DNS hot path uses only immutable in-memory data.
- Initial storage is an embedded JSON file, not SQLite. This is intentional for the first vertical slice; migration can be added behind this bounded context.

## Limitations
No wildcard semantics, per-client policies, zone authority, DNSSEC local signing, or distributed synchronization yet. TXT strings over 255 bytes are split into DNS character strings.
