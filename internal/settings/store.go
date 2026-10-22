// Package settings stores operator-defined resolver configuration in SQLite.
package settings

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS resolver_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)"); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func ValidateUpstreams(addresses []string) ([]string, error) {
	if len(addresses) == 0 || len(addresses) > 8 {
		return nil, errors.New("configure between 1 and 8 upstream resolvers")
	}
	out := make([]string, 0, len(addresses))
	seen := make(map[string]bool)
	for _, part := range addresses {
		parsed, err := netip.ParseAddrPort(strings.TrimSpace(part))
		if err != nil || parsed.Port() == 0 {
			return nil, fmt.Errorf("invalid upstream address: %q", part)
		}
		value := parsed.String()
		if seen[value] {
			return nil, errors.New("duplicate upstream address")
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}
func (s *Store) Upstreams(fallback []string) ([]string, error) {
	var encoded string
	err := s.db.QueryRow("SELECT value FROM resolver_settings WHERE key='upstreams'").Scan(&encoded)
	if err == sql.ErrNoRows {
		return ValidateUpstreams(fallback)
	}
	if err != nil {
		return nil, err
	}
	var addresses []string
	if err = json.Unmarshal([]byte(encoded), &addresses); err != nil {
		return nil, err
	}
	return ValidateUpstreams(addresses)
}
func (s *Store) SaveUpstreams(addresses []string) error {
	normalized, err := ValidateUpstreams(addresses)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec("INSERT INTO resolver_settings(key,value) VALUES('upstreams',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(encoded))
	return err
}
