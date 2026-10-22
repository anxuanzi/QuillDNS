// Package httpapi exposes the management interface, without owning DNS business rules.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/miekg/dns"
)

type API struct {
	Records   *records.Store
	DNS       *dnsserver.Server
	Telemetry *observability.Collector
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ok", "version": "dev"})
	})
	mux.HandleFunc("GET /api/v1/records", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"data": a.Records.List()})
	})
	mux.HandleFunc("POST /api/v1/records", a.create)
	mux.HandleFunc("DELETE /api/v1/records/{id}", a.delete)
	mux.HandleFunc("GET /api/v1/metrics/summary", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.Telemetry.Summary()) })
	mux.HandleFunc("GET /api/v1/queries", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"data": a.Telemetry.Events(100)})
	})
	mux.HandleFunc("POST /api/v1/dns/resolve", a.lookup)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// No credentials are sent cross-origin; this also prevents browser-based
			// cross-site mutations when deploying without OIDC.
			if r.Method != "GET" && r.Method != "HEAD" {
				origin := r.Header.Get("Origin")
				if origin != "" {
					u, err := url.Parse(origin)
					if err != nil || u.Host != r.Host {
						problem(w, 403, "cross-origin requests are not permitted")
						return
					}
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]any{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		problem(w, 400, "invalid JSON payload")
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		problem(w, 400, "only one JSON object is allowed")
		return false
	}
	return true
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var v records.Record
	if !decode(w, r, &v) {
		return
	}
	if v.ID == "" {
		v.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if v.TTL == 0 {
		v.TTL = 300
	}
	v.Enabled = true
	if err := a.Records.Save(v); err != nil {
		status := 400
		if errors.Is(err, records.ErrConflict) {
			status = 409
		}
		problem(w, status, err.Error())
		return
	}
	respond(w, 201, v)
}
func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	err := a.Records.Delete(r.PathValue("id"))
	if errors.Is(err, records.ErrNotFound) {
		problem(w, 404, "record not found")
		return
	}
	if err != nil {
		problem(w, 500, "record deletion failed")
		return
	}
	w.WriteHeader(204)
}
func (a *API) lookup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if !decode(w, r, &input) {
		return
	}
	name, err := records.Normalize(input.Name)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	qtype, ok := dns.StringToType[strings.ToUpper(input.Type)]
	if !ok {
		problem(w, 400, "unsupported DNS type")
		return
	}
	result, err := a.DNS.Lookup(r.Context(), name, qtype)
	if err != nil {
		problem(w, 502, "DNS lookup failed")
		return
	}
	answers := make([]string, 0, len(result.Answer))
	for _, rr := range result.Answer {
		answers = append(answers, rr.String())
	}
	respond(w, 200, map[string]any{"rcode": dns.RcodeToString[result.Rcode], "answers": answers})
}

// ListenAddress enforces a private loopback default when no authentication exists.
func ListenAddress() string { return net.JoinHostPort("127.0.0.1", "8080") }
