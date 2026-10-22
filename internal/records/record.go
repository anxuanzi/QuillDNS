// Package records owns local DNS override semantics independent of transport.
package records

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/miekg/dns"
)

// Record is a single override; multiple A, AAAA, MX, TXT or SRV values form an RRset.
type Record struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      uint32 `json:"ttl"`
	Priority uint16 `json:"priority,omitempty"`
	Weight   uint16 `json:"weight,omitempty"`
	Port     uint16 `json:"port,omitempty"`
	Flags    uint8  `json:"flags,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// Normalize applies absolute DNS naming and rejects malformed hostnames.
func Normalize(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, " \r\n\t\\\"") {
		return "", errors.New("invalid DNS name")
	}
	name = dns.Fqdn(name)
	if len(name) > 255 {
		return "", errors.New("DNS name exceeds 255 bytes")
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			return "", errors.New("invalid DNS label length")
		}
		for i, c := range label {
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || (c == '_' && i == 0) {
				continue
			}
			return "", errors.New("invalid DNS label character")
		}
	}
	return name, nil
}

func (r *Record) Validate() error {
	n, err := Normalize(r.Name)
	if err != nil {
		return err
	}
	r.Name = n
	if r.TTL == 0 || r.TTL > 86400 {
		return errors.New("TTL must be between 1 and 86400 seconds")
	}
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	switch r.Type {
	case "A", "AAAA":
		ip, e := netip.ParseAddr(r.Value)
		if e != nil || (r.Type == "A" && !ip.Is4()) || (r.Type == "AAAA" && !ip.Is6()) {
			return fmt.Errorf("invalid %s address", r.Type)
		}
		r.Value = ip.String()
	case "CNAME", "PTR", "MX", "SRV":
		v, e := Normalize(r.Value)
		if e != nil {
			return fmt.Errorf("invalid target: %w", e)
		}
		r.Value = v
	case "CAA":
		r.Tag = strings.ToLower(strings.TrimSpace(r.Tag))
		if r.Tag == "" || len(r.Tag) > 15 {
			return errors.New("CAA tag must contain 1-15 alphanumeric characters")
		}
		for _, char := range r.Tag {
			if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
				return errors.New("CAA tag may only contain ASCII letters and digits")
			}
		}
		if len(r.Value) > 255 || strings.ContainsAny(r.Value, "\r\n") {
			return errors.New("CAA value is invalid or exceeds 255 bytes")
		}
	case "TXT":
		if len(r.Value) > 4096 {
			return errors.New("TXT text exceeds 4096 bytes")
		}
	default:
		return fmt.Errorf("unsupported DNS record type: %q", r.Type)
	}
	if r.Type == "CNAME" && (r.Value == r.Name) {
		return errors.New("CNAME cannot point to itself")
	}
	return nil
}

// RR returns a fresh wire-format DNS RR. Never mutate shared records on the request path.
func (r Record) RR() (dns.RR, error) {
	h := dns.RR_Header{Name: r.Name, Rrtype: dns.StringToType[r.Type], Class: dns.ClassINET, Ttl: r.TTL}
	switch r.Type {
	case "A":
		ip, _ := netip.ParseAddr(r.Value)
		return &dns.A{Hdr: h, A: ip.AsSlice()}, nil
	case "AAAA":
		ip, _ := netip.ParseAddr(r.Value)
		return &dns.AAAA{Hdr: h, AAAA: ip.AsSlice()}, nil
	case "CNAME":
		return &dns.CNAME{Hdr: h, Target: r.Value}, nil
	case "PTR":
		return &dns.PTR{Hdr: h, Ptr: r.Value}, nil
	case "TXT":
		return &dns.TXT{Hdr: h, Txt: splitTXT(r.Value)}, nil
	case "CAA":
		return &dns.CAA{Hdr: h, Flag: r.Flags, Tag: r.Tag, Value: r.Value}, nil
	case "MX":
		return &dns.MX{Hdr: h, Preference: r.Priority, Mx: r.Value}, nil
	case "SRV":
		return &dns.SRV{Hdr: h, Priority: r.Priority, Weight: r.Weight, Port: r.Port, Target: r.Value}, nil
	default:
		return nil, errors.New("unsupported record")
	}
}

func splitTXT(v string) []string {
	if v == "" {
		return []string{""}
	}
	result := make([]string, 0, 1)
	// DNS TXT <character-string> is at most 255 bytes.
	for len(v) > 255 {
		result = append(result, v[:255])
		v = v[255:]
	}
	return append(result, v)
}
