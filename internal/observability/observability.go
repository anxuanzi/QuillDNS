// Package observability implements bounded, integrated query analytics without external services.
package observability

import (
 "sync"
 "time"
)
type Event struct {
 Time time.Time `json:"time"`
 Name string `json:"name"`
 Type string `json:"type"`
 Client string `json:"client"`
 Source string `json:"source"`
 Rcode string `json:"rcode"`
 DurationMS float64 `json:"duration_ms"`
}
type Summary struct {
 Queries uint64 `json:"queries"`
 Local uint64 `json:"local"`
 Cache uint64 `json:"cache"`
 Upstream uint64 `json:"upstream"`
 Errors uint64 `json:"errors"`
}
type Collector struct {
 mu sync.RWMutex
 events []Event
 max int
 next int
 full bool
 summary Summary
}
func New(max int)*Collector{if max<1{max=1000};return &Collector{events:make([]Event,max),max:max}}
func(c *Collector) Record(e Event){
 c.mu.Lock();defer c.mu.Unlock()
 c.events[c.next]=e;c.next=(c.next+1)%c.max
 if c.next==0{c.full=true}
 c.summary.Queries++
 switch e.Source{case "local":c.summary.Local++;case "cache":c.summary.Cache++;case "upstream":c.summary.Upstream++}
 if e.Rcode!="NOERROR"{c.summary.Errors++}
}
func(c *Collector) Summary() Summary{c.mu.RLock();defer c.mu.RUnlock();return c.summary}
func(c *Collector) Events(limit int) []Event{
 c.mu.RLock();defer c.mu.RUnlock()
 n:=c.next;if c.full{n=c.max}
 if limit<=0||limit>n{limit=n}
 out:=make([]Event,0,limit)
 for i:=0;i<limit;i++{idx:=(c.next-1-i+c.max)%c.max;out=append(out,c.events[idx])}
 return out
}
