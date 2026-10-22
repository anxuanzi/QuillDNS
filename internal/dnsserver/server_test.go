package dnsserver

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/miekg/dns"
)

type fakeUpstream struct{ calls int }

func (f *fakeUpstream) ExchangeContext(_ context.Context, req *dns.Msg, _ string) (*dns.Msg, time.Duration, error) {
	f.calls++
	r := new(dns.Msg)
	r.SetReply(req)
	r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: net.IPv4(8, 8, 8, 8)}}
	return r, 0, nil
}
func TestLocalFallbackAndCache(t *testing.T) {
	store, _ := records.Open(filepath.Join(t.TempDir(), "records.json"))
	if err := store.Save(records.Record{ID: "one", Name: "api.example", Type: "A", Value: "10.1.2.3", TTL: 60, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	srv := New(store, observability.New(10), []string{"127.0.0.1:53"})
	fake := &fakeUpstream{}
	srv.Client = fake
	m := new(dns.Msg)
	m.SetQuestion("api.example.", dns.TypeA)
	answer, source := srv.resolve(context.Background(), m)
	if source != "local" || answer.Answer[0].(*dns.A).A.String() != "10.1.2.3" {
		t.Fatalf("%s %v", source, answer)
	}
	m.SetQuestion("example.org.", dns.TypeA)
	_, source = srv.resolve(context.Background(), m)
	if source != "upstream" {
		t.Fatal(source)
	}
	_, source = srv.resolve(context.Background(), m)
	if source != "cache" || fake.calls != 1 {
		t.Fatalf("source %s calls %d", source, fake.calls)
	}
}
