package dnsserver

import (
 "net"
 "path/filepath"
 "sync/atomic"
 "testing"
 "time"

 "github.com/anxuanzi/QuillDNS/internal/observability"
 "github.com/anxuanzi/QuillDNS/internal/records"
 "github.com/miekg/dns"
)
func TestUDPAndTCPResolutionEndToEnd(t *testing.T){
 upstreamPacket,err:=net.ListenPacket("udp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 var upstreamCalls atomic.Int32
 upstreamHandler:=dns.HandlerFunc(func(w dns.ResponseWriter,r *dns.Msg){
  upstreamCalls.Add(1)
  response:=new(dns.Msg)
  response.SetReply(r)
  q:=r.Question[0]
  header:=dns.RR_Header{Name:q.Name,Rrtype:q.Qtype,Class:dns.ClassINET,Ttl:30}
  if q.Qtype==dns.TypeTXT{response.Answer=[]dns.RR{&dns.TXT{Hdr:header,Txt:[]string{"upstream-public"}}}}
  if q.Qtype==dns.TypeA{response.Answer=[]dns.RR{&dns.A{Hdr:header,A:net.IPv4(8,8,4,4)}}}
  _=w.WriteMsg(response)
 })
 upstreamServer:=&dns.Server{PacketConn:upstreamPacket,Handler:upstreamHandler}
 go func(){_ = upstreamServer.ActivateAndServe()}()
 defer upstreamServer.Shutdown()

 store,err:=records.Open(filepath.Join(t.TempDir(),"db.sqlite"));if err!=nil{t.Fatal(err)}
 defer store.Close()
 for _,r:=range []records.Record{
  {ID:"a",Name:"api.example.test",Type:"A",Value:"10.0.0.20",TTL:60,Enabled:true},
  {ID:"txt",Name:"api.example.test",Type:"TXT",Value:"local-verified",TTL:60,Enabled:true},
 }{
  if err=store.Save(r);err!=nil{t.Fatal(err)}
 }
 proxy:=New(store,observability.New(50),[]string{upstreamPacket.LocalAddr().String()})
 if err=proxy.SetAllowedNetworks("127.0.0.0/8");err!=nil{t.Fatal(err)}
 udpSocket,err:=net.ListenPacket("udp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 udpServer:=&dns.Server{PacketConn:udpSocket,Handler:proxy}
 go func(){_ = udpServer.ActivateAndServe()}()
 defer udpServer.Shutdown()
 tcpSocket,err:=net.Listen("tcp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 tcpServer:=&dns.Server{Listener:tcpSocket,Handler:proxy}
 go func(){_ = tcpServer.ActivateAndServe()}()
 defer tcpServer.Shutdown()
 tests:=[]struct{
  net string
  address string
  name string
  kind uint16
  want string
 }{
  {"udp",udpSocket.LocalAddr().String(),"api.example.test.",dns.TypeA,"10.0.0.20"},
  {"tcp",tcpSocket.Addr().String(),"api.example.test.",dns.TypeTXT,"local-verified"},
  {"udp",udpSocket.LocalAddr().String(),"else.example.test.",dns.TypeA,"8.8.4.4"},
  {"udp",udpSocket.LocalAddr().String(),"api.example.test.",dns.TypeMX,""},
 }
 for _,tc:=range tests{
  client:=&dns.Client{Net:tc.net,Timeout:3*time.Second}
  q:=new(dns.Msg);q.SetQuestion(tc.name,tc.kind)
  resp,_,err:=client.Exchange(q,tc.address)
  if err!=nil{t.Fatalf("%s %s: %v",tc.net,tc.name,err)}
  if resp.Rcode!=dns.RcodeSuccess{t.Fatalf("unexpected rcode %d",resp.Rcode)}
  if tc.want!=""{
   if len(resp.Answer)==0{t.Fatalf("missing %s answer",tc.name)}
   if tc.kind==dns.TypeA&&resp.Answer[0].(*dns.A).A.String()!=tc.want{t.Fatalf("wrong A %v",resp.Answer)}
   if tc.kind==dns.TypeTXT&&resp.Answer[0].(*dns.TXT).Txt[0]!=tc.want{t.Fatalf("wrong TXT %v",resp.Answer)}
  }
 }
 if upstreamCalls.Load()!=2{t.Fatalf("unexpected upstream count: %d",upstreamCalls.Load())}
 q:=new(dns.Msg);q.SetQuestion("else.example.test.",dns.TypeA)
 for i:=0;i<2;i++{
  client:=&dns.Client{Net:"udp",Timeout:3*time.Second}
  if _,_,err=client.Exchange(q,udpSocket.LocalAddr().String());err!=nil{t.Fatal(err)}
 }
 if upstreamCalls.Load()!=2{t.Fatalf("expected cache hit; upstream count %d",upstreamCalls.Load())}
}
