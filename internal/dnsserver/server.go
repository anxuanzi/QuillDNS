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
type Server struct {
	Records   *records.Store
	Logs      *observability.Collector
	Client    Upstream
	upstreamSet atomic.Value
	Timeout   time.Duration
	mu        sync.Mutex
	cache     map[string]cacheEntry
	MaxCache  int
	Allow     []netip.Prefix
}

func New(r *records.Store, o *observability.Collector, upstreams []string) *Server {
	s := &Server{Records:r,Logs:o,Client:&dns.Client{Net:"udp",Timeout:2*time.Second},Timeout:2*time.Second,cache:make(map[string]cacheEntry),MaxCache:10000}
 s.upstreamSet.Store(append([]string(nil),upstreams...))
 return s
}
func (s *Server) SetAllowedNetworks(networks string) error {
	prefixes := make([]netip.Prefix, 0)
	for _, part := range strings.Split(networks, ",") {
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
		s.mu.Lock()
		entry, found := s.cache[k]
		if found && time.Now().Before(entry.expires) {
			r := entry.answer.Copy()
			age := uint32(time.Since(entry.stored) / time.Second)
			// Age TTLs in all sections, not just the answer section.
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
			r.Id = req.Id
			r.RecursionDesired = req.RecursionDesired
			s.mu.Unlock()
			return r, "cache"
		}
		if found {
			delete(s.cache, k)
		}
		s.mu.Unlock()
	}
	var response *dns.Msg
	for _, addr := range s.UpstreamAddresses() {
		attempt, cancel := context.WithTimeout(ctx, s.Timeout)
		r, _, err := s.Client.ExchangeContext(attempt, req, addr)
		cancel()
		if err != nil {
			continue
		}
		if r != nil && r.Truncated {
			tcp := &dns.Client{Net: "tcp", Timeout: s.Timeout}
			attempt, cancel = context.WithTimeout(ctx, s.Timeout)
			r, _, err = tcp.ExchangeContext(attempt, req, addr)
			cancel()
			if err != nil {
				continue
			}
		}
		if r != nil && r.Id == req.Id {
			response = r
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
			s.mu.Lock()
			if len(s.cache) >= s.MaxCache {
				s.cache = make(map[string]cacheEntry)
			}
			s.cache[k] = cacheEntry{answer: response.Copy(), stored: time.Now(), expires: time.Now().Add(time.Duration(ttl) * time.Second)}
			s.mu.Unlock()
		}
	}
	return response, "upstream"
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

func(s *Server) UpstreamAddresses()[]string {
 addresses:=s.upstreamSet.Load().([]string)
 return append([]string(nil),addresses...)
}

// SetUpstreams validates and publishes a new immutable resolver configuration.
func(s *Server) SetUpstreams(addresses []string)error{
 if len(addresses)==0||len(addresses)>8{return errors.New("configure between 1 and 8 upstreams")}
 normalized:=make([]string,0,len(addresses))
 seen:=make(map[string]bool)
 for _,item:=range addresses{
  ipport,err:=netip.ParseAddrPort(strings.TrimSpace(item))
  if err!=nil||!ipport.Addr().IsValid()||ipport.Port()==0{return errors.New("upstream must be a numeric IP:port address")}
  candidate:=ipport.String()
  if seen[candidate]{return errors.New("duplicate upstream address")}
  seen[candidate]=true
  normalized=append(normalized,candidate)
 }
 s.upstreamSet.Store(normalized)
 s.mu.Lock();s.cache=make(map[string]cacheEntry);s.mu.Unlock()
 return nil
}
