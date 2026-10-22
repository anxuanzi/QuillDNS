package records

import (
	"github.com/miekg/dns"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		r  Record
		ok bool
	}{
		{Record{ID: "1", Name: "API.Example.COM", Type: "A", Value: "10.0.0.10", TTL: 60, Enabled: true}, true},
		{Record{Name: "api.example.com", Type: "A", Value: "999.1.2.3", TTL: 60}, false},
		{Record{Name: "api.example.com", Type: "TXT", Value: "verification=ok", TTL: 60}, true},
		{Record{Name: "api.example.com", Type: "SPF", Value: "x", TTL: 60}, false},
		{Record{Name: "bad name", Type: "A", Value: "10.0.0.1", TTL: 60}, false},
	}
	for _, tt := range tests {
		err := tt.r.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("%+v: got %v", tt.r, err)
		}
	}
}
func TestStoreLocalAndFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.sqlite")
 s, e := Open(path)
 defer s.Close()
	if e != nil {
		t.Fatal(e)
	}
	r := Record{ID: "1", Name: "API.Example.COM", Type: "A", Value: "10.0.0.10", TTL: 60, Enabled: true}
	if e = s.Save(r); e != nil {
		t.Fatal(e)
	}
	a, ok := s.Lookup("api.example.com.", dns.TypeA)
	if !ok || len(a) != 1 || a[0].(*dns.A).A.String() != "10.0.0.10" {
		t.Fatalf("unexpected A answer: %v", a)
	}
	if _, ok = s.Lookup("api.example.com", dns.TypeTXT); ok {
		t.Fatal("TXT must fall through")
	}
	s2, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s2.Close()
 if _, ok = s2.Lookup("api.example.com", dns.TypeA); !ok {
		t.Fatal("persistence failed")
	}
	if e = s2.Delete("1"); e != nil {
		t.Fatal(e)
	}
	if _, ok = s2.Lookup("api.example.com", dns.TypeA); ok {
		t.Fatal("delete failed")
	}
}
func TestCNAMEConflict(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "records.json"))
	if err := s.Save(Record{ID: "1", Name: "x.example", Type: "CNAME", Value: "target.example", TTL: 60, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Record{ID: "2", Name: "x.example", Type: "A", Value: "127.0.0.1", TTL: 60, Enabled: true}); err == nil {
		t.Fatal("accepted CNAME conflict")
	}
	if _, ok := s.Lookup("x.example", dns.TypeA); !ok {
		t.Fatal("failed transaction changed snapshot")
	}
}
