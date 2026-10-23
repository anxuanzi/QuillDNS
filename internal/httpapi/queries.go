package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/observability"
)

// exportRowLimit bounds how many history rows a single CSV download may contain.
const exportRowCap = 50_000

// parseQueryFilter decodes the shared query-history filters. Every filter is
// a bound matcher; unknown values simply match nothing rather than failing.
func parseQueryFilter(w http.ResponseWriter, r *http.Request) (observability.QueryFilter, bool) {
	values := r.URL.Query()
	filter := observability.QueryFilter{
		Q:      strings.TrimSpace(values.Get("q")),
		Name:   strings.TrimSpace(values.Get("name")),
		Client: strings.TrimSpace(values.Get("client")),
		Source: strings.TrimSpace(values.Get("source")),
		Rcode:  strings.ToUpper(strings.TrimSpace(values.Get("rcode"))),
		Type:   strings.ToUpper(strings.TrimSpace(values.Get("type"))),
	}
	sortBy := values.Get("sort")
	if sortBy != "" && sortBy != "asc" && sortBy != "desc" {
		problem(w, 400, "sort must be asc or desc")
		return filter, false
	}
	filter.Sort = sortBy
	since, err := parseTimeParam(values.Get("since"))
	if err != nil {
		problem(w, 400, err.Error())
		return filter, false
	}
	filter.Since = since
	until, err := parseTimeParam(values.Get("until"))
	if err != nil {
		problem(w, 400, err.Error())
		return filter, false
	}
	filter.Until = until
	if cursor := values.Get("cursor"); cursor != "" {
		parsed, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || parsed < 1 {
			problem(w, 400, "cursor must be a positive row id")
			return filter, false
		}
		filter.Cursor = parsed
	}
	return filter, true
}

// parseTimeParam accepts RFC3339 timestamps only; empty means unbounded.
func parseTimeParam(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp %q must use RFC3339 format", value)
	}
	return parsed, nil
}

// queries serves server-side paginated, filtered, searchable query history.
// Pagination is keyset-based on the row id: pass next_cursor from the previous
// response to continue the walk, which stays fast no matter how deep the log.
func (a *API) queries(w http.ResponseWriter, r *http.Request) {
	filter, ok := parseQueryFilter(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()
	filter.Limit = observability.MaxPageSize // clamped to the default below
	if limit := values.Get("limit"); limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err != nil || parsed < 1 || parsed > observability.MaxPageSize {
			problem(w, 400, fmt.Sprintf("limit must be between 1 and %d", observability.MaxPageSize))
			return
		}
		filter.Limit = parsed
	} else {
		filter.Limit = 100
	}
	events, hasMore, err := a.Telemetry.Search(r.Context(), filter)
	if err != nil {
		problem(w, 503, "query history unavailable")
		return
	}
	page := map[string]any{"data": events}
	if hasMore && len(events) > 0 {
		page["next_cursor"] = events[len(events)-1].ID
	}
	if values.Get("count") != "" {
		total, err := a.Telemetry.Count(r.Context(), filter)
		if err != nil {
			problem(w, 503, "query history unavailable")
			return
		}
		page["total"] = total
	}
	respond(w, 200, page)
}

// exportQueries streams the filtered history as a bounded CSV download.
func (a *API) exportQueries(w http.ResponseWriter, r *http.Request) {
	filter, ok := parseQueryFilter(w, r)
	if !ok {
		return
	}
	events, err := a.Telemetry.Export(r.Context(), filter, exportRowCap)
	if err != nil {
		problem(w, 503, "query history unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="quilldns-queries.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write([]string{"time", "name", "type", "client", "source", "rcode", "duration_ms"})
	for _, e := range events {
		if err := csvWriter.Write([]string{
			e.Time.UTC().Format(time.RFC3339Nano),
			e.Name,
			e.Type,
			e.Client,
			e.Source,
			e.Rcode,
			strconv.FormatFloat(e.DurationMS, 'f', -1, 64),
		}); err != nil {
			return
		}
	}
	csvWriter.Flush()
}
