//go:build js

package catsclient

import (
	"context"

	"github.com/coder/websocket"
)

// Dial opens the WebSocket from a browser. A page cannot set headers on the
// upgrade request, so the credential does not ride the handshake: the app
// redeems its token with POST /login first (a fetch the browser makes with
// credentials) and the session cookie that sets is what the browser attaches
// here. catway accepts both routes. TLS, likewise, is the browser's: the pin
// in DialOptions is ignored, and a self-signed catway must already be trusted
// by the browser, which is why the WASM target is the localhost loop rather
// than a deployment.
func Dial(ctx context.Context, endpoint Endpoint, _ DialOptions) (*WSSocket, error) {
	conn, _, err := websocket.Dial(ctx, endpoint.WSURL(), nil)
	if err != nil {
		return nil, err
	}
	// No keepAlive: a browser cannot send ping frames, and it answers the
	// server's in its own network stack.
	return newWSSocket(conn), nil
}
