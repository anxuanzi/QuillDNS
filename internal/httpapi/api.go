// Package httpapi exposes the management interface without implementing DNS rules.
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/anxuanzi/QuillDNS/internal/settings"
	"github.com/miekg/dns"
)

type API struct {
	Records   *records.Store
	DNS       *dnsserver.Server
	Telemetry *observability.Collector
	Settings  *settings.Store

	started time.Time
}

func (a *API) Handler() http.Handler {
	a.started = time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ok", "version": "dev"})
	})
	mux.HandleFunc("GET /api/v1/records", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"data": a.Records.List()})
	})
	mux.HandleFunc("POST /api/v1/records", a.create)
	mux.HandleFunc("PUT /api/v1/records/{id}", a.update)
	mux.HandleFunc("DELETE /api/v1/records/{id}", a.delete)
	mux.HandleFunc("GET /api/v1/metrics/summary", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.Telemetry.Summary()) })
	mux.HandleFunc("GET /api/v1/metrics/trend", a.trend)
	mux.HandleFunc("GET /api/v1/metrics/tops", a.tops)
	mux.HandleFunc("GET /api/v1/metrics/resolver", a.resolverHealth)
	mux.HandleFunc("GET /api/v1/queries", a.queries)
	mux.HandleFunc("GET /api/v1/queries/export", a.exportQueries)
	mux.HandleFunc("POST /api/v1/dns/resolve", a.lookup)
	mux.HandleFunc("GET /api/v1/upstreams", a.getUpstreams)
	mux.HandleFunc("PUT /api/v1/upstreams", a.putUpstreams)
	mux.HandleFunc("GET /metrics", a.prometheusMetrics)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Method != "GET" && r.Method != "HEAD" {
				origin := r.Header.Get("Origin")
				if origin != "" {
					u, err := url.Parse(origin)
					if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host != r.Host {
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
	if err := dec.Decode(&trailing); err != io.EOF {
		problem(w, 400, "only one JSON object is allowed")
		return false
	}
	return true
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var v records.Record
	if !decode(w, r, &v) {
		return
	}
	if v.ID != "" {
		problem(w, 400, "ID is server-generated")
		return
	}
	id, err := newID()
	if err != nil {
		problem(w, 500, "unable to create record ID")
		return
	}
	v.ID = id
	if v.TTL == 0 {
		v.TTL = 300
	}
	v.Enabled = true
	a.save(w, v, 201)
}
func (a *API) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	for _, v := range a.Records.List() {
		if v.ID == id {
			found = true
			break
		}
	}
	if !found {
		problem(w, 404, "record not found")
		return
	}
	var v records.Record
	if !decode(w, r, &v) {
		return
	}
	if v.ID != "" && v.ID != id {
		problem(w, 400, "ID mismatch")
		return
	}
	v.ID = id
	a.save(w, v, 200)
}
func (a *API) save(w http.ResponseWriter, v records.Record, status int) {
	if err := a.Records.Save(v); err != nil {
		code := 400
		if errors.Is(err, records.ErrConflict) {
			code = 409
		}
		problem(w, code, err.Error())
		return
	}
	respond(w, status, v)
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
	started := time.Now()
	result, err := a.DNS.Lookup(r.Context(), name, qtype)
	if err != nil {
		problem(w, 502, "DNS lookup failed")
		return
	}
	answers := make([]string, 0, len(result.Answer))
	for _, rr := range result.Answer {
		answers = append(answers, rr.String())
	}
	respond(w, 200, map[string]any{"rcode": dns.RcodeToString[result.Rcode], "answers": answers, "duration_ms": float64(time.Since(started).Microseconds()) / 1000})
}
func ListenAddress() string { return net.JoinHostPort("127.0.0.1", "8080") }

var _ = fmt.Sprintf

func (a *API) getUpstreams(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]any{"data": a.DNS.UpstreamAddresses()})
}
func (a *API) putUpstreams(w http.ResponseWriter, r *http.Request) {
	if a.Settings == nil {
		problem(w, 503, "upstream configuration unavailable")
		return
	}
	var req struct {
		Addresses []string `json:"addresses"`
	}
	if !decode(w, r, &req) {
		return
	}
	normalized, err := settings.ValidateUpstreams(req.Addresses)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	if err = a.Settings.SaveUpstreams(normalized); err != nil {
		problem(w, 500, "could not persist upstreams")
		return
	}
	if err = a.DNS.SetUpstreams(normalized); err != nil {
		problem(w, 500, "could not apply upstreams")
		return
	}
	respond(w, 200, map[string]any{"data": normalized})
}
