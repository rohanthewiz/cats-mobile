package store

import (
	"testing"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/grmob/mobile"
)

// The store is exercised through its file-backed path: a temp data directory,
// a write, a simulated relaunch (Close, then Open), and a read. The in-memory
// path is the same code minus the file, so a pass here covers both.

func openTemp(t *testing.T) *Store {
	t.Helper()
	mobile.SetDataDir(t.TempDir())
	t.Cleanup(func() {
		Close()
		mobile.SetDataDir("")
	})
	return Open()
}

func TestTokensAndPinsSurviveARelaunch(t *testing.T) {
	s := openTemp(t)
	if err := s.WriteToken("ep1", "sess.abc"); err != nil {
		t.Fatal(err)
	}
	if err := s.WritePin("ep1", "deadbeef"); err != nil {
		t.Fatal(err)
	}

	Close()
	s = Open()

	if tok, ok, _ := s.ReadToken("ep1"); !ok || tok != "sess.abc" {
		t.Errorf("token after relaunch: %q, %v", tok, ok)
	}
	if pin, ok, _ := s.ReadPin("ep1"); !ok || pin != "deadbeef" {
		t.Errorf("pin after relaunch: %q, %v", pin, ok)
	}
	if _, ok, _ := s.ReadToken("never"); ok {
		t.Error("an unknown endpoint reported a token")
	}
}

func TestEndpointsReplaceBySameIDAndTrackActive(t *testing.T) {
	s := openTemp(t)
	a := catsclient.Endpoint{ID: "a", Host: "10.0.0.1", Port: 8443, TLS: true, PinnedSHA256: "aa"}
	b := catsclient.Endpoint{ID: "b", Host: "10.0.0.2", Port: 8443, TLS: true}

	if err := s.AddEndpoint(a); err != nil {
		t.Fatal(err)
	}
	if err := s.AddEndpoint(b); err != nil {
		t.Fatal(err)
	}
	if active, ok := s.Active(); !ok || active.ID != "b" {
		t.Errorf("the most recent pairing should be active, got %+v %v", active, ok)
	}

	// Re-pairing with a: same id, new pin, and it becomes active again.
	a2 := a.WithPin("bb")
	if err := s.AddEndpoint(a2); err != nil {
		t.Fatal(err)
	}
	list := s.Endpoints()
	if len(list) != 2 {
		t.Fatalf("re-pairing must replace, not duplicate: %+v", list)
	}
	if list[0].PinnedSHA256 != "bb" {
		t.Errorf("re-pairing did not replace the pin: %+v", list[0])
	}
	if active, _ := s.Active(); active.ID != "a" {
		t.Errorf("re-pairing did not make a active: %+v", active)
	}
}

func TestRemoveEndpointForgetsEverythingUnderIt(t *testing.T) {
	s := openTemp(t)
	a := catsclient.Endpoint{ID: "a", Host: "h", Port: 1}
	b := catsclient.Endpoint{ID: "b", Host: "h", Port: 2}
	_ = s.AddEndpoint(a)
	_ = s.AddEndpoint(b)
	_ = s.WriteToken("b", "t")
	_ = s.WritePin("b", "p")

	if err := s.RemoveEndpoint("b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ReadToken("b"); ok {
		t.Error("token survived RemoveEndpoint")
	}
	if _, ok, _ := s.ReadPin("b"); ok {
		t.Error("pin survived RemoveEndpoint")
	}
	// Active falls back to the remaining endpoint rather than dangling.
	if active, ok := s.Active(); !ok || active.ID != "a" {
		t.Errorf("active after removal: %+v %v", active, ok)
	}
	_ = s.RemoveEndpoint("a")
	if _, ok := s.Active(); ok {
		t.Error("active still set with no endpoints")
	}
}

func TestNoDataDirRunsInMemory(t *testing.T) {
	mobile.SetDataDir("")
	Close()
	t.Cleanup(Close)
	s := Open()
	if s == nil {
		t.Fatal("Open returned nil without a data directory")
	}
	if err := s.WriteToken("x", "y"); err != nil {
		t.Fatal(err)
	}
	if tok, ok, _ := s.ReadToken("x"); !ok || tok != "y" {
		t.Errorf("in-memory read: %q %v", tok, ok)
	}
}
