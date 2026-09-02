// Package catsapp is the cats phone client: the UI, written in Go on the
// grmob framework, over the protocol layer in internal/catsclient.
//
// It is the port of the Flutter app this repository used to be. The Dart
// package under ../packages stays as a reference until phase 6 deletes it;
// the two consume the same wire contract from the cats repo's `wire` package.
//
// # Integration contract
//
// The init below registers the root view with grmob's mobile bridge, which is
// the whole of what a native shell needs. Building for a device is then:
//
//	gomobile bind github.com/rohanthewiz/grmob/mobile ./app
//
// and for the browser preview, ./wasm in this repo (see scripts/wasm.sh).
//
// # Screen map
//
//	Navigator (root)
//	  └─ shell                  pair screen until an endpoint exists, then the
//	       │                    bottom nav: Agents · Windows · Alerts · More
//	       ├─ Agents            the roster, grouped by state, longest-blocked first
//	       ├─ Windows           desktop windows; follow one / follow primary
//	       ├─ Alerts            notifications, the record indicator, runbook runs
//	       └─ More              endpoints, forget device, version + pinned cats sha
//	  └─ pushed: pane viewer (TextGrid + chrome + the reply composer)
//
// # The organizing principle
//
// The phone observes and replies; it never rearranges the desktop. The
// protocol layer enforces that in three places (see catsclient's package doc)
// and this package adds the fourth: nothing here builds a wire.Resize, which
// catsclient's viewer_mode_test walks this directory to prove. The one
// desktop-moving action a screen offers (reveal a pane at the desk) sits
// behind an explicit confirmation and is never a side effect of navigating.
//
// # Hook discipline
//
// grmob hooks are positional slots on the Context: a hook call that happens
// on one pass and not the next shifts every later slot. So no screen calls a
// hook conditionally, every NewState sits at the top of its function, and
// each tab renders in its own ctx.Scope so rendering only the selected tab
// cannot disturb the others' slots. Same rules as church_mobile, for the same
// reason.
package catsapp

import (
	"github.com/rohanthewiz/grmob/core"
	"github.com/rohanthewiz/grmob/mobile"
)

func init() {
	ctx := core.NewContext().WithConfig(&core.AppConfig{
		Name:    "Cats",
		Version: "0.1.0",
		Locale:  "en-US",
	})
	mobile.Register(ctx, App)
}

// AppName exists to be bindable. gobind only links a bound package when it
// references at least one bindable exported symbol; App is not bindable (a
// function-typed parameter), and without this the package, including the
// init that registers the app, would be dropped from the native library,
// leaving the bridge with a nil manager.
func AppName() string { return "Cats" }
