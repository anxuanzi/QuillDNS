// Package config defines QuillDNS process configuration and its env/CLI contract.
// Environment variables work in containers; CLI flags take precedence for operators.
package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

const (
	DefaultDataPath      = "./data/quilldns.db"
	DefaultHTTPListen    = "127.0.0.1:8080"
	DefaultDNSListen     = "127.0.0.1:1053"
	DefaultUpstreams     = "1.1.1.1:53,8.8.8.8:53"
	DefaultAllowNetworks = "127.0.0.0/8,::1/128"

	// DefaultLogRetentionDays bounds how long persisted query history is kept.
	DefaultLogRetentionDays = 7
	// DefaultLogMaxRows bounds how many query-history rows may accumulate,
	// so a busy resolver can never outgrow its disk through logging alone.
	DefaultLogMaxRows = 100_000
)

// Bounds for the query-history retention knobs. The upper limits keep a
// single operator's history database manageable and keep prune passes short.
const (
	MinLogRetentionDays = 1
	MaxLogRetentionDays = 365
	MinLogMaxRows       = 1_000
	MaxLogMaxRows       = 5_000_000
)

// Config contains only core resolver process settings. OIDC secrets are read
// separately at startup and must never be exposed through management endpoints.
type Config struct {
	DataPath         string
	HTTPListen       string
	DNSListen        string
	Upstreams        string
	AllowNetworks    string
	LogRetentionDays int
	LogMaxRows       int
}

// Parse accepts a dependency-injected env reader for hermetic unit tests.
// Precedence: explicit flags > QUILLDNS_* environment > documented defaults.
func Parse(args []string, lookupEnv func(string) (string, bool)) (Config, error) {
	entries := []struct {
		key, fallback string
		dest          *string
	}{}
	c := Config{}
	entries = append(entries,
		struct {
			key, fallback string
			dest          *string
		}{"QUILLDNS_DATA_PATH", DefaultDataPath, &c.DataPath},
		struct {
			key, fallback string
			dest          *string
		}{"QUILLDNS_HTTP_LISTEN", DefaultHTTPListen, &c.HTTPListen},
		struct {
			key, fallback string
			dest          *string
		}{"QUILLDNS_DNS_LISTEN", DefaultDNSListen, &c.DNSListen},
		struct {
			key, fallback string
			dest          *string
		}{"QUILLDNS_UPSTREAMS", DefaultUpstreams, &c.Upstreams},
		struct {
			key, fallback string
			dest          *string
		}{"QUILLDNS_ALLOW_NETWORKS", DefaultAllowNetworks, &c.AllowNetworks},
	)
	for _, entry := range entries {
		if value, ok := lookupEnv(entry.key); ok {
			value = strings.TrimSpace(value)
			if value == "" {
				return Config{}, fmt.Errorf("%s must not be empty", entry.key)
			}
			*entry.dest = value
		} else {
			*entry.dest = entry.fallback
		}
	}
	if err := envInt(lookupEnv, "QUILLDNS_LOG_RETENTION_DAYS", DefaultLogRetentionDays, &c.LogRetentionDays); err != nil {
		return Config{}, err
	}
	if err := envInt(lookupEnv, "QUILLDNS_LOG_MAX_ROWS", DefaultLogMaxRows, &c.LogMaxRows); err != nil {
		return Config{}, err
	}
	flags := flag.NewFlagSet("quilldns", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&c.DataPath, "data", c.DataPath, "SQLite database path")
	flags.StringVar(&c.HTTPListen, "http", c.HTTPListen, "management HTTP bind address")
	flags.StringVar(&c.DNSListen, "dns", c.DNSListen, "DNS UDP/TCP bind address")
	flags.StringVar(&c.Upstreams, "upstreams", c.Upstreams, "comma-separated DNS upstream addresses")
	flags.StringVar(&c.AllowNetworks, "allow-networks", c.AllowNetworks, "comma-separated permitted DNS client CIDRs")
	flags.IntVar(&c.LogRetentionDays, "log-retention-days", c.LogRetentionDays, "days to keep persisted query history")
	flags.IntVar(&c.LogMaxRows, "log-max-rows", c.LogMaxRows, "maximum number of persisted query-history rows")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if len(flags.Args()) != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %q", flags.Args())
	}
	if strings.TrimSpace(c.DataPath) == "" {
		return Config{}, fmt.Errorf("database path must not be empty")
	}
	if err := validListener("HTTP", c.HTTPListen); err != nil {
		return Config{}, err
	}
	if err := validListener("DNS", c.DNSListen); err != nil {
		return Config{}, err
	}
	if err := validUpstreams(c.Upstreams); err != nil {
		return Config{}, err
	}
	if err := validNetworks(c.AllowNetworks); err != nil {
		return Config{}, err
	}
	if c.LogRetentionDays < MinLogRetentionDays || c.LogRetentionDays > MaxLogRetentionDays {
		return Config{}, fmt.Errorf("log retention %dd is outside %d..%d days",
			c.LogRetentionDays, MinLogRetentionDays, MaxLogRetentionDays)
	}
	if c.LogMaxRows < MinLogMaxRows || c.LogMaxRows > MaxLogMaxRows {
		return Config{}, fmt.Errorf("log row budget %d is outside %d..%d rows",
			c.LogMaxRows, MinLogMaxRows, MaxLogMaxRows)
	}
	return c, nil
}

// envInt reads an optional integer environment variable into dest, keeping
// the documented default when unset. Empty and malformed values are rejected.
func envInt(lookupEnv func(string) (string, bool), key string, fallback int, dest *int) error {
	if raw, ok := lookupEnv(key); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("%s must be an integer, got %q", key, raw)
		}
		*dest = value
		return nil
	}
	*dest = fallback
	return nil
}

func validListener(name, address string) error {
	host, port, err := net.SplitHostPort(address)
	_ = host // Empty hosts are permitted, but main rejects unauthenticated remote HTTP binds.
	if err != nil {
		return fmt.Errorf("invalid %s listener %q: %w", name, address, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("invalid %s listener port %q", name, port)
	}
	return nil
}

func validUpstreams(value string) error {
	count := 0
	for address := range strings.SplitSeq(value, ",") {
		address = strings.TrimSpace(address)
		if address == "" {
			return fmt.Errorf("upstream list contains an empty address")
		}
		if err := validListener("upstream", address); err != nil {
			return err
		}
		count++
	}
	if count > 8 {
		return fmt.Errorf("at most eight upstreams are supported")
	}
	return nil
}

func validNetworks(value string) error {
	count := 0
	for network := range strings.SplitSeq(value, ",") {
		if strings.TrimSpace(network) == "" {
			return fmt.Errorf("client network list contains an empty CIDR")
		}
		if _, err := netip.ParsePrefix(strings.TrimSpace(network)); err != nil {
			return fmt.Errorf("invalid client CIDR %q: %w", network, err)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("at least one client network is required")
	}
	return nil
}
