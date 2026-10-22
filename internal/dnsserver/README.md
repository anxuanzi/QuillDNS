# DNS resolution domain

Go DNS server using `miekg/dns` for UDP/TCP. Algorithm: local exact RRset (or CNAME) → positive response cache → ordered public upstream fallback. Replies identify local/cache/upstream origins for integrated observability.

## Limits of this foundation
- Upstreams are plain DNS UDP/TCP; DoH/DoT, health scoring, and encrypted transport are future work.
- Positive cache is bounded and only used for queries without EDNS and CD; negative caching is not implemented.
- DNSSEC-validating semantics, full CNAME synthesis, wildcard policies, client access rules, and robust truncation across all edge cases require further implementation/testing before internet-facing operation.
- Use only on a trusted private network in this initial release.
