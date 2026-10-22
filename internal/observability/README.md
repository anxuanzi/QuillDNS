# Observability domain

Integrated, bounded query event ring and aggregate counters. No Prometheus, Grafana, or external runtime dependencies. DNS request handling never waits on disk I/O for logs.

Events are held in memory in this initial slice and are lost on restart. Persistent retention, interval metrics, histograms, and privacy controls are follow-up work. API consumers must not mistake these in-memory counts for lifetime totals.
