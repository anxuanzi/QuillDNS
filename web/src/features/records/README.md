# DNS records UI

The records feature owns the table, mutation form, and transport-facing view state. Pinia Colada owns server state. All validation and business invariants are enforced again by the Go records domain.

The initial interface supports create/delete for A, AAAA, TXT, CNAME, PTR, MX and SRV records. Record edits, wildcards, CAA, bulk management, and record-set grouping remain follow-up work.
