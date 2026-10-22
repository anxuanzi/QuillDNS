package dnsserver

import (
	"github.com/anxuanzi/QuillDNS/internal/records"
	"path/filepath"
	"testing"
)

func TestUpstreamHotReloadAndValidation(t *testing.T) {
	s, err := records.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := New(s, nil, []string{"1.1.1.1:53"})
	if err = server.SetUpstreams([]string{"8.8.8.8:53", "[2606:4700:4700::1111]:53"}); err != nil {
		t.Fatal(err)
	}
	got := server.UpstreamAddresses()
	if len(got) != 2 || got[0] != "8.8.8.8:53" {
		t.Fatalf("unexpected upstreams: %v", got)
	}
	got[0] = "modified"
	if server.UpstreamAddresses()[0] != "8.8.8.8:53" {
		t.Fatal("leaked mutable upstream slice")
	}
	if err = server.SetUpstreams([]string{"invalid"}); err == nil {
		t.Fatal("accepted invalid configuration")
	}
	if server.UpstreamAddresses()[0] != "8.8.8.8:53" {
		t.Fatal("invalid config replaced active upstreams")
	}
}
