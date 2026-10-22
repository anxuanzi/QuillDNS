package settings

import (
	"path/filepath"
	"testing"
)

func TestUpstreamsPersistAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := s.Upstreams([]string{"1.1.1.1:53"})
	if err != nil || len(addresses) != 1 {
		t.Fatalf("%v %v", addresses, err)
	}
	if err = s.SaveUpstreams([]string{"8.8.8.8:53", "[2606:4700:4700::1111]:53"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	addresses, err = s.Upstreams(nil)
	if err != nil || len(addresses) != 2 {
		t.Fatalf("saved upstreams %v %v", addresses, err)
	}
	for _, invalid := range [][]string{{}, {":53"}, {"not-a-host:53"}, {"1.1.1.1:53", "1.1.1.1:53"}, {"127.0.0.1:0"}} {
		if err = s.SaveUpstreams(invalid); err == nil {
			t.Errorf("accepted invalid addresses: %v", invalid)
		}
	}
}
