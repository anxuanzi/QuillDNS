package dnsserver

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/miekg/dns"
)

// countingUpstream answers every query with one A record, recording how many
// exchanges each upstream address received.
type countingUpstream struct {
	calls   map[string]int
	failFor map[string]bool
	delay   map[string]time.Duration
}

func (u *countingUpstream) ExchangeContext(ctx context.Context, req *dns.Msg, addr string) (*dns.Msg, time.Duration, error) {
	u.calls[addr]++
	if u.failFor[addr] {
		return nil, 0, errors.New("connection refused")
	}
	if delay := u.delay[addr]; delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	response := new(dns.Msg)
	response.SetReply(req)
	header := dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}
	response.Answer = []dns.RR{&dns.A{Hdr: header, A: net.IPv4(192, 0, 2, 1)}}
	return response, 0, nil
}

func newTestServer(t *testing.T, upstream Upstream, upstreams []string, maxCache int) *Server {
	t.Helper()
	store, err := records.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s := &Server{Records: store, Logs: observability.New(16), Client: upstream, Timeout: 500 * time.Millisecond, cache: make(map[string]cacheEntry), MaxCache: maxCache, upstreamStats: make(map[string]*upstreamStat)}
	return s.withUpstreams(upstreams)
}

func query(name string) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), dns.TypeA)
	req.RecursionDesired = true
	return req
}

func (s *Server) withUpstreams(upstreams []string) *Server {
	s.upstreamSet.Store(append([]string(nil), upstreams...))
	return s
}

func TestCacheCountersTrackHitsMissesAndSize(t *testing.T) {
	upstream := &countingUpstream{calls: make(map[string]int), failFor: make(map[string]bool)}
	s := newTestServer(t, upstream, []string{"10.0.0.1:53"}, 10)
	ctx := context.Background()
	want := []string{"upstream", "cache", "cache"}
	for i, expected := range want {
		if _, source := s.resolve(ctx, query("one.example.test")); source != expected {
			t.Fatalf("attempt %d: expected %q, got %q", i, expected, source)
		}
	}
	stats := s.CacheStats()
	if stats.Entries != 1 || stats.Misses != 1 || stats.Hits != 2 {
		t.Fatalf("unexpected cache stats: %+v", stats)
	}
	if upstream.calls["10.0.0.1:53"] != 1 {
		t.Fatalf("expected single upstream exchange, got %d", upstream.calls["10.0.0.1:53"])
	}
}

func TestCacheStaysBoundedUnderPressure(t *testing.T) {
	upstream := &countingUpstream{calls: make(map[string]int), failFor: make(map[string]bool)}
	s := newTestServer(t, upstream, []string{"10.0.0.1:53"}, 2)
	ctx := context.Background()
	for i := range 20 {
		name := string(rune('a'+i%26)) + "-" + string(rune('a'+(i/26)%26)) + ".example.test"
		if _, source := s.resolve(ctx, query(name)); source != "upstream" {
			t.Fatalf("query %d: unexpected source %q", i, source)
		}
	}
	stats := s.CacheStats()
	if stats.Entries > stats.Max {
		t.Fatalf("cache exceeded its bound: %d > %d", stats.Entries, stats.Max)
	}
	if stats.Evictions == 0 {
		t.Fatal("expected pressure evictions to be counted")
	}
}

func TestUpstreamBreakerSkipsRepeatedFailures(t *testing.T) {
	upstream := &countingUpstream{calls: make(map[string]int), failFor: map[string]bool{"10.9.9.9:53": true}}
	s := newTestServer(t, upstream, []string{"10.9.9.9:53", "10.0.0.1:53"}, 10)
	ctx := context.Background()
	// Breaker opens after three consecutive failures of the first upstream.
	for i := range 3 {
		if _, source := s.resolve(ctx, query("fail"+string(rune('a'+i))+".example.test")); source != "upstream" {
			t.Fatalf("attempt %d: expected upstream success through second resolver, got %q", i, source)
		}
	}
	if upstream.calls["10.9.9.9:53"] != 3 {
		t.Fatalf("expected 3 attempts at the failing upstream, got %d", upstream.calls["10.9.9.9:53"])
	}
	firstCalls := upstream.calls["10.0.0.1:53"]
	for i := range 5 {
		if _, source := s.resolve(ctx, query("skip"+string(rune('a'+i))+".example.test")); source != "upstream" {
			t.Fatalf("breaker attempt %d: unexpected source %q", i, source)
		}
	}
	if upstream.calls["10.9.9.9:53"] != 3 {
		t.Fatalf("breaker failed to skip failing upstream: %d calls", upstream.calls["10.9.9.9:53"])
	}
	if upstream.calls["10.0.0.1:53"] != firstCalls+5 {
		t.Fatalf("healthy upstream should serve breaker-window queries: %d", upstream.calls["10.0.0.1:53"])
	}
	health := s.UpstreamHealth()
	if health[0].Status != "cooling" || health[0].Errors != 3 || health[1].Status != "healthy" {
		t.Fatalf("unexpected health snapshot: %+v", health)
	}
}

func TestUpstreamHealthRecordsLatencyAndErrors(t *testing.T) {
	upstream := &countingUpstream{calls: make(map[string]int), failFor: map[string]bool{"10.9.9.9:53": true}, delay: map[string]time.Duration{"10.0.0.1:53": 10 * time.Millisecond}}
	s := newTestServer(t, upstream, []string{"10.9.9.9:53", "10.0.0.1:53"}, 10)
	ctx := context.Background()
	if _, source := s.resolve(ctx, query("latency.example.test")); source != "upstream" {
		t.Fatalf("unexpected source %q", source)
	}
	health := s.UpstreamHealth()
	var failing, healthy *UpstreamHealth
	for i := range health {
		switch health[i].Address {
		case "10.9.9.9:53":
			failing = &health[i]
		case "10.0.0.1:53":
			healthy = &health[i]
		}
	}
	if failing == nil || healthy == nil {
		t.Fatalf("missing upstream health entries: %+v", health)
	}
	if failing.Errors != 1 || failing.Requests != 1 || failing.LastError == "" || failing.LastErrorAt == nil {
		t.Fatalf("failing upstream not recorded: %+v", failing)
	}
	if healthy.Requests != 1 || healthy.Errors != 0 || healthy.AvgLatencyMS < 5 {
		t.Fatalf("healthy upstream latency not recorded: %+v", healthy)
	}
	if _, source := s.resolve(ctx, query("more.example.test")); source != "upstream" {
		t.Fatalf("unexpected source %q", source)
	}
	if s.CacheStats().Misses != 2 {
		t.Fatalf("expected 2 cache misses, got %+v", s.CacheStats())
	}
}
