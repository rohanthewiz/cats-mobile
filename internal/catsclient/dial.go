//go:build !js

package catsclient

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// Dial opens the WebSocket to endpoint with the two hooks a browser client
// cannot have: an Authorization header on the upgrade, and a certificate
// callback. Both are load-bearing. The bearer token IS the credential, and
// catway serves a self-signed certificate that only a pin can validate.
//
// No Origin header is sent. gwauth.OriginOK accepts an empty Origin ("a
// non-browser client; auth is still enforced"), so a native client passes the
// check for free: the header exists to stop a hostile PAGE, and there is no
// page here.
func Dial(ctx context.Context, endpoint Endpoint, opts DialOptions) (*WSSocket, error) {
	client := &http.Client{}
	if endpoint.TLS {
		client.Transport = &http.Transport{
			TLSClientConfig: PinnedTLSConfig(endpoint, opts.StoredPin, opts.OnCertDecision),
		}
	}
	header := http.Header{}
	if opts.Token != "" {
		header.Set("Authorization", "Bearer "+opts.Token)
	}
	conn, _, err := websocket.Dial(ctx, endpoint.WSURL(), &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: header,
	})
	if err != nil {
		return nil, err
	}
	s := newWSSocket(conn)
	interval := opts.PingInterval
	if interval == 0 {
		interval = 20 * time.Second
	}
	go s.keepAlive(interval)
	return s, nil
}
