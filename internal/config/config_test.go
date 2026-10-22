package config

import (
	"flag"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestSafeDefaults(t *testing.T) {
	got, err := Parse(nil, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.HTTPListen != "127.0.0.1:8080" || got.DNSListen != "127.0.0.1:1053" {
		t.Fatalf("defaults must bind to loopback: %+v", got)
	}
	if got.DataPath != DefaultDataPath || got.AllowNetworks != DefaultAllowNetworks {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestEnvironmentVariables(t *testing.T) {
	got, err := Parse(nil, env(map[string]string{
		"QUILLDNS_DATA_PATH":      "/data/dns.sqlite",
		"QUILLDNS_HTTP_LISTEN":    "127.0.0.1:18080",
		"QUILLDNS_DNS_LISTEN":     "192.168.1.10:53",
		"QUILLDNS_UPSTREAMS":      "9.9.9.9:53,[2620:fe::fe]:53",
		"QUILLDNS_ALLOW_NETWORKS": "192.168.1.0/24,127.0.0.0/8",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got.DataPath != "/data/dns.sqlite" || got.DNSListen != "192.168.1.10:53" ||
		got.HTTPListen != "127.0.0.1:18080" || got.AllowNetworks != "192.168.1.0/24,127.0.0.0/8" {
		t.Fatalf("env was not respected: %+v", got)
	}
}

func TestFlagsOverrideEnvironment(t *testing.T) {
	got, err := Parse([]string{"-http", "127.0.0.1:19090", "-data", "/cli/override.sqlite"},
		env(map[string]string{
			"QUILLDNS_HTTP_LISTEN": "127.0.0.1:8081",
			"QUILLDNS_DATA_PATH":   "/env/data.sqlite",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if got.HTTPListen != "127.0.0.1:19090" || got.DataPath != "/cli/override.sqlite" {
		t.Fatalf("flag precedence failed: %+v", got)
	}
}

func TestRejectInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		flags []string
	}{
		{"blank env", map[string]string{"QUILLDNS_DATA_PATH": ""}, nil},
		{"blank http env", map[string]string{"QUILLDNS_HTTP_LISTEN": ""}, nil},
		{"missing DNS port", map[string]string{"QUILLDNS_DNS_LISTEN": "127.0.0.1"}, nil},
		{"bad HTTP port", map[string]string{"QUILLDNS_HTTP_LISTEN": "127.0.0.1:99999"}, nil},
		{"bad upstream", map[string]string{"QUILLDNS_UPSTREAMS": "1.1.1.1"}, nil},
		{"empty upstream", map[string]string{"QUILLDNS_UPSTREAMS": "1.1.1.1:53,"}, nil},
		{"bad client range", map[string]string{"QUILLDNS_ALLOW_NETWORKS": "0.0.0.0/999"}, nil},
		{"empty cli path", nil, []string{"-data", ""}},
		{"unknown flag", nil, []string{"-invented"}},
		{"positional args", nil, []string{"hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := Parse(tt.flags, env(tt.env)); err == nil {
				t.Fatalf("accepted invalid configuration: %+v", got)
			}
		})
	}
}

func TestHelpReturnsSentinel(t *testing.T) {
	_, err := Parse([]string{"-h"}, env(nil))
	if err != flag.ErrHelp {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestIPv6ListenAddresses(t *testing.T) {
	c, err := Parse([]string{"-dns", "[::1]:1053", "-http", "[::1]:8080"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.DNSListen, "[::1]") {
		t.Fatalf("unexpected address: %+v", c)
	}
}
