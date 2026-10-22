package records

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

// Snapshot is immutable once published. Readers never acquire the writer lock.
type Snapshot struct {
	Items []Record
	Index map[string]map[uint16][]Record
}
type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	snapshot atomic.Pointer[Snapshot]
}

var ErrNotFound = errors.New("record not found")
var ErrConflict = errors.New("record conflicts with an existing record")

// Open initializes the embedded, transactional SQLite record store.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"CREATE TABLE IF NOT EXISTS dns_records(id TEXT PRIMARY KEY, payload TEXT NOT NULL)",
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize records: %w", err)
		}
	}
	rows, err := db.Query("SELECT payload FROM dns_records ORDER BY id")
	if err != nil {
		db.Close()
		return nil, err
	}
	items := make([]Record, 0)
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var record Record
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			break
		}
		items = append(items, record)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("load records: %w", err)
	}
	snap, err := build(items)
	if err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db}
	store.snapshot.Store(snap)
	return store, nil
}
func (s *Store) Close() error { return s.db.Close() }

func build(items []Record) (*Snapshot, error) {
	snap := &Snapshot{Items: make([]Record, 0, len(items)), Index: make(map[string]map[uint16][]Record)}
	ids := make(map[string]bool)
	owners := make(map[string]map[string]bool)
	unique := make(map[string]bool)
	for _, original := range items {
		r := original
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("record %q: %w", r.ID, err)
		}
		if r.ID == "" || ids[r.ID] {
			return nil, errors.New("duplicate or empty record id")
		}
		ids[r.ID] = true
		snap.Items = append(snap.Items, r)
		if !r.Enabled {
			continue
		}
		identity := fmt.Sprintf("%s|%s|%s|%d|%d|%d", r.Name, r.Type, r.Value, r.Priority, r.Weight, r.Port)
		if unique[identity] {
			return nil, ErrConflict
		}
		unique[identity] = true
		types := owners[r.Name]
		if types == nil {
			types = map[string]bool{}
			owners[r.Name] = types
		}
		types[r.Type] = true
		if snap.Index[r.Name] == nil {
			snap.Index[r.Name] = make(map[uint16][]Record)
		}
		qt := dns.StringToType[r.Type]
		snap.Index[r.Name][qt] = append(snap.Index[r.Name][qt], r)
	}
	for _, types := range owners {
		if types["CNAME"] && len(types) > 1 {
			return nil, ErrConflict
		}
	}
	sort.Slice(snap.Items, func(i, j int) bool {
		if snap.Items[i].Name != snap.Items[j].Name {
			return snap.Items[i].Name < snap.Items[j].Name
		}
		return snap.Items[i].ID < snap.Items[j].ID
	})
	return snap, nil
}
func (s *Store) List() []Record { return append([]Record(nil), s.snapshot.Load().Items...) }
func (s *Store) Lookup(name string, kind uint16) ([]dns.RR, bool) {
	snap := s.snapshot.Load()
	types := snap.Index[strings.ToLower(dns.Fqdn(name))]
	rrsets := types[kind]
	if len(rrsets) == 0 && kind != dns.TypeCNAME {
		rrsets = types[dns.TypeCNAME]
	}
	if len(rrsets) == 0 {
		return nil, false
	}
	ans := make([]dns.RR, 0, len(rrsets))
	for _, r := range rrsets {
		rr, _ := r.RR()
		ans = append(ans, rr)
	}
	return ans, true
}
func (s *Store) Save(record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]Record(nil), s.snapshot.Load().Items...)
	found := false
	for i := range items {
		if items[i].ID == record.ID {
			items[i] = record
			found = true
			break
		}
	}
	if !found {
		items = append(items, record)
	}
	snap, err := build(items)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO dns_records(id,payload) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", record.ID, string(raw))
	if err != nil {
		return err
	}
	s.snapshot.Store(snap)
	return nil
}
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.snapshot.Load().Items
	items := make([]Record, 0, len(old))
	for _, r := range old {
		if r.ID != id {
			items = append(items, r)
		}
	}
	if len(items) == len(old) {
		return ErrNotFound
	}
	snap, err := build(items)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec("DELETE FROM dns_records WHERE id=?", id); err != nil {
		return err
	}
	s.snapshot.Store(snap)
	return nil
}
