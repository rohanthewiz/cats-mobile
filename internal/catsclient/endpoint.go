package catsclient

import (
	"net"
	"net/url"
	"strconv"
	"sync"
)

// How a client reaches one catway, and how it decides to trust it.
//
// A phone learns its first endpoint from a `catctl pair` QR: a cats://pair URI
// carrying the URL, a single-use grant, and the served certificate's SHA-256
// over the DER. That last field is the whole reason this file has a trust model
// at all: the device pins out of band, at the moment it first learns the
// address, instead of trusting whatever certificate a later connection happens
// to present.

// EndpointKind is where an endpoint sits, which decides how its certificate is
// validated and in what order it is raced.
type EndpointKind int

const (
	// KindDirect is a LAN address or a Tailscale name: catway's own self-signed
	// certificate, trusted by pin.
	KindDirect EndpointKind = iota
	// KindRelay is reached through the relay. Still catway's own certificate
	// (the relay is an SNI byte pipe that terminates nothing and holds no key),
	// so also pinned. A separate kind because it is raced second and its
	// latency budget is different.
	KindRelay
)

func (k EndpointKind) String() string {
	if k == KindRelay {
		return "relay"
	}
	return "direct"
}

// Endpoint is one reachable address for one catway.
type Endpoint struct {
	// ID is the stable key for the token and pin stores. Not a display name.
	ID   string
	Host string
	Port int
	TLS  bool
	Kind EndpointKind
	// PinnedSHA256 is the certificate's SHA-256 over its DER, lowercase hex, or
	// "" when nothing is pinned (plain HTTP, or an address typed in by hand).
	//
	// DER, not the SPKI. catway mints a fresh ECDSA key on every certificate
	// regeneration (internal/gwtls), so SPKI pinning would survive nothing DER
	// pinning does not, and the DER hash is the value openssl and every
	// browser's certificate viewer show, which makes it checkable by hand.
	PinnedSHA256 string
}

// PairGrant is what a scanned pairing QR yields: where to connect, with what
// certificate, and the single-use grant to redeem for a session.
type PairGrant struct {
	Endpoint Endpoint
	// Token is worth minutes and exactly one use. Redeem it at POST /login with
	// password=<token>; what comes back is an ordinary session, which is the
	// credential the device actually keeps.
	//
	// There is no expiry field, because the URI carries none. A grant that ran
	// out answers with a 401, which is a path the app has to handle regardless;
	// a second source of truth for the same fact would only be a way to
	// disagree with the server about whether the code still works.
	Token string
}

// ParsePairURI parses the cats://pair?u=…&t=…&f=… deep link that `catctl pair`
// renders into a QR code.
//
// The keys are single letters because the URI has to fit a scannable symbol: a
// typical LAN pairing URI is 141 bytes, which is a version-8 QR, and cats has a
// test (TestPairURIFitsAQRCode) asserting it stays there.
//
// The grant's expiry is deliberately NOT in the URI for the same reason. It is
// five minutes and single use; the honest signal that it has run out is the
// 401 from redeeming it, which the app has to handle anyway.
//
// Reports false rather than panicking or erroring on anything malformed. This
// arrives from a camera pointed at an arbitrary QR code in the world, so "not
// one of ours" is the common case, not an error.
func ParsePairURI(raw string) (PairGrant, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "cats" || u.Host != "pair" {
		return PairGrant{}, false
	}
	q := u.Query()
	target, token := q.Get("u"), q.Get("t")
	if target == "" || token == "" {
		return PairGrant{}, false
	}
	t, err := url.Parse(target)
	if err != nil || t.Host == "" {
		return PairGrant{}, false
	}
	tls := t.Scheme == "https" || t.Scheme == "wss"
	host, port, ok := splitHostPort(t, tls)
	if !ok {
		return PairGrant{}, false
	}
	return PairGrant{
		Endpoint: Endpoint{
			ID:   net.JoinHostPort(host, strconv.Itoa(port)),
			Host: host,
			Port: port,
			TLS:  tls,
			// Empty when catway is serving plain HTTP, in which case there is
			// nothing to pin and nothing pinning would protect.
			PinnedSHA256: q.Get("f"),
		},
		Token: token,
	}, true
}

// splitHostPort takes the host and port off a parsed URL, defaulting the port
// from the scheme the way a browser would.
func splitHostPort(t *url.URL, tls bool) (host string, port int, ok bool) {
	host = t.Hostname()
	if host == "" {
		return "", 0, false
	}
	if p := t.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", 0, false
		}
		return host, n, true
	}
	if tls {
		return host, 443, true
	}
	return host, 80, true
}

// hostPort is the authority the URLs below carry. IPv6 literals need brackets;
// net.JoinHostPort adds them only when needed.
func (e Endpoint) hostPort() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// WSURL is the WebSocket address: wss://host:port/ws, or ws:// without TLS.
func (e Endpoint) WSURL() string {
	scheme := "ws"
	if e.TLS {
		scheme = "wss"
	}
	return (&url.URL{Scheme: scheme, Host: e.hostPort(), Path: "/ws"}).String()
}

// HTTPURL is the base address for the HTTP side (POST /login, and the rest).
func (e Endpoint) HTTPURL() string {
	scheme := "http"
	if e.TLS {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: e.hostPort(), Path: "/"}).String()
}

// WithPin returns the endpoint with its pinned fingerprint replaced.
func (e Endpoint) WithPin(fingerprint string) Endpoint {
	e.PinnedSHA256 = fingerprint
	return e
}

func (e Endpoint) String() string {
	s := "Endpoint(" + e.ID + ", " + e.Kind.String()
	if e.TLS {
		s += ", tls"
	}
	return s + ")"
}

// TrustStore remembers per-endpoint secrets. The app backs it with bytdb in
// mobile.DataDir(); tests use MemoryTrustStore.
//
// The session token IS the security model: there is no second factor and no
// refresh flow, so it never goes anywhere a preferences file would.
//
// A missing value is reported as ok == false, not as an error. An error is for
// the store itself failing (disk, corruption), which the app treats differently
// from "we have never met this server".
type TrustStore interface {
	ReadToken(endpointID string) (token string, ok bool, err error)
	WriteToken(endpointID, token string) error
	ClearToken(endpointID string) error

	// ReadPin is the pinned certificate fingerprint, or ok == false when this
	// endpoint has never been seen.
	ReadPin(endpointID string) (sha256Hex string, ok bool, err error)
	WritePin(endpointID, sha256Hex string) error
}

// MemoryTrustStore is a TrustStore for tests and for a demo build with no
// server. Safe for concurrent use; the real store is, so this one should not
// let a test pass that the real one would fail.
type MemoryTrustStore struct {
	mu     sync.Mutex
	tokens map[string]string
	pins   map[string]string
}

func NewMemoryTrustStore() *MemoryTrustStore {
	return &MemoryTrustStore{tokens: map[string]string{}, pins: map[string]string{}}
}

func (s *MemoryTrustStore) ReadToken(id string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	return t, ok, nil
}

func (s *MemoryTrustStore) WriteToken(id, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[id] = token
	return nil
}

func (s *MemoryTrustStore) ClearToken(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, id)
	return nil
}

func (s *MemoryTrustStore) ReadPin(id string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pins[id]
	return p, ok, nil
}

func (s *MemoryTrustStore) WritePin(id, sha256Hex string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins[id] = sha256Hex
	return nil
}
