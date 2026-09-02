package catsclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The native redemption against a stand-in for catway's /login handler: the
// same form field, the same two reply shapes, the same 401.

func loginServer(t *testing.T, accept string) (*httptest.Server, Endpoint) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("password") != accept {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid password or pairing token"}`))
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"session":"1700000000.abcd","expires_at":1700000000}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "hsess", Value: "1700000000.abcd", Path: "/"})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	host, port, _ := strings.Cut(u.Host, ":")
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return server, Endpoint{ID: "t", Host: host, Port: p}
}

func TestLoginRedeemsAGrantForASession(t *testing.T) {
	_, endpoint := loginServer(t, "grant-1")
	session, err := Login(context.Background(), endpoint, "grant-1", LoginOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if session != "1700000000.abcd" {
		t.Errorf("session = %q", session)
	}
}

func TestLoginReportsARefusedCredential(t *testing.T) {
	_, endpoint := loginServer(t, "grant-1")
	_, err := Login(context.Background(), endpoint, "stale", LoginOptions{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestLoginReportsAnUnreachableServer(t *testing.T) {
	// A closed port: the dial fails rather than the credential.
	_, err := Login(context.Background(), Endpoint{ID: "t", Host: "127.0.0.1", Port: 1}, "x", LoginOptions{})
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want a transport error, got %v", err)
	}
}
