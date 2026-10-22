package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
)

func fixture(t *testing.T) http.Handler {
	t.Helper()
	s, err := records.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	o := observability.New(10)
	return (&API{Records: s, DNS: dnsserver.New(s, o, nil), Telemetry: o}).Handler()
}
func TestRecordLifecycle(t *testing.T) {
	h := fixture(t)
	body := []byte(`{"name":"api.example","type":"A","value":"10.0.0.2","ttl":60}`)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/records", bytes.NewReader(body)))
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	var created records.Record
	if err := json.Unmarshal(r.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/records", nil))
	if r.Code != 200 || !bytes.Contains(r.Body.Bytes(), []byte("api.example.")) {
		t.Fatalf("list: %s", r.Body.String())
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("PUT", "/api/v1/records/"+created.ID, bytes.NewBufferString(`{"name":"api.example","type":"A","value":"10.0.0.4","ttl":60,"enabled":true}`)))
	if r.Code != 200 {
		t.Fatalf("update: %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("DELETE", "/api/v1/records/"+created.ID, nil))
	if r.Code != 204 {
		t.Fatalf("delete: %d", r.Code)
	}
}
func TestRejectInvalidAddressAndCrossOrigin(t *testing.T) {
	h := fixture(t)
	req := httptest.NewRequest("POST", "/api/v1/records", bytes.NewBufferString(`{"name":"x.example","type":"A","value":"999.2.3.4","ttl":60}`))
	r := httptest.NewRecorder()
	h.ServeHTTP(r, req)
	if r.Code != 400 {
		t.Fatalf("invalid address: %d", r.Code)
	}
	req = httptest.NewRequest("POST", "/api/v1/records", bytes.NewBufferString("{}"))
	req.Header.Set("Origin", "https://attacker.example")
	r = httptest.NewRecorder()
	h.ServeHTTP(r, req)
	if r.Code != 403 {
		t.Fatalf("cross-origin: %d", r.Code)
	}
}
func TestRejectInvalidPayloadAndClientIDs(t *testing.T) {
	h := fixture(t)
	cases := []string{
		`{"id":"admin","name":"x.example","type":"A","value":"10.0.0.1"}`,
		`{"name":"x.example","type":"A","value":"10.0.0.1"}{}`,
		`{"name":"x.example","type":"A","value":"10.0.0.1","ttl":60,"unexpected":true}`,
	}
	for _, payload := range cases {
		req := httptest.NewRequest("POST", "/api/v1/records", bytes.NewBufferString(payload))
		r := httptest.NewRecorder()
		h.ServeHTTP(r, req)
		if r.Code != 400 {
			t.Errorf("payload %s returned %d", payload, r.Code)
		}
	}
}
