package records

import (
	"testing"
)

func BenchmarkSnapshotLocalLookup(b *testing.B) {
	s, err := Open(b.TempDir() + "/records.sqlite")
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 100; i++ {
		name := "host" + string(rune('a'+i/26)) + string(rune('a'+i%26)) + ".example.test"
		if err = s.Save(Record{ID: name, Name: name, Type: "A", Value: "10.0.0.1", TTL: 300, Enabled: true}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Lookup("hostaa.example.test.", 1)
	}
}
