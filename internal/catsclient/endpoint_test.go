package catsclient

import (
	"strings"
	"testing"
)

// Ported from the Dart suite's endpoint_test.dart (deleted in phase 6). The sha256 vectors are
// gone with the hand-rolled hash: crypto/sha256 is the standard library's.

func TestFingerprintsCompareAcrossFormatting(t *testing.T) {
	if !FingerprintsMatch("AB:CD:EF", "abcdef") {
		t.Error("colons and case should not matter")
	}
	if !FingerprintsMatch("ab cd ef", "ABCDEF") {
		t.Error("spaces and case should not matter")
	}
	if FingerprintsMatch("abcdef", "abcdee") {
		t.Error("different digits must not match")
	}
}

func TestFingerprintSHA256IsTheStandardDigest(t *testing.T) {
	// FIPS 180-4's "abc" vector, so a wrong hash choice is caught here rather
	// than at the first mismatch against catway's printed fingerprint.
	got := FingerprintSHA256([]byte("abc"))
	if got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("sha256(abc) = %s", got)
	}
}

func TestParsePairURIParsesWhatCatctlPairMints(t *testing.T) {
	// Byte-for-byte the shape catctl's pairURI builds: single-letter keys, no
	// expiry. Verified against a live `catctl pair --json` run.
	grant, ok := ParsePairURI("cats://pair?f=aa11bb22&t=abc123&u=https%3A%2F%2F192.168.2.52%3A8443")
	if !ok {
		t.Fatal("expected a grant")
	}
	if grant.Token != "abc123" {
		t.Errorf("token = %q", grant.Token)
	}
	e := grant.Endpoint
	if e.Host != "192.168.2.52" || e.Port != 8443 || !e.TLS || e.PinnedSHA256 != "aa11bb22" {
		t.Errorf("endpoint = %+v", e)
	}
	if e.ID != "192.168.2.52:8443" {
		t.Errorf("id = %q", e.ID)
	}
}

func TestParsePairURIPlainHTTPHasNoPin(t *testing.T) {
	// Because there is nothing to pin.
	grant, ok := ParsePairURI("cats://pair?t=tok&u=http%3A%2F%2F10.0.0.5%3A8421")
	if !ok {
		t.Fatal("expected a grant")
	}
	if grant.Endpoint.TLS {
		t.Error("http is not tls")
	}
	if grant.Endpoint.PinnedSHA256 != "" {
		t.Errorf("pin = %q, want none", grant.Endpoint.PinnedSHA256)
	}
	if got := grant.Endpoint.WSURL(); got != "ws://10.0.0.5:8421/ws" {
		t.Errorf("ws url = %s", got)
	}
}

func TestParsePairURIRejectsEverythingElse(t *testing.T) {
	// Because a camera sees everything.
	for _, raw := range []string{
		"https://example.com",
		"cats://pair",
		"cats://pair?url=nonsense&token=t",
		"cats://pair?t=t&u=not%20a%20url",
		"cats://pair?t=t&u=https%3A%2F%2Fhost%3A99999",
		"WIFI:S:home;T:WPA;P:hunter2;;",
		"",
		"\x00\xff",
	} {
		if _, ok := ParsePairURI(raw); ok {
			t.Errorf("%q parsed; it should not", raw)
		}
	}
}

func TestParsePairURIBuildsAWSURLFromThePairedAddress(t *testing.T) {
	grant, ok := ParsePairURI("cats://pair?u=https%3A%2F%2Fhost%3A9000&t=t")
	if !ok {
		t.Fatal("expected a grant")
	}
	if got := grant.Endpoint.WSURL(); got != "wss://host:9000/ws" {
		t.Errorf("ws url = %s", got)
	}
	if got := grant.Endpoint.HTTPURL(); got != "https://host:9000/" {
		t.Errorf("http url = %s", got)
	}
}

func TestParsePairURIDefaultsThePortFromTheScheme(t *testing.T) {
	grant, _ := ParsePairURI("cats://pair?u=https%3A%2F%2Frelay.example&t=t")
	if grant.Endpoint.Port != 443 {
		t.Errorf("https port = %d, want 443", grant.Endpoint.Port)
	}
	grant, _ = ParsePairURI("cats://pair?u=http%3A%2F%2Fbox&t=t")
	if grant.Endpoint.Port != 80 {
		t.Errorf("http port = %d, want 80", grant.Endpoint.Port)
	}
}

func TestEndpointIPv6IsBracketedInURLs(t *testing.T) {
	e := Endpoint{ID: "v6", Host: "fe80::1", Port: 8443, TLS: true}
	if got := e.WSURL(); got != "wss://[fe80::1]:8443/ws" {
		t.Errorf("ws url = %s", got)
	}
}

func TestDecideCert(t *testing.T) {
	der := []byte("a certificate, pretend")
	fingerprint := FingerprintSHA256(der)

	t.Run("a matching pin is accepted", func(t *testing.T) {
		d := DecideCert(der, fingerprint, "")
		if d.Verdict != VerdictPinned || !d.Accepted() {
			t.Errorf("decision = %+v", d)
		}
	})
	t.Run("the pin from pairing counts before anything is stored", func(t *testing.T) {
		d := DecideCert(der, "", fingerprint)
		if d.Verdict != VerdictPinned {
			t.Errorf("verdict = %v", d.Verdict)
		}
	})
	t.Run("a CHANGED pin is refused, and says so distinctly", func(t *testing.T) {
		// The two rejections need different words. "We have not met" asks
		// the user to pair; "this is not the certificate we pinned" is the
		// one that deserves alarm, and conflating them trains people to tap
		// through both.
		d := DecideCert(der, strings.Repeat("ff", 32), "")
		if d.Verdict != VerdictMismatch || d.Accepted() {
			t.Errorf("decision = %+v", d)
		}
		if d.Fingerprint != fingerprint {
			t.Error("the decision should carry what it saw, so the UI can show it")
		}
	})
	t.Run("an unpinned endpoint is trust-on-first-use", func(t *testing.T) {
		d := DecideCert(der, "", "")
		if d.Verdict != VerdictFirstUse || !d.Accepted() {
			t.Errorf("decision = %+v", d)
		}
	})
}

func TestMemoryTrustStoreKeepsTokensAndPinsApart(t *testing.T) {
	store := NewMemoryTrustStore()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(store.WriteToken("e1", "sess"))
	must(store.WritePin("e1", "aa"))
	if tok, ok, _ := store.ReadToken("e1"); !ok || tok != "sess" {
		t.Errorf("token = %q %v", tok, ok)
	}
	if pin, ok, _ := store.ReadPin("e1"); !ok || pin != "aa" {
		t.Errorf("pin = %q %v", pin, ok)
	}
	must(store.ClearToken("e1"))
	if _, ok, _ := store.ReadToken("e1"); ok {
		t.Error("token should be gone")
	}
	if pin, ok, _ := store.ReadPin("e1"); !ok || pin != "aa" {
		t.Error("signing out must not forget which server this is")
	}
	if _, ok, _ := store.ReadToken("never"); ok {
		t.Error("an unknown endpoint has no token")
	}
}
