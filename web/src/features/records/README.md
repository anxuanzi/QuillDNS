# DNS record management UI

This domain owns record listing, text search, adding, editing, deleting and conditional fields by RR type. Server state belongs to Pinia Colada. The Go records bounded context remains authoritative for validation and conflict prevention.

CRUD supports A, AAAA, CNAME, TXT, MX, SRV and PTR. Wildcard records, bulk import/export, CAA and more complex DNS service records remain planned. The API persists records before replacing the immutable in-memory lookup snapshot.
