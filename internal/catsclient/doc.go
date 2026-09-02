// Package catsclient is the phone's side of the cats browser protocol: how it
// finds a catway, decides to trust it, holds one WebSocket session with it, and
// folds the down-message stream into state a screen can render.
//
// It is the Go port of the Dart package that preceded it (packages/catsproto),
// file for file, and every test file here mirrors its Dart counterpart so the
// two can be read side by side until the Dart is deleted.
//
// # Layout
//
//	endpoint.go   Endpoint, PairGrant, ParsePairURI, TrustStore
//	trust.go      certificate pinning: DecideCert and the tls.Config hook
//	conn.go       Conn: the handshake, Send's viewer guard, Invoke, the caps
//	dial.go       the real socket (coder/websocket); dial_js.go for WASM
//	backoff.go    the reconnect ladder
//	grid.go       PaneGrid: one pane's cells as a column store
//	session.go    Session: the fold, and the views over it
//
// # The organizing principle
//
// The phone observes and replies; it never rearranges the desktop. catway takes
// the session grid from the first connection that declares one and shares it
// with every other, so a phone honestly reporting 40×20 would reflow every pane
// for everybody at the desk. Four independent layers enforce that a cats client
// is a viewer, and this package is three of them:
//
//  1. Conn builds the wire.Init itself, with hardcoded zeros and Viewer set.
//     There is no exported way to pass one.
//  2. Conn.Send type-switches on an allowlist of up-messages; Resize and a
//     second Init are refused with ViewerModeViolation.
//  3. FollowWorkspace refuses to send workspace.focus to a server that does not
//     advertise wire.CapWindow, where it is a session-wide switch.
//  4. viewer_mode_test.go walks app/ and internal/ with go/ast and fails on any
//     wire.Resize literal or non-zero Cols/Rows key.
//
// # Concurrency
//
// Conn owns one reader goroutine; every decoded message is handed to the
// OnMessage callback from that goroutine. Session has no lock of its own: the
// app holds it behind a pointer with a mutex (the church_mobile hooks pattern)
// and takes that lock around Apply and around every read during a render pass.
// Keeping the lock out of Session keeps the tests plain and the fold pure.
package catsclient
