//go:build !js

package catsclient

import (
	"net/http"
)

// loginClient builds the HTTP client for a native redemption: the pinned TLS
// config (the same one Dial uses), and an Accept header asking for the JSON
// shape so the session comes back in the body rather than as a cookie.
//
// Redirects are refused. The JSON path never redirects, so following one
// would only ever mean the server misread the Accept header, and a redirect
// to "/" answered with the app's HTML must not be mistaken for a session.
func loginClient(endpoint Endpoint, opts LoginOptions, req *http.Request) *http.Client {
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if endpoint.TLS {
		client.Transport = &http.Transport{
			TLSClientConfig: PinnedTLSConfig(endpoint, opts.StoredPin, opts.OnCertDecision),
		}
	}
	return client
}

// sessionFromLogin reads the native reply: the JSON body.
func sessionFromLogin(_ *http.Response, body []byte) (string, error) {
	return jsonSession(body)
}
