package httpapi

import (
 "bytes"
 "encoding/json"
 "net/http/httptest"
 "path/filepath"
 "testing"

 "github.com/anxuanzi/QuillDNS/internal/dnsserver"
 "github.com/anxuanzi/QuillDNS/internal/observability"
 "github.com/anxuanzi/QuillDNS/internal/records"
 "github.com/anxuanzi/QuillDNS/internal/settings"
)
func TestUpstreamManagement(t *testing.T){
 path:=filepath.Join(t.TempDir(),"data.sqlite")
 rs,err:=records.Open(path);if err!=nil{t.Fatal(err)}
 defer rs.Close()
 ss,err:=settings.Open(path);if err!=nil{t.Fatal(err)}
 defer ss.Close()
 resolver:=dnsserver.New(rs,observability.New(10),[]string{"1.1.1.1:53"})
 api:=(&API{Records:rs,Settings:ss,DNS:resolver,Telemetry:observability.New(10)}).Handler()
 call:=func(method,payload string) *httptest.ResponseRecorder{
  t.Helper()
  rec:=httptest.NewRecorder()
  api.ServeHTTP(rec,httptest.NewRequest(method,"/api/v1/upstreams",bytes.NewBufferString(payload)))
  return rec
 }
 rec:=call("PUT",`{"addresses":["8.8.8.8:53","9.9.9.9:53"]}`)
 if rec.Code!=200{t.Fatalf("save: %d %s",rec.Code,rec.Body.String())}
 rec=call("GET","")
 if rec.Code!=200{t.Fatalf("get %d",rec.Code)}
 var parsed struct{Data []string `json:"data"`}
 if err=json.Unmarshal(rec.Body.Bytes(),&parsed);err!=nil{t.Fatal(err)}
 if len(parsed.Data)!=2||parsed.Data[0]!="8.8.8.8:53"{t.Fatalf("wrong response %v",parsed)}
 rec=call("PUT",`{"addresses":["not-a-resolver"]}`)
 if rec.Code!=400{t.Fatalf("invalid address accepted: %d",rec.Code)}
 got,err:=ss.Upstreams(nil)
 if err!=nil||len(got)!=2{t.Fatalf("bad persisted state: %v %v",got,err)}
}
