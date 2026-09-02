//go:build js

package catsclient

import (
	"net/http"
)

// loginClient builds the HTTP client for a browser redemption.
//
// No Accept header: the cookie shape is the one a browser can use, since the
// WebSocket upgrade in dial_js.go cannot carry a bearer. The
// "js.fetch:credentials" pseudo-header is Go's syscall/js hook for fetch's
// credentials option; "include" is what makes the browser both store the
// Set-Cookie from this reply and attach it to the later socket upgrade.
//
// The redirect the cookie shape answers with (303 to "/") is followed by the
// browser's fetch itself, so the reply seen here is whatever "/" served: a
// 200 with the app's HTML once the cookie is set. That is why the WASM path
// keys on the status alone and returns no session value.
//
// TLS is the browser's; the pin in LoginOptions is ignored, as it is in
// dial_js.go. A self-signed catway must already be trusted by the browser,
// and the cookie is same-site strict, so the preview works when the page is
// served from the catway's own origin (or a localhost loop) rather than from
// an arbitrary static host.
func loginClient(_ Endpoint, _ LoginOptions, req *http.Request) *http.Client {
	req.Header.Set("js.fetch:credentials", "include")
	return &http.Client{}
}

// sessionFromLogin on WASM has no session to return: the credential is in
// the cookie jar, which the app cannot read and does not need to.
func sessionFromLogin(_ *http.Response, _ []byte) (string, error) {
	return "", nil
}
