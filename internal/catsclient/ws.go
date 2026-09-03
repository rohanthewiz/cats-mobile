package catsclient

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// WSSocket is the real Socket: one coder/websocket connection. Dial builds
// it; the two Dial implementations (dial.go for native, dial_js.go for WASM)
// differ only in how the credential and the certificate pin reach the
// handshake, which is the one thing the two platforms cannot do the same way.
type WSSocket struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// DialOptions is what a dial needs beyond the endpoint.
type DialOptions struct {
	// Token is the session credential, sent as a bearer header on native.
	// Ignored on WASM, where the browser attaches the session cookie itself.
	Token string
	// StoredPin is the fingerprint the trust store holds for this endpoint,
	// "" when none. Native only: a browser does its own TLS.
	StoredPin string
	// OnCertDecision hears the pin verdict for a self-signed certificate, so
	// the app can record a first-use pin or show a mismatch. Native only.
	OnCertDecision func(CertDecision)
	// PingInterval keeps a NAT from dropping an idle flow. catway pings at 30 s
	// and reads with a 90 s deadline, so the default 20 s is comfortably inside
	// its window without being chatty on cellular. Native only: a browser
	// answers pings in its own network stack and cannot send them.
	PingInterval time.Duration
}

// readLimit bounds one incoming frame. The library's default is 32 KiB, and a
// full PaneFrame for a 200×60 desktop grid is several times that as JSON, so
// the default would drop the very first frame after connect.
const readLimit = 16 << 20

func newWSSocket(conn *websocket.Conn) *WSSocket {
	ctx, cancel := context.WithCancel(context.Background())
	conn.SetReadLimit(readLimit)
	return &WSSocket{conn: conn, ctx: ctx, cancel: cancel}
}

// keepAlive pings on a ticker until the socket closes. Run on its own
// goroutine by the native Dial.
func (s *WSSocket) keepAlive(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			pingCtx, cancel := context.WithTimeout(s.ctx, interval)
			err := s.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				// A failed ping means the flow is dead; closing here unblocks
				// Recv with an error so the Conn fails over to reconnect
				// instead of waiting out the server's read deadline.
				_ = s.Close()
				return
			}
		}
	}
}

// Ping implements Pinger over the library's control-frame ping: it returns
// when the peer's pong arrives, or with an error once ctx expires or the
// socket is closed. Conn.Probe calls it on the foreground transition; the
// keep-alive ticker above is the other caller, through the library directly.
func (s *WSSocket) Ping(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

// Recv returns the next text frame. Binary frames are reserved for a future
// packed encoding behind a version bump, so one arriving now is skipped
// rather than surfaced as an empty message.
func (s *WSSocket) Recv() (string, error) {
	for {
		typ, data, err := s.conn.Read(s.ctx)
		if err != nil {
			return "", err
		}
		if typ == websocket.MessageText {
			return string(data), nil
		}
	}
}

func (s *WSSocket) Send(text string) error {
	return s.conn.Write(s.ctx, websocket.MessageText, []byte(text))
}

// Close sends a normal closure and tears the connection down. Idempotent.
func (s *WSSocket) Close() error {
	var err error
	s.once.Do(func() {
		err = s.conn.Close(websocket.StatusNormalClosure, "")
		s.cancel()
	})
	return err
}
