package records

import (
 "path/filepath"
 "sync"
 "testing"

 "github.com/miekg/dns"
)

func TestSQLitePersistenceAndConcurrentReads(t *testing.T){
 path:=filepath.Join(t.TempDir(),"quilldns.db")
 store,err:=Open(path);if err!=nil{t.Fatal(err)}
 record:=Record{ID:"1",Name:"local.example",Type:"TXT",Value:"hello",TTL:60,Enabled:true}
 if err=store.Save(record);err!=nil{t.Fatal(err)}
 var wg sync.WaitGroup
 for i:=0;i<8;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   for j:=0;j<300;j++{
    if _,found:=store.Lookup("local.example.",dns.TypeTXT);!found{t.Error("missing record")}
   }
  }()
 }
 wg.Wait()
 if err=store.Close();err!=nil{t.Fatal(err)}
 reopened,err:=Open(path);if err!=nil{t.Fatal(err)}
 defer reopened.Close()
 if records,ok:=reopened.Lookup("local.example.",dns.TypeTXT);!ok||len(records)!=1{t.Fatal("persisted records missing")}
 if err=reopened.Delete("1");err!=nil{t.Fatal(err)}
 if _,ok:=reopened.Lookup("local.example.",dns.TypeTXT);ok{t.Fatal("deleted record visible")}
}
func TestDuplicateActiveRRRejected(t *testing.T){
 s,err:=Open(filepath.Join(t.TempDir(),"db.sqlite"));if err!=nil{t.Fatal(err)}
 defer s.Close()
 r:=Record{ID:"a",Name:"one.test",Type:"A",Value:"10.0.0.5",TTL:30,Enabled:true}
 if err=s.Save(r);err!=nil{t.Fatal(err)}
 r.ID="b"
 if err=s.Save(r);err!=ErrConflict{t.Fatalf("expected conflict, got %v",err)}
}
