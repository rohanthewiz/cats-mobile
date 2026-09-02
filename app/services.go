package catsapp

import (
	"context"
	"sync"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats-mobile/internal/store"
)

// Services holds the app's long-lived objects: the persistence store and the
// one connection to the active catway.
//
// # Why a package singleton rather than something threaded through the tree
//
// grmob's Context carries hook slots, a theme and a config struct, and
// nothing else an app can extend, so the choice is between a singleton and
// threading pointers through every screen signature. A singleton is also
// what the framework itself assumes: mobile.Register installs one root view
// for the process, mobile.DataDir names one writable directory, and bytdb
// takes an exclusive lock on the file underneath it. Same shape as
// church_mobile's services and grmob's own examples/todoapp.
//
// Everything here is safe for concurrent use: renders read it, tap handlers
// mutate it, and the socket's reader goroutine writes into it.
type Services struct {
	Store *store.Store
	Conn  *Connection

	bootOnce sync.Once
}

var (
	servicesOnce sync.Once
	services     *Services
)

// Get returns the process-wide services, building them on first use.
func Get() *Services {
	servicesOnce.Do(func() {
		services = newServices(store.Open(), realDialer)
	})
	return services
}

// newServices builds a Services around an explicit store and dialer. Tests
// use it with a memory store and a fake socket; Get uses it with the real
// ones.
func newServices(st *store.Store, dialer Dialer) *Services {
	return &Services{
		Store: st,
		Conn:  newConnection(st, dialer),
	}
}

// Bind attaches the render loop and runs the boot sequence, exactly once.
// Called from the root view's first render, the earliest point at which a
// Context exists (mobile.Register runs in package init, before there is one).
//
// The boot sequence is one thing: if the phone has an active endpoint, dial
// it. Pairing state is read straight from the store on every pass, so there
// is nothing to restore into memory first.
func (s *Services) Bind(ctx interface{ RequestRender() }) {
	s.bootOnce.Do(func() {
		s.Conn.SetNotify(ctx.RequestRender)
		if endpoint, ok := s.Store.Active(); ok {
			s.Conn.Connect(endpoint)
		}
	})
}

// Paired reports whether the phone has an endpoint to connect to. The shell
// shows the pair screen until this is true.
func (s *Services) Paired() bool {
	_, ok := s.Store.Active()
	return ok
}

// realDialer is the production Dialer: catsclient's own Dial, which on
// native sends the bearer and pins the certificate, and on WASM dials bare
// and relies on the cookie POST /login set (see catsclient/dial_js.go).
func realDialer(ctx context.Context, endpoint catsclient.Endpoint, opts catsclient.DialOptions) (catsclient.Socket, error) {
	socket, err := catsclient.Dial(ctx, endpoint, opts)
	if err != nil {
		// Returning a typed nil through an interface would be a non-nil
		// interface holding a nil pointer; the explicit nil keeps the
		// caller's `socket == nil` honest.
		return nil, err
	}
	return socket, nil
}
