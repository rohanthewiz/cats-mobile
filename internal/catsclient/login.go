package catsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Login is the pairing redemption: POST /login with the grant from the QR
// (or the shared password typed by hand) and receive the session credential
// the device keeps from then on.
//
// # Two platforms, one endpoint
//
// catway's handler answers in two shapes, chosen by the Accept header. A
// caller asking for JSON gets the session value in the body, which is what a
// native client wants: it has no cookie jar, and the value goes straight into
// the Authorization header of every later dial (see dial.go). A caller that
// does NOT ask for JSON gets a Set-Cookie and a redirect into the app, which
// is what a browser wants: it cannot set upgrade headers on a WebSocket, so
// the cookie the browser attaches by itself is the only way the credential
// can reach the socket (see dial_js.go).
//
// The two files login_native.go and login_js.go choose the shape; this file
// is the request and the error handling they share.
//
// # Errors
//
// A 401 is ErrUnauthorized: the grant expired, was already redeemed, or the
// password is wrong. The server's own message is kept for the screen, since
// it distinguishes nothing further and the app's wording ("scan a fresh QR")
// is the useful part. Anything else is a transport or server failure and is
// returned as is.
var ErrUnauthorized = errors.New("catsclient: the server refused the credential")

// LoginOptions is what a redemption needs beyond the endpoint and the
// credential.
type LoginOptions struct {
	// StoredPin and OnCertDecision are the same hooks Dial takes: the login
	// is the FIRST contact with a freshly paired server, so this is where a
	// pairing pin is checked against the certificate actually served, and
	// where a hand-typed endpoint gets its trust-on-first-use pin. Native
	// only; a browser does its own TLS.
	StoredPin      string
	OnCertDecision func(CertDecision)
	// Timeout bounds the whole round trip. 15 s when zero: a LAN answers in
	// milliseconds and a relay in under a second, so anything longer is a
	// server that is not there, and the pair screen should say so rather
	// than spin.
	Timeout time.Duration
}

// Login redeems credential at endpoint and returns the session value.
//
// On WASM the returned session is empty and the credential lives in the
// browser's cookie jar; the app records that it is paired and dials without
// a token. On native the returned value is the bearer token.
func Login(ctx context.Context, endpoint Endpoint, credential string, opts LoginOptions) (string, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	form := url.Values{"password": {credential}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(endpoint.HTTPURL(), "/")+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := loginClient(endpoint, opts, req)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return "", ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", fmt.Errorf("catsclient: login answered %s", resp.Status)
	}
	return sessionFromLogin(resp, body)
}

// jsonSession is the native reply shape: {"session": "...", "expires_at": n}.
func jsonSession(body []byte) (string, error) {
	var reply struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return "", fmt.Errorf("catsclient: login reply is not JSON: %w", err)
	}
	if reply.Session == "" {
		return "", errors.New("catsclient: login reply carried no session")
	}
	return reply.Session, nil
}
