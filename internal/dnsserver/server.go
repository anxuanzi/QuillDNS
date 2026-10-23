// Package dnsserver implements local-first resolution and public DNS forwarding.
package dnsserver

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/miekg/dns"
)

type Upstream interface {
	ExchangeContext(context.Context, *dns.Msg, string) (*dns.Msg, time.Duration, error)
}
type cacheEntry struct {
	answer  *dns.Msg
	expires time.Time
	stored  time.Time
}

// upstreamStat tracks per-upstream resolver health for observability and the
// failure breaker. Every field is lock-free.
type upstreamStat struct {
	requests     atomic.Uint64
	errors       atomic.Uint64
	latencyNS    atomic.Int64
	consecFails  atomic.Int64
	blockedUntil atomic.Int64 // unix nanoseconds
	lastError    atomic.Value // string
	lastErrorAt  atomic.Int64 // unix nanoseconds
}

// CacheStats is a point-in-time snapshot of the resolver response cache.
type CacheStats struct {
	Entries   int    `json:"entries"`
	Max       int    `json:"max"`
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	Evictions uint64 `json:"evictions"`
}

// UpstreamHealth is a point-in-time snapshot of one upstream forwarder.
type UpstreamHealth struct {
	Address      string     `json:"address"`
	Requests     uint64     `json:"requests"`
	Errors       uint64     `json:"errors"`
	AvgLatencyMS float64    `json:"avg_latency_ms"`
	LatencySumMS float64    `json:"latency_sum_ms"`
	Status       string     `json:"status"`
	LastError    string     `json:"last_error,omitempty"`
	LastErrorAt  *time.Time `json:"last_error_at,omitempty"`
}

// Upstream failure breaker: after this many consecutive exchange errors an
// upstream is skipped until the cooldown elapses, so a dead forwarder stops
// taxing every client query with its full timeout.
const (
	upstreamBreakerThreshold = 3
	upstreamBreakerCooldown  = 30 * time.Second
)

type Server struct {
	Records     *records.Store
	Logs        *observability.Collector
	Client      Upstream
	upstreamSet atomic.Value
	Timeout     time.Duration
	mu          sync.Mutex
	cache       map[string]cacheEntry
	MaxCache    int
	Allow       []netip.Prefix

	cacheHits      atomic.Uint64
	cacheMisses    atomic.Uint64
	cacheEvictions atomic.Uint64
	upstreamMu     sync.RWMutex
	upstreamStats  map[string]*upstreamStat
}

func New(r *records.Store, o *observability.Collector, upstreams []string) *Server {
	s := &Server{Records: r, Logs: o, Client: &dns.Client{Net: "udp", Timeout: 2 * time.Second}, Timeout: 2 * time.Second, cache: make(map[string]cacheEntry), MaxCache: 10000, upstreamStats: make(map[string]*upstreamStat)}
	s.upstreamSet.Store(append([]string(nil), upstreams...))
	return s
}
func (s *Server) SetAllowedNetworks(networks string) error {
	prefixes := make([]netip.Prefix, 0)
	for part := range strings.SplitSeq(networks, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			return err
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	if len(prefixes) == 0 {
		return errors.New("at least one client CIDR required")
	}
	s.Allow = prefixes
	return nil
}
func (s *Server) permitted(addr net.Addr) bool {
	if len(s.Allow) == 0 {
		return false
	}
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, prefix := range s.Allow {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
func key(q dns.Question) string {
	return strings.ToLower(dns.Fqdn(q.Name)) + "|" + dns.TypeToString[q.Qtype] + "|" + dns.ClassToString[q.Qclass]
}
func respond(req *dns.Msg, rcode int) *dns.Msg {
	r := new(dns.Msg)
	r.SetRcode(req, rcode)
	r.RecursionAvailable = true
	return r
}

// ageTTLs subtracts the seconds a cached answer has been stored from every
// record's TTL across all response sections, floored at zero.
func ageTTLs(r *dns.Msg, stored time.Time) *dns.Msg {
	age := uint32(time.Since(stored) / time.Second)
	for _, section := range [][]dns.RR{r.Answer, r.Ns, r.Extra} {
		for _, rr := range section {
			h := rr.Header()
			if h.Ttl > age {
				h.Ttl -= age
			} else {
				h.Ttl = 0
			}
		}
	}
	return r
}
func (s *Server) resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, string) {
	if req == nil {
		return nil, "error"
	}
	if req.Opcode != dns.OpcodeQuery || len(req.Question) != 1 || req.Question[0].Qclass != dns.ClassINET {
		return respond(req, dns.RcodeFormatError), "error"
	}
	q := req.Question[0]
	if rr, ok := s.Records.Lookup(q.Name, q.Qtype); ok {
		response := respond(req, dns.RcodeSuccess)
		response.Answer = rr
		// Local DNS overrides are unsigned. Never convey upstream validation of synthetic answers.
		response.AuthenticatedData = false
		if edns := req.IsEdns0(); edns != nil {
			response.SetEdns0(edns.UDPSize(), false)
		}
		return response, "local"
	}
	// DNSSEC, EDNS client subnet, CD and nonstandard query flags must be forwarded intact.
	// Conservative cache behavior prevents mixing responses with different DNS semantics.
	cacheable := req.IsEdns0() == nil && !req.CheckingDisabled && !req.AuthenticatedData
	k := key(q)
	if cacheable {
		now := time.Now()
		s.mu.Lock()
		entry, found := s.cache[k]
		if found && now.Before(entry.expires) {
			answer, stored := entry.answer, entry.stored
			s.mu.Unlock()
			s.cacheHits.Add(1)
			r := ageTTLs(answer.Copy(), stored)
			r.Id = req.Id
			r.RecursionDesired = req.RecursionDesired
			return r, "cache"
		}
		if found {
			delete(s.cache, k)
			s.cacheEvictions.Add(1)
		}
		s.mu.Unlock()
		s.cacheMisses.Add(1)
	}
	var response *dns.Msg
	for _, addr := range s.UpstreamAddresses() {
		if stat := s.upstreamStatFor(addr); stat != nil && time.Now().UnixNano() < stat.blockedUntil.Load() {
			continue // breaker open; the cooldown expired upstreams are retried first
		}
		started := time.Now()
		attempt, err := s.exchangeUpstream(ctx, addr, req)
		s.recordUpstream(addr, attempt, err, time.Since(started))
		if attempt != nil && attempt.Id == req.Id {
			response = attempt
			break
		}
	}
	if response == nil {
		return respond(req, dns.RcodeServerFailure), "error"
	}
	if cacheable && response.Rcode == dns.RcodeSuccess && len(response.Answer) > 0 && !response.Truncated {
		ttl := uint32(300)
		for _, section := range [][]dns.RR{response.Answer, response.Ns, response.Extra} {
			for _, rr := range section {
				if rr.Header().Ttl < ttl {
					ttl = rr.Header().Ttl
				}
			}
		}
		if ttl > 0 {
			now := time.Now()
			s.mu.Lock()
			if len(s.cache) >= s.MaxCache {
				// Evict a single random entry instead of flushing the whole
				// map: at high query rates a full reset would stampede every
				// cached key into the upstream path at once.
				for victim := range s.cache {
					delete(s.cache, victim)
					s.cacheEvictions.Add(1)
					break
				}
			}
			s.cache[k] = cacheEntry{answer: response.Copy(), stored: now, expires: now.Add(time.Duration(ttl) * time.Second)}
			s.mu.Unlock()
		}
	}
	return response, "upstream"
}

// exchangeUpstream performs one forwarded exchange, retrying over TCP when
// the UDP response is truncated.
func (s *Server) exchangeUpstream(ctx context.Context, addr string, req *dns.Msg) (*dns.Msg, error) {
	attempt, cancel := context.WithTimeout(ctx, s.Timeout)
	r, _, err := s.Client.ExchangeContext(attempt, req, addr)
	cancel()
	if err != nil {
		return nil, err
	}
	if r != nil && r.Truncated {
		tcp := &dns.Client{Net: "tcp", Timeout: s.Timeout}
		attempt, cancel := context.WithTimeout(ctx, s.Timeout)
		defer cancel()
		retried, _, err := tcp.ExchangeContext(attempt, req, addr)
		return retried, err
	}
	return r, nil
}

// recordUpstream folds one exchange result into the upstream's health stats
// and advances the consecutive-failure breaker.
func (s *Server) recordUpstream(addr string, resp *dns.Msg, err error, elapsed time.Duration) {
	stat := s.upstreamStatFor(addr)
	if stat == nil {
		return
	}
	stat.requests.Add(1)
	stat.latencyNS.Add(int64(elapsed))
	if err == nil && resp != nil {
		stat.consecFails.Store(0)
		return
	}
	stat.errors.Add(1)
	message := "exchange failed"
	if err != nil {
		message = err.Error()
	}
	stat.lastError.Store(message)
	stat.lastErrorAt.Store(time.Now().UnixNano())
	if stat.consecFails.Add(1) >= upstreamBreakerThreshold {
		stat.blockedUntil.Store(time.Now().Add(upstreamBreakerCooldown).UnixNano())
	}
}

// upstreamStatFor returns the stat record for an address, creating it on
// first use; it returns nil after shutdown races that nil out the map.
func (s *Server) upstreamStatFor(addr string) *upstreamStat {
	s.upstreamMu.RLock()
	stat, ok := s.upstreamStats[addr]
	s.upstreamMu.RUnlock()
	if ok {
		return stat
	}
	s.upstreamMu.Lock()
	defer s.upstreamMu.Unlock()
	if s.upstreamStats == nil {
		return nil
	}
	if stat, ok = s.upstreamStats[addr]; !ok {
		stat = &upstreamStat{}
		s.upstreamStats[addr] = stat
	}
	return stat
}

// CacheStats snapshots resolver response-cache telemetry.
func (s *Server) CacheStats() CacheStats {
	s.mu.Lock()
	entries := len(s.cache)
	max := s.MaxCache
	s.mu.Unlock()
	return CacheStats{
		Entries:   entries,
		Max:       max,
		Hits:      s.cacheHits.Load(),
		Misses:    s.cacheMisses.Load(),
		Evictions: s.cacheEvictions.Load(),
	}
}

// UpstreamHealth reports per-upstream request counts, errors, latency and
// breaker state, ordered like the configured resolver list.
func (s *Server) UpstreamHealth() []UpstreamHealth {
	addresses := s.UpstreamAddresses()
	out := make([]UpstreamHealth, 0, len(addresses))
	now := time.Now().UnixNano()
	for _, addr := range addresses {
		health := UpstreamHealth{Address: addr, Status: "healthy"}
		if stat := s.upstreamStatFor(addr); stat != nil {
			health.Requests = stat.requests.Load()
			health.Errors = stat.errors.Load()
			health.LatencySumMS = float64(stat.latencyNS.Load()) / float64(time.Millisecond)
			if health.Requests > 0 {
				health.AvgLatencyMS = health.LatencySumMS / float64(health.Requests)
			}
			switch {
			case stat.blockedUntil.Load() > now:
				health.Status = "cooling"
			case stat.consecFails.Load() > 0:
				health.Status = "failing"
			}
			if message, ok := stat.lastError.Load().(string); ok {
				health.LastError = message
				at := time.Unix(0, stat.lastErrorAt.Load()).UTC()
				health.LastErrorAt = &at
			}
		}
		out = append(out, health)
	}
	return out
}
func (s *Server) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	start := time.Now()
	if req == nil {
		return
	}
	var resp *dns.Msg
	source := "error"
	if !s.permitted(w.RemoteAddr()) {
		resp = respond(req, dns.RcodeRefused)
	} else {
		resp, source = s.resolve(context.Background(), req)
	}
	if resp == nil {
		resp = respond(req, dns.RcodeServerFailure)
	}
	_ = w.WriteMsg(resp)
	if s.Logs != nil {
		e := observability.Event{Time: time.Now().UTC(), Source: source, Rcode: dns.RcodeToString[resp.Rcode], DurationMS: float64(time.Since(start).Microseconds()) / 1000}
		if len(req.Question) > 0 {
			e.Name = req.Question[0].Name
			e.Type = dns.TypeToString[req.Question[0].Qtype]
		}
		if w.RemoteAddr() != nil {
			e.Client = w.RemoteAddr().String()
		}
		s.Logs.Record(e)
	}
}
func (s *Server) Lookup(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
	if s == nil {
		return nil, errors.New("server unavailable")
	}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	r, _ := s.resolve(ctx, req)
	return r, nil
}

func (s *Server) UpstreamAddresses() []string {
	addresses := s.upstreamSet.Load().([]string)
	return append([]string(nil), addresses...)
}

// SetUpstreams validates and publishes a new immutable resolver configuration.
func (s *Server) SetUpstreams(addresses []string) error {
	if len(addresses) == 0 || len(addresses) > 8 {
		return errors.New("configure between 1 and 8 upstreams")
	}
	normalized := make([]string, 0, len(addresses))
	seen := make(map[string]bool)
	for _, item := range addresses {
		ipport, err := netip.ParseAddrPort(strings.TrimSpace(item))
		if err != nil || !ipport.Addr().IsValid() || ipport.Port() == 0 {
			return errors.New("upstream must be a numeric IP:port address")
		}
		candidate := ipport.String()
		if seen[candidate] {
			return errors.New("duplicate upstream address")
		}
		seen[candidate] = true
		normalized = append(normalized, candidate)
	}
	s.upstreamSet.Store(normalized)
	s.mu.Lock()
	s.cache = make(map[string]cacheEntry)
	s.mu.Unlock()
	return nil
}
