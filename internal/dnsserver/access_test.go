package dnsserver

import (
	"net"
	"net/netip"
	"path/filepath"
	"testing"

	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/miekg/dns"
)

func TestDNSAccessPolicy(t *testing.T) {
	store, err := records.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := New(store, nil, nil)
	if srv.permitted(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 42}) {
		t.Fatal("default must deny")
	}
	if err = srv.SetAllowedNetworks("127.0.0.0/8,::1/128"); err != nil {
		t.Fatal(err)
	}
	if !srv.permitted(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 42}) {
		t.Fatal("loopback denied")
	}
	if srv.permitted(&net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 42}) {
		t.Fatal("public client was allowed")
	}
	if err = srv.SetAllowedNetworks("invalid"); err == nil {
		t.Fatal("invalid CIDR allowed")
	}
	if _, err = netip.ParsePrefix("::1/128"); err != nil {
		t.Fatal(err)
	}
}
func TestLocalAnswerKeepsEDNS(t *testing.T) {
	store, err := records.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Save(records.Record{ID: "x", Name: "api.test", Type: "A", Value: "10.0.0.3", TTL: 30, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	srv := New(store, nil, nil)
	req := new(dns.Msg)
	req.SetQuestion("api.test.", dns.TypeA)
	req.SetEdns0(1232, true)
	answer, source := srv.resolve(nil, req)
	if source != "local" || len(answer.Answer) != 1 || answer.IsEdns0() == nil || answer.AuthenticatedData {
		t.Fatalf("unexpected local EDNS reply: %+v", answer)
	}
}
