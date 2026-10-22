# Upstream resolver settings

SQLite-backed management of the ordered list of public DNS resolvers. Only numeric IP:port addresses are accepted; duplicate or empty configurations are rejected. On a successful update, the management layer persists the configuration first and publishes a new immutable snapshot to the DNS engine, invalidating the old response cache.

External daemons are not required. Upstreams are persisted in the same SQLite file as records, using a distinct table and Go bounded context. DNS queries read snapshots only, never SQLite. DoH/DoT and health checks remain future work.
