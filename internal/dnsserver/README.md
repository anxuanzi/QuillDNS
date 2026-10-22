# DNS resolution domain

The Go DNS server uses `miekg/dns` for UDP/TCP and an in-memory RRset index. Resolution: local exact override/CNAME → bounded positive cache → ordered public DNS upstreams. The query handler never uses SQL or filesystem I/O.

## Access restrictions
All DNS clients are denied unless their IP address is in `--allow-networks`. Defaults are `127.0.0.0/8,::1/128`. Change both the DNS bind address and the CIDRs for private infrastructure use. Do not allow `0.0.0.0/0` or `::/0` on a publicly reachable endpoint.

## DNS behavior and remaining limitations
- Incoming UDP and TCP supported; truncated upstream UDP queries retry with TCP.
- Only plain UDP/TCP upstreams are available. Health-aware upstream scoring, DoT, DoH and DoQ remain future work.
- Locally synthetic answers are not marked DNSSEC authenticated.
- EDNS and DNSSEC-sensitive upstream requests bypass the basic positive cache.
- TTL-aware positive cache is bounded. Negative caching is not yet supported.
- Wildcards, DNSSEC policy, detailed CNAME resolution, retry classification and additional RFC edge cases need further integration tests.
