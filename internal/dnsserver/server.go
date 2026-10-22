// Package dnsserver implements local-first resolution and public upstream fallback.
package dnsserver

import (
 "context"
 "errors"
 "strings"
 "sync"
 "time"

 "github.com/anxuanzi/QuillDNS/internal/observability"
 "github.com/anxuanzi/QuillDNS/internal/records"
 "github.com/miekg/dns"
)
type Upstream interface {ExchangeContext(context.Context,*dns.Msg,string)(*dns.Msg,time.Duration,error)}
type cacheEntry struct{answer *dns.Msg; expires time.Time; stored time.Time}
type Server struct{
 Records *records.Store
 Logs *observability.Collector
 Client Upstream
 Upstreams []string
 Timeout time.Duration
 mu sync.Mutex
 cache map[string]cacheEntry
 MaxCache int
}
func New(r *records.Store,o *observability.Collector,upstreams []string)*Server{
 return &Server{Records:r,Logs:o,Client:&dns.Client{Net:"udp",Timeout:2*time.Second},Upstreams:upstreams,Timeout:2*time.Second,cache:make(map[string]cacheEntry),MaxCache:10000}
}
func key(q dns.Question)string{return strings.ToLower(dns.Fqdn(q.Name))+"|"+dns.TypeToString[q.Qtype]+"|"+dns.ClassToString[q.Qclass]}
func(s *Server) resolve(ctx context.Context,req *dns.Msg)(*dns.Msg,string){
 if len(req.Question)!=1||req.Question[0].Qclass!=dns.ClassINET{
  r:=new(dns.Msg);r.SetRcode(req,dns.RcodeFormatError);return r,"error"
 }
 q:=req.Question[0]
 if rr,ok:=s.Records.Lookup(q.Name,q.Qtype);ok{
  r:=new(dns.Msg);r.SetReply(req);r.Answer=rr;return r,"local"
 }
 // Cache only ordinary queries; skip DNSSEC/EDNS options to avoid losing request semantics.
 cacheable:=req.IsEdns0()==nil && !req.CheckingDisabled
 if cacheable{
  s.mu.Lock()
  entry,found:=s.cache[key(q)]
  if found&&time.Now().Before(entry.expires){
   r:=entry.answer.Copy();age:=uint32(time.Since(entry.stored)/time.Second)
   for _,rr:=range r.Answer{h:=rr.Header();if h.Ttl>age{h.Ttl-=age}else{h.Ttl=0}}
   r.Id=req.Id;r.RecursionDesired=req.RecursionDesired
   s.mu.Unlock()
   return r,"cache"
  }
  if found{delete(s.cache,key(q))}
  s.mu.Unlock()
 }
 var response *dns.Msg
 for _,addr:=range s.Upstreams{
  attempt,cancel:=context.WithTimeout(ctx,s.Timeout)
  r,_,err:=s.Client.ExchangeContext(attempt,req,addr)
  cancel()
  if err!=nil{continue}
  if r!=nil && r.Truncated {
   tcp:=&dns.Client{Net:"tcp",Timeout:s.Timeout}
   attempt,cancel=context.WithTimeout(ctx,s.Timeout)
   r,_,err=tcp.ExchangeContext(attempt,req,addr);cancel()
   if err!=nil{continue}
  }
  if r!=nil{response=r;break}
 }
 if response==nil{r:=new(dns.Msg);r.SetRcode(req,dns.RcodeServerFailure);return r,"error"}
 if cacheable&&response.Rcode==dns.RcodeSuccess&&len(response.Answer)>0{
  ttl:=uint32(300)
  for _,rr:=range response.Answer{if rr.Header().Ttl<ttl{ttl=rr.Header().Ttl}}
  if ttl>0{
   s.mu.Lock()
   if len(s.cache)>=s.MaxCache{s.cache=make(map[string]cacheEntry)}
   s.cache[key(q)]=cacheEntry{answer:response.Copy(),stored:time.Now(),expires:time.Now().Add(time.Duration(ttl)*time.Second)}
   s.mu.Unlock()
  }
 }
 return response,"upstream"
}
func(s *Server) ServeDNS(w dns.ResponseWriter,req *dns.Msg){
 start:=time.Now()
 ctx:=context.Background()
 if req==nil{return}
 resp,source:=s.resolve(ctx,req)
 if resp==nil{resp=new(dns.Msg);resp.SetRcode(req,dns.RcodeServerFailure)}
 _=w.WriteMsg(resp)
 if s.Logs!=nil {
  e:=observability.Event{Time:time.Now().UTC(),Source:source,Rcode:dns.RcodeToString[resp.Rcode],DurationMS:float64(time.Since(start).Microseconds())/1000}
  if len(req.Question)>0{e.Name=req.Question[0].Name;e.Type=dns.TypeToString[req.Question[0].Qtype]}
  if w.RemoteAddr()!=nil{e.Client=w.RemoteAddr().String()}
  s.Logs.Record(e)
 }
}
func(s *Server) Lookup(ctx context.Context,name string,qtype uint16)(*dns.Msg,error){
 if s==nil{return nil,errors.New("server unavailable")}
 req:=new(dns.Msg);req.SetQuestion(dns.Fqdn(name),qtype)
 r,_:=s.resolve(ctx,req);return r,nil
}
