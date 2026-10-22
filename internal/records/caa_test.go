package records

import (
 "path/filepath"
 "testing"

 "github.com/miekg/dns"
)

func TestCAARecordRoundTripAndFallback(t *testing.T) {
 store,err:=Open(filepath.Join(t.TempDir(),"records.sqlite"))
 if err!=nil {t.Fatal(err)}
 defer store.Close()
 r:=Record{ID:"caa-1",Name:"example.org",Type:"CAA",Tag:"ISSUE",Value:"letsencrypt.org",TTL:300,Enabled:true}
 if err=store.Save(r);err!=nil {t.Fatal(err)}
 found,ok:=store.Lookup("example.org.",dns.TypeCAA)
 if !ok||len(found)!=1 {t.Fatalf("CAA not found: %v",found)}
 caa,ok:=found[0].(*dns.CAA)
 if !ok||caa.Tag!="issue"||caa.Value!="letsencrypt.org" {t.Fatalf("unexpected CAA: %v",found)}
 if _,ok=store.Lookup("example.org.",dns.TypeA);ok {t.Fatal("CAA should not mask public A record")}
}
func TestCAAInvalidTag(t *testing.T) {
 cases:=[]Record{
  {Name:"example.org",Type:"CAA",Tag:"",Value:"issuer",TTL:60},
  {Name:"example.org",Type:"CAA",Tag:"issue\n",Value:"issuer",TTL:60},
  {Name:"example.org",Type:"CAA",Tag:"issue",Value:"a\nb",TTL:60},
 }
 for _,r:=range cases {
  if err:=r.Validate();err==nil {t.Errorf("accepted invalid CAA record %+v",r)}
 }
}
