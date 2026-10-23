package observability

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.sqlite")
	c, err := Open(path, 3, 7, 10000)
	if err != nil {
		t.Fatal(err)
	}
	c.Record(Event{Time: time.Now().UTC(), Name: "example.test.", Type: "A", Source: "local", Rcode: "NOERROR", DurationMS: 1.5})
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, err := Open(path, 3, 7, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(context.Background())
	records, err := next.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "example.test." {
		t.Fatalf("history: %+v", records)
	}
}
func TestRingBounded(t *testing.T) {
	c := New(2)
	for range 4 {
		c.Record(Event{Name: "test", Source: "cache", Rcode: "NOERROR"})
	}
	if len(c.Events(50)) != 2 || c.Summary().Cache != 4 {
		t.Fatal("bad ring or counters")
	}
}
