// Package store is the phone's persistence: which catways it has paired with,
// the session token and certificate pin for each, and the handful of
// preferences the app keeps. It is the catsclient.TrustStore implementation
// the real app runs on.
//
// # Shape
//
// One bytdb file in mobile.DataDir(), one key/value table. The same
// integration shape as church_mobile's session store and grmob's own
// examples/todoapp: the engine opens lazily on first use, every method is
// nil-receiver-safe, and with no data directory registered (the WASM preview,
// bare unit tests) the store runs in memory. Callers never branch on whether
// persistence exists; they get the same answers either way, just not across a
// relaunch.
//
// # The honest security note
//
// The session token IS the credential: there is no second factor and no
// refresh flow (see catsclient.TrustStore). grmob has no keystore binding yet,
// so the token lives in the app's private data directory rather than the
// Keychain or EncryptedSharedPreferences. On a non-rooted device the OS sandbox
// keeps other apps out of it; it is NOT protected at rest by the device
// passcode. The mitigation is server-side: a session expires on catway's TTL,
// and "forget device" here clears the local copy. Swap the read/write pair in
// this file for grmob's keystore when that lands.
//
// # Keys
//
//	token:<endpointID>   the session credential
//	pin:<endpointID>     the certificate's SHA-256, lowercase hex
//	endpoints            the paired endpoints, as a JSON array (Endpoint)
//	active               the endpoint the app connects to on launch
//
// Tokens and pins are per endpoint id rather than per host so that a
// re-pairing after a certificate regeneration replaces the old pin under the
// same key instead of accumulating stale ones.
package store

import (
	"encoding/json"
	"log"
	"path/filepath"
	"sync"

	"github.com/rohanthewiz/bytdb"
	bsql "github.com/rohanthewiz/bytdb/sql"
	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/grmob/mobile"
)

// Store is the app's persistence handle. Safe for concurrent use: the
// connection manager writes a pin from the dial goroutine while a render
// pass reads the endpoint list.
//
// The zero value is not usable; call Open.
type Store struct {
	mu sync.Mutex
	// mem is the in-memory fallback, and also the write-through cache when
	// the file is open. Reads always come from here after the first load,
	// which keeps a render pass off the disk.
	mem map[string]string
	// eng/db are nil when running in memory.
	eng  *bytdb.Engine
	db   *bsql.DB
	path string
}

var (
	openMu sync.Mutex
	opened *Store
)

// Open returns the store for the current data directory, opening it on first
// use. Cheap to call repeatedly: after the first call it is a mutex acquire
// and a string compare. A changed data directory (tests moving to a fresh
// t.TempDir) closes the old engine and opens the new path.
//
// Never returns nil: with no data directory, or a file that will not open,
// the store runs in memory and says so in the log. Losing "stay paired"
// beats losing the app.
func Open() *Store {
	openMu.Lock()
	defer openMu.Unlock()

	dir := mobile.DataDir()
	path := ""
	if dir != "" {
		path = filepath.Join(dir, "cats.bytdb")
	}
	if opened != nil && opened.path == path {
		return opened
	}
	if opened != nil {
		opened.close()
		opened = nil
	}

	s := &Store{mem: map[string]string{}, path: path}
	if path != "" {
		if err := s.openFile(path); err != nil {
			log.Printf("store: %v; running in memory", err)
		}
	}
	opened = s
	return s
}

// Close releases the engine and forgets the singleton. The app never calls
// it (the engine lives for the process, and bytdb's WAL makes a hard kill
// safe); tests use it to simulate a relaunch.
func Close() {
	openMu.Lock()
	defer openMu.Unlock()
	if opened != nil {
		opened.close()
		opened = nil
	}
}

// openFile opens the bytdb file, creates the table, and loads every row into
// the cache so subsequent reads are memory reads.
func (s *Store) openFile(path string) error {
	eng, err := bytdb.Open(path)
	if err != nil {
		return err
	}
	db := bsql.New(eng)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kv (key text PRIMARY KEY, value text)`); err != nil {
		_ = eng.Close()
		return err
	}
	res, err := db.Exec(`SELECT key, value FROM kv`)
	if err != nil {
		_ = eng.Close()
		return err
	}
	for _, row := range res.Rows {
		if len(row) < 2 {
			continue
		}
		key, _ := row[0].(string)
		value, _ := row[1].(string)
		if key != "" {
			s.mem[key] = value
		}
	}
	s.eng, s.db = eng, db
	return nil
}

func (s *Store) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eng != nil {
		if err := s.eng.Close(); err != nil {
			log.Printf("store: closing %s: %v", s.path, err)
		}
		s.eng, s.db = nil, nil
	}
}

// get reads one key from the cache.
func (s *Store) get(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.mem[key]
	return v, ok
}

// put writes one key, to the cache and then to the file. A file write
// failure is returned: pairing persists the token before it activates the
// connection, so a failure surfaces as an error the pair screen shows rather
// than as a session the next launch cannot find.
func (s *Store) put(key, value string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem[key] = value
	if s.db == nil {
		return nil
	}
	// DELETE + INSERT rather than an upsert: bytdb's SQL layer has no
	// ON CONFLICT, and the pair is atomic enough under this mutex, which is
	// the only writer.
	if _, err := s.db.Exec(`DELETE FROM kv WHERE key = $1`, key); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO kv (key, value) VALUES ($1, $2)`, key, value)
	return err
}

// del removes one key. Failures are logged and swallowed: local teardown
// must always succeed from the caller's perspective, so that "forget device"
// works in airplane mode and a corrupt row cannot trap the user with a
// server they cannot reach.
func (s *Store) del(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mem, key)
	if s.db == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM kv WHERE key = $1`, key); err != nil {
		log.Printf("store: deleting %s: %v", key, err)
	}
}

// --- catsclient.TrustStore --------------------------------------------------

func (s *Store) ReadToken(endpointID string) (string, bool, error) {
	v, ok := s.get("token:" + endpointID)
	return v, ok, nil
}

func (s *Store) WriteToken(endpointID, token string) error {
	return s.put("token:"+endpointID, token)
}

func (s *Store) ClearToken(endpointID string) error {
	s.del("token:" + endpointID)
	return nil
}

func (s *Store) ReadPin(endpointID string) (string, bool, error) {
	v, ok := s.get("pin:" + endpointID)
	return v, ok, nil
}

func (s *Store) WritePin(endpointID, sha256Hex string) error {
	return s.put("pin:"+endpointID, sha256Hex)
}

// ClearPin forgets an endpoint's certificate. Part of "forget device": a
// re-pairing after the server regenerated its certificate must start from
// no pin, or DecideCert reports a mismatch against the one it just forgot.
func (s *Store) ClearPin(endpointID string) {
	s.del("pin:" + endpointID)
}

// --- endpoints ----------------------------------------------------------------

const (
	keyEndpoints = "endpoints"
	keyActive    = "active"
)

// Endpoints is every catway the phone has paired with, in pairing order.
// The slice is the caller's to keep; it is decoded fresh from the cache.
func (s *Store) Endpoints() []catsclient.Endpoint {
	raw, ok := s.get(keyEndpoints)
	if !ok || raw == "" {
		return nil
	}
	var list []catsclient.Endpoint
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// A row that will not decode is treated as empty rather than fatal;
		// the next pairing overwrites it. Logged so it is not silent.
		log.Printf("store: endpoints row is unreadable: %v", err)
		return nil
	}
	return list
}

// AddEndpoint records an endpoint, replacing any earlier one with the same
// ID, and makes it the active one. Pairing again with the same server is the
// common way to recover from a regenerated certificate, so "same id" means
// "replace", not "duplicate".
func (s *Store) AddEndpoint(e catsclient.Endpoint) error {
	list := s.Endpoints()
	replaced := false
	for i := range list {
		if list[i].ID == e.ID {
			list[i] = e
			replaced = true
		}
	}
	if !replaced {
		list = append(list, e)
	}
	if err := s.putEndpoints(list); err != nil {
		return err
	}
	return s.put(keyActive, e.ID)
}

// RemoveEndpoint forgets an endpoint and everything stored under its id. If
// it was the active one, the first remaining endpoint (if any) becomes
// active, so the app lands on a working server rather than an empty screen.
func (s *Store) RemoveEndpoint(id string) error {
	list := s.Endpoints()
	kept := list[:0]
	for _, e := range list {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	if err := s.putEndpoints(kept); err != nil {
		return err
	}
	_ = s.ClearToken(id)
	s.ClearPin(id)
	if active, _ := s.get(keyActive); active == id {
		if len(kept) > 0 {
			return s.put(keyActive, kept[0].ID)
		}
		s.del(keyActive)
	}
	return nil
}

func (s *Store) putEndpoints(list []catsclient.Endpoint) error {
	if len(list) == 0 {
		s.del(keyEndpoints)
		return nil
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.put(keyEndpoints, string(raw))
}

// Active is the endpoint the app connects to on launch, or false when the
// phone has never paired (or has forgotten every server).
func (s *Store) Active() (catsclient.Endpoint, bool) {
	id, ok := s.get(keyActive)
	if !ok {
		return catsclient.Endpoint{}, false
	}
	for _, e := range s.Endpoints() {
		if e.ID == id {
			return e, true
		}
	}
	return catsclient.Endpoint{}, false
}

// SetActive switches which endpoint the app connects to. Unknown ids are
// ignored rather than stored, so Active can never name an endpoint the list
// does not hold.
func (s *Store) SetActive(id string) error {
	for _, e := range s.Endpoints() {
		if e.ID == id {
			return s.put(keyActive, id)
		}
	}
	return nil
}
