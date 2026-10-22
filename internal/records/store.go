package records

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "sync/atomic"

 "github.com/miekg/dns"
)

type Snapshot struct { Items []Record; Index map[string]map[uint16][]Record }
type Store struct { mu sync.Mutex; path string; snapshot atomic.Pointer[Snapshot] }
var ErrNotFound=errors.New("record not found")
var ErrConflict=errors.New("record conflicts with an existing record")

func Open(path string) (*Store,error) {
 s:=&Store{path:path}
 var items []Record
 data,err:=os.ReadFile(path)
 if err==nil {
  if err=json.Unmarshal(data,&items);err!=nil{return nil,fmt.Errorf("read records: %w",err)}
 }else if !errors.Is(err,os.ErrNotExist){return nil,err}
 snap,err:=build(items);if err!=nil{return nil,err}
 s.snapshot.Store(snap);return s,nil
}

func build(items []Record)(*Snapshot,error){
 snap:=&Snapshot{Items:make([]Record,0,len(items)),Index:make(map[string]map[uint16][]Record)}
 ids:=make(map[string]bool)
 owners:=make(map[string]map[string]bool)
 for _,original:=range items{
  r:=original
  if err:=r.Validate();err!=nil{return nil,fmt.Errorf("record %q: %w",r.ID,err)}
  if r.ID==""||ids[r.ID]{return nil,errors.New("duplicate or empty record id")}
  ids[r.ID]=true
  if !r.Enabled {snap.Items=append(snap.Items,r);continue}
  types:=owners[r.Name];if types==nil{types=map[string]bool{};owners[r.Name]=types}
  types[r.Type]=true
  snap.Items=append(snap.Items,r)
  if snap.Index[r.Name]==nil{snap.Index[r.Name]=make(map[uint16][]Record)}
  qt:=dns.StringToType[r.Type]
  snap.Index[r.Name][qt]=append(snap.Index[r.Name][qt],r)
 }
 for _,types:=range owners{
  if types["CNAME"]&&len(types)>1{return nil,ErrConflict}
 }
 sort.Slice(snap.Items,func(i,j int)bool{return snap.Items[i].Name<snap.Items[j].Name})
 return snap,nil
}
func(s *Store) List() []Record {
 snap:=s.snapshot.Load()
 return append([]Record(nil),snap.Items...)
}
func(s *Store) Lookup(name string,kind uint16)([]dns.RR,bool){
 snap:=s.snapshot.Load()
 n:=strings.ToLower(dns.Fqdn(name))
 types:=snap.Index[n]
 rrsets:=types[kind]
 if len(rrsets)==0 && kind!=dns.TypeCNAME {rrsets=types[dns.TypeCNAME]}
 if len(rrsets)==0{return nil,false}
 ans:=make([]dns.RR,0,len(rrsets))
 for _,r:=range rrsets{rr,_:=r.RR();ans=append(ans,rr)}
 return ans,true
}
func(s *Store) Save(record Record) error {
 if err:=record.Validate();err!=nil{return err}
 s.mu.Lock();defer s.mu.Unlock()
 items:=append([]Record(nil),s.snapshot.Load().Items...)
 found:=false
 for i:=range items {if items[i].ID==record.ID{items[i]=record;found=true;break}}
 if !found{items=append(items,record)}
 return s.commit(items)
}
func(s *Store) Delete(id string) error{
 s.mu.Lock();defer s.mu.Unlock()
 old:=s.snapshot.Load().Items
 items:=make([]Record,0,len(old))
 for _,r:=range old{if r.ID!=id{items=append(items,r)}}
 if len(old)==len(items){return ErrNotFound}
 return s.commit(items)
}
func(s *Store) commit(items []Record) error{
 snap,err:=build(items);if err!=nil{return err}
 data,err:=json.MarshalIndent(snap.Items,"","  ");if err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(s.path),".records-*.tmp");if err!=nil{return err}
 defer os.Remove(f.Name())
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(data);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(f.Name(),s.path);err!=nil{return err}
 s.snapshot.Store(snap)
 return nil
}
