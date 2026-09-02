package catsapp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats-mobile/internal/store"
	"github.com/rohanthewiz/cats/wire"
)

// Connection is the app's one live link to a catway: the reconnect loop,
// the Session it folds messages into, and the status the screens show.
//
// # The shape, as the plan drew it
//
//	WS reader goroutine ──► c.mu.Lock(); session.Apply(msg); Unlock()
//	                     ──► notify()   (ctx.RequestRender: the render manager
//	                                     coalesces a burst of PaneDiffs into
//	                                     one pass, because a pass drains the
//	                                     dirty flag rather than a queue)
//
// The Session has no lock of its own (see catsclient's package doc); this
// type holds it behind mu and takes that lock around Apply and around every
// read a render pass makes through Read. The connection lives at the root
// scope for the life of the process, not in a navigation frame: popping the
// pane screen must stop nothing.
//
// # Generations
//
// Connect may be called again (a new pairing, a switch of endpoint, a retry)
// while an old loop is still winding down. Every loop carries a generation
// number, and a message or status write from a loop that is no longer the
// current one is dropped. Without this an old socket's final "disconnected"
// could overwrite the new socket's "connected".
type Connection struct {
	mu       sync.Mutex
	session  *catsclient.Session
	conn     *catsclient.Conn
	status   Status
	lastErr  error
	endpoint catsclient.Endpoint
	// certSeen is the fingerprint the server presented when the pin refused
	// it, for the mismatch screen to show.
	certSeen string
	// agentsAt is when the last agents rollup arrived. since_ms is an age as
	// of the rollup, and the rollup only goes out on a change, so the label
	// has to tick from here (see AgentAge).
	agentsAt time.Time
	gen      int
	cancel   context.CancelFunc
	// wake is signalled by Retry so a loop sitting in its backoff wait tries
	// again now. Buffer of one: a second Retry during the same wait is the
	// same request.
	wake chan struct{}

	notify func()
	dialer Dialer
	store  *store.Store
}

// Status is where the connection is, for the banner and the More screen.
type Status int

const (
	// StatusIdle: no endpoint, nothing dialled.
	StatusIdle Status = iota
	// StatusConnecting: dialling, or waiting for the welcome.
	StatusConnecting
	StatusConnected
	// StatusReconnecting: the socket dropped and the backoff ladder is
	// running. The last session state stays on screen, marked stale.
	StatusReconnecting
	// StatusCertMismatch: a pin exists and the server's certificate is not
	// it. A hard stop, never retried: the only way out is "forget device"
	// and a fresh pairing.
	StatusCertMismatch
	// StatusUnauthorized: the server refused the credential. The session
	// expired or was revoked; re-pair.
	StatusUnauthorized
	// StatusVersionMismatch: the server speaks a protocol version this app
	// does not. "App update required."
	StatusVersionMismatch
)

func (s Status) String() string {
	switch s {
	case StatusIdle:
		return "Not connected"
	case StatusConnecting:
		return "Connecting…"
	case StatusConnected:
		return "Connected"
	case StatusReconnecting:
		return "Reconnecting…"
	case StatusCertMismatch:
		return "Certificate changed"
	case StatusUnauthorized:
		return "Session expired"
	case StatusVersionMismatch:
		return "App update required"
	}
	return "Unknown"
}

// Dialer opens the transport. It is the seam the tests use to replace the
// network with a fake socket, so every screen test runs the real fold over
// a recorded transcript rather than a mock of the session.
type Dialer func(ctx context.Context, endpoint catsclient.Endpoint, opts catsclient.DialOptions) (catsclient.Socket, error)

func newConnection(st *store.Store, dialer Dialer) *Connection {
	return &Connection{
		session: catsclient.NewSession(),
		store:   st,
		dialer:  dialer,
		wake:    make(chan struct{}, 1),
		notify:  func() {},
	}
}

// SetNotify installs the render trigger. Called once from Bind.
func (c *Connection) SetNotify(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn != nil {
		c.notify = fn
	}
}

// Connect starts (or restarts) the reconnect loop against endpoint. Any
// previous loop is cancelled; its socket closes on its own goroutine, so this
// never blocks a tap handler.
func (c *Connection) Connect(endpoint catsclient.Endpoint) {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.gen++
	gen := c.gen
	c.endpoint = endpoint
	c.status = StatusConnecting
	c.lastErr = nil
	c.certSeen = ""
	c.conn = nil
	c.session = catsclient.NewSession()
	c.mu.Unlock()
	c.notify()

	go c.run(ctx, gen, endpoint)
}

// Disconnect stops the loop and drops the session. Used by "forget device"
// and by switching endpoints.
func (c *Connection) Disconnect() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.gen++
	c.status = StatusIdle
	c.conn = nil
	c.session = catsclient.NewSession()
	c.mu.Unlock()
	c.notify()
}

// Retry asks a loop that is waiting out its backoff to try now, and restarts
// a loop that stopped on a hard error (the user tapped "try again" after
// fixing something). A user who asks is saying they think it should work;
// making them wait out a 30 s ladder answers that with a shrug.
func (c *Connection) Retry() {
	c.mu.Lock()
	status := c.status
	endpoint := c.endpoint
	running := c.cancel != nil
	c.mu.Unlock()

	if running && status == StatusReconnecting {
		select {
		case c.wake <- struct{}{}:
		default:
		}
		return
	}
	if endpoint.Host != "" {
		c.Connect(endpoint)
	}
}

// run is one generation of the reconnect loop. It returns when ctx is
// cancelled or on a hard stop (certificate mismatch, refused credential,
// protocol mismatch); every other failure walks the backoff ladder.
func (c *Connection) run(ctx context.Context, gen int, endpoint catsclient.Endpoint) {
	backoff := catsclient.NewBackoff(nil)
	for {
		if !c.setStatus(gen, StatusConnecting, nil) {
			return
		}

		token, _, _ := c.store.ReadToken(endpoint.ID)
		pin, _, _ := c.store.ReadPin(endpoint.ID)
		if pin == "" {
			// The pairing QR carried one; the dial should check against it
			// even before the first successful connect has stored it.
			pin = endpoint.PinnedSHA256
		}

		// The decision callback runs synchronously inside the TLS handshake,
		// so by the time the dial returns it has either fired or there was
		// no self-signed certificate to decide about.
		var decision *catsclient.CertDecision
		socket, err := c.dialer(ctx, endpoint, catsclient.DialOptions{
			Token:     token,
			StoredPin: pin,
			OnCertDecision: func(d catsclient.CertDecision) {
				decision = &d
			},
		})
		if ctx.Err() != nil {
			if socket != nil {
				_ = socket.Close()
			}
			return
		}
		if decision != nil && decision.Verdict == catsclient.VerdictFirstUse {
			// Trust on first use: remember what we saw, so a later change is
			// a mismatch rather than another first use.
			_ = c.store.WritePin(endpoint.ID, decision.Fingerprint)
		}
		if err != nil {
			switch {
			case errors.Is(err, catsclient.ErrCertMismatch):
				seen := ""
				if decision != nil {
					seen = decision.Fingerprint
				}
				c.mu.Lock()
				if gen == c.gen {
					c.certSeen = seen
				}
				c.mu.Unlock()
				c.setStatus(gen, StatusCertMismatch, err)
				return
			case isUnauthorized(err):
				c.setStatus(gen, StatusUnauthorized, err)
				return
			}
			if !c.setStatus(gen, StatusReconnecting, err) {
				return
			}
			if !c.wait(ctx, backoff.Next()) {
				return
			}
			continue
		}

		// A new socket: the grids from the old one are corruption waiting to
		// happen (diff indices relative to another frame's width), so the
		// session drops them before the first message can arrive.
		c.mu.Lock()
		if gen != c.gen {
			c.mu.Unlock()
			_ = socket.Close()
			return
		}
		c.session.ResetForNewSocket()
		conn := catsclient.New(socket, catsclient.Options{
			Endpoint:  endpoint,
			OnMessage: func(msg any) { c.onMessage(gen, msg) },
		})
		c.conn = conn
		c.mu.Unlock()

		if _, err := conn.Welcome(ctx); err != nil {
			_ = conn.Close()
			if ctx.Err() != nil {
				return
			}
			var mismatch *catsclient.VersionMismatchError
			var refused *catsclient.CommandError
			if errors.As(err, &mismatch) || errors.As(err, &refused) {
				// The server said no to the handshake itself. Retrying
				// cannot change its mind; the app needs updating (or the
				// server does).
				c.setStatus(gen, StatusVersionMismatch, err)
				return
			}
			if !c.setStatus(gen, StatusReconnecting, err) {
				return
			}
			if !c.wait(ctx, backoff.Next()) {
				return
			}
			continue
		}

		backoff.Reset()
		if !c.setStatus(gen, StatusConnected, nil) {
			_ = conn.Close()
			return
		}

		select {
		case <-ctx.Done():
			_ = conn.Close()
			return
		case <-conn.Done():
		}
		cause := conn.Err()
		c.mu.Lock()
		if gen == c.gen && c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		if !c.setStatus(gen, StatusReconnecting, cause) {
			return
		}
		if !c.wait(ctx, backoff.Next()) {
			return
		}
	}
}

// wait sleeps out one backoff step, cut short by Retry or cancellation.
// Reports false when the loop should stop.
func (c *Connection) wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-c.wake:
		return true
	case <-timer.C:
		return true
	}
}

// setStatus records a status change for generation gen, and reports false
// if that generation is no longer the current one (so the loop exits).
func (c *Connection) setStatus(gen int, status Status, err error) bool {
	c.mu.Lock()
	if gen != c.gen {
		c.mu.Unlock()
		return false
	}
	c.status = status
	c.lastErr = err
	c.mu.Unlock()
	c.notify()
	return true
}

// onMessage is the fold: runs on the socket's reader goroutine, one message
// at a time, under the lock a render pass also takes.
func (c *Connection) onMessage(gen int, msg any) {
	c.mu.Lock()
	if gen != c.gen {
		c.mu.Unlock()
		return
	}
	c.session.Apply(msg)
	if _, ok := msg.(*wire.Agents); ok {
		c.agentsAt = time.Now()
	}
	c.mu.Unlock()
	c.notify()
}

// Read runs fn with the session under the lock. Every render-pass read goes
// through here; fn must not block and must not call back into Connection.
func (c *Connection) Read(fn func(s *catsclient.Session)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c.session)
}

// Info is a snapshot of the connection's state for a render pass.
type Info struct {
	Status   Status
	Err      error
	Endpoint catsclient.Endpoint
	CertSeen string
	// Caps is the server's advertised capability set, empty when not
	// connected. Screens gate window-following and pane-addressed input on
	// it.
	Caps []string
}

func (c *Connection) Info() Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	info := Info{
		Status:   c.status,
		Err:      c.lastErr,
		Endpoint: c.endpoint,
		CertSeen: c.certSeen,
	}
	if c.conn != nil {
		info.Caps = c.conn.Caps()
	}
	return info
}

// HasCap reports whether the live server advertised a capability. False
// when disconnected.
func (c *Connection) HasCap(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.conn.HasCap(name)
}

// Live is the current Conn for issuing commands, or nil while disconnected.
// Callers hold it only for the duration of one command; a reconnect makes a
// new one.
func (c *Connection) Live() *catsclient.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}
	return c.conn
}

// AgentAge is how long an agent has been in its state, as of now: the
// rollup's since_ms plus the time since the rollup arrived. Negative
// since_ms means the age was never published; that is reported as -1 so a
// label can say nothing rather than "0s".
func (c *Connection) AgentAge(item wire.AgentItem) time.Duration {
	if item.SinceMs < 0 {
		return -1
	}
	c.mu.Lock()
	at := c.agentsAt
	c.mu.Unlock()
	age := time.Duration(item.SinceMs) * time.Millisecond
	if !at.IsZero() {
		age += time.Since(at)
	}
	return age
}

// isUnauthorized recognises a refused credential in a dial error.
//
// coder/websocket reports a non-101 upgrade as an error whose text carries
// the status code, and there is no typed error to unwrap for it. Matching on
// the code is a string test, and it is here in one place with this note so
// that a library change that reshapes the message breaks one function.
func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "403")
}
