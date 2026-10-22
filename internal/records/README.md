# Records bounded context

Validated, immutable, type-specific local DNS record snapshots. Read requests avoid locks and database access; writes persist transactionally to an embedded SQLite database in WAL mode, then atomically publish a new in-memory snapshot. Domains own DNS semantics; neither HTTP nor the UI implements record invariants.

## Supported types
A, AAAA, CNAME, TXT, MX, SRV, PTR and CAA. The same name may have multiple distinct RRsets; a CNAME cannot coexist with other record types at the same enabled owner. TXT values are encoded into DNS character strings.

A local answer overrides the public upstream for that *name and type*. Other types at the same name still forward publicly. Wildcards and full authoritative DNS zones require dedicated semantics and are not supported in this release.

## Storage
The `--data` parameter is an SQLite database path, by default `./data/quilldns.db`. Do not point it at the obsolete pre-SQLite JSON file. Run backup commands using SQLite's backup API or stop the service before copying the database with its WAL files.
