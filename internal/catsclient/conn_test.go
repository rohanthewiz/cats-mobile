package catsclient

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/wire"
)

// Ported from the Dart suite's connection_test.dart (deleted in phase 6), plus the "picking a
// window" half of views_test.dart, which exercised the connection.

func newTestConn(t *testing.T, socket *fakeSocket, opts Options) *Conn {
	t.Helper()
	opts.Endpoint = testEndpoint
	c := New(socket, opts)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// --- handshake -----------------------------------------------------------------

func TestHandshakeDeclaresNothing(t *testing.T) {
	// The phone never sizes the desktop grid.
	socket := newFakeSocket()
	newTestConn(t, socket, Options{})

	if socket.sentCount() != 1 {
		t.Fatalf("sent %d messages, want only the handshake", socket.sentCount())
	}
	init := socket.sentAt(0)
	if init["t"] != "init" {
		t.Errorf("t = %v; the discriminator must be stamped, wire.Marshal does not do it", init["t"])
	}
	if init["v"] != float64(wire.ProtocolVersion) {
		t.Errorf("v = %v", init["v"])
	}
	// catway's registerConn has TWO guards, both keyed on > 0: the session
	// grid, and the cell metrics that ride β create_pane/resize. Clearing one
	// and not the other is the easy mistake, so both are asserted.
	for _, k := range []string{"cols", "rows", "cell_w_px", "cell_h_px"} {
		if init[k] != float64(0) {
			t.Errorf("%s = %v, want 0", k, init[k])
		}
	}
	if init["viewer"] != true {
		t.Error("viewer must be declared")
	}
}

func TestHandshakeCompletesWelcomeAndRecordsCaps(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	socket.deliver(welcomeWith(wire.CapViewer, wire.CapKeyPane, wire.CapClients))

	w, err := conn.Welcome(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if w.V != wire.ProtocolVersion {
		t.Errorf("v = %d", w.V)
	}
	if !conn.HasCap(wire.CapKeyPane) {
		t.Error("key.pane should be recorded")
	}
	if conn.HasCap(wire.CapWindow) {
		t.Error("window was not advertised")
	}
	if len(conn.Caps()) != 3 {
		t.Errorf("caps = %v", conn.Caps())
	}
}

func TestHandshakeARejectedWelcomeFailsRatherThanHanging(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	socket.deliver(map[string]any{"t": "welcome", "v": 2, "error": "protocol version"})

	_, err := conn.Welcome(context.Background())
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Command != "init" {
		t.Errorf("err = %v, want a CommandError on init", err)
	}
}

func TestHandshakeAWelcomeOnAnotherVersionIsAppUpdateRequired(t *testing.T) {
	// Gate 3 of the lockstep story: the server accepted us but speaks a
	// protocol this build cannot read.
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	socket.deliver(map[string]any{"t": "welcome", "v": wire.ProtocolVersion + 1})

	_, err := conn.Welcome(context.Background())
	var vm *VersionMismatchError
	if !errors.As(err, &vm) || vm.Server != wire.ProtocolVersion+1 {
		t.Errorf("err = %v, want a VersionMismatchError", err)
	}
}

func TestHandshakeWelcomeHonoursTheCallersContext(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := conn.Welcome(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
}

// --- viewer mode ------------------------------------------------------------------

func TestSendRefusesResize(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	// Constructing one here is the only place in the module that does, and
	// viewer_mode_test.go skips test files for exactly this reason.
	var violation *ViewerModeViolation
	if err := conn.Send(wire.Resize{Cols: 40, Rows: 20}); !errors.As(err, &violation) {
		t.Errorf("by value: err = %v", err)
	}
	if err := conn.Send(&wire.Resize{Cols: 40, Rows: 20}); !errors.As(err, &violation) {
		t.Errorf("by pointer: err = %v", err)
	}
	if socket.sentCount() != 1 {
		t.Error("only the handshake should have gone out")
	}
}

func TestSendRefusesASecondInit(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	err := conn.Send(wire.Init{V: wire.ProtocolVersion, Cols: 200, Rows: 60, DPR: 3, CellWPx: 8, CellHPx: 16})
	var violation *ViewerModeViolation
	if !errors.As(err, &violation) {
		t.Errorf("err = %v", err)
	}
}

func TestSendRefusesFocusAndRaw(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	if err := conn.Send(wire.Focus{Focused: true}); err == nil {
		t.Error("a viewer's focus must not un-park a TUI's caret at the desk")
	}
	if err := conn.Send(wire.Raw{Data: []byte("x")}); err == nil {
		t.Error("a viewer never pre-encodes")
	}
	if socket.sentCount() != 1 {
		t.Error("nothing but the handshake should have gone out")
	}
}

func TestSendOrdinaryUpMessagesGoThroughStamped(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	if err := conn.Send(wire.Key{Pane: 3, Code: "Escape", Key: "Escape", Kind: wire.KeyDown}); err != nil {
		t.Fatal(err)
	}
	last := socket.last()
	if last["t"] != "key" {
		t.Errorf("t = %v; the discriminator is stamped even when the caller left it empty", last["t"])
	}
	if last["pane"] != float64(3) {
		t.Errorf("pane = %v", last["pane"])
	}
	if _, has := last["mods"]; !has {
		t.Error("mods has no omitempty in Go, so the server always sees the key")
	}

	if err := conn.Send(&wire.Paste{Data: "hi"}); err != nil {
		t.Fatal(err)
	}
	if socket.last()["t"] != "paste" {
		t.Errorf("t = %v", socket.last()["t"])
	}
	if _, has := socket.last()["pane"]; has {
		t.Error("pane 0 means \"the focused pane\" and must be ABSENT, not 0")
	}
}

func TestSendAfterCloseIsDisconnected(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	_ = conn.Close()
	var disc *DisconnectedError
	if err := conn.Send(wire.Paste{Data: "x"}); !errors.As(err, &disc) {
		t.Errorf("err = %v", err)
	}
}

// --- command correlation ----------------------------------------------------------

// call runs fn on a goroutine and returns a channel with its error, so a test
// can answer the command it sends.
func call[R any](fn func() (R, error)) <-chan struct {
	v   R
	err error
} {
	ch := make(chan struct {
		v   R
		err error
	}, 1)
	go func() {
		v, err := fn()
		ch <- struct {
			v   R
			err error
		}{v, err}
	}()
	return ch
}

func TestInvokeAReplyResolvesTheMatchingCall(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	ctx := context.Background()

	done := call(func() (wire.CaptureResult, error) {
		return conn.Capture(ctx, wire.CaptureParams{Pane: 1, Scope: 1, Lines: 200, Unwrap: true})
	})
	socket.waitSent(t, 2)
	sent := socket.last()
	if sent["t"] != "cmd" || sent["name"] != wire.CmdCapture {
		t.Errorf("sent = %v", sent)
	}
	if sent["id"] == nil || sent["id"] == "" {
		t.Error("capture is reply-gated: no id, no run")
	}
	if sent["params"].(map[string]any)["unwrap"] != true {
		t.Errorf("params = %v", sent["params"])
	}

	socket.deliver(map[string]any{"t": "cmd_result", "id": sent["id"], "ok": true,
		"data": map[string]any{"text": "hello from the pane"}})
	r := <-done
	if r.err != nil || r.v.Text != "hello from the pane" {
		t.Errorf("result = %+v, %v", r.v, r.err)
	}
}

func TestInvokeAnOkFalseReplyErrorsWithTheCommandNameAttached(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})

	done := call(func() (struct{}, error) {
		return struct{}{}, conn.PaneSendInput(context.Background(), wire.SendInputParams{Pane: 1, Text: "hi", Submit: true})
	})
	socket.waitSent(t, 2)
	socket.deliver(map[string]any{"t": "cmd_result", "id": socket.last()["id"], "ok": false, "error": "workspace is locked"})

	r := <-done
	var cmdErr *CommandError
	if !errors.As(r.err, &cmdErr) {
		t.Fatalf("err = %v", r.err)
	}
	if cmdErr.Command != wire.CmdPaneSendInput || cmdErr.Message != "workspace is locked" {
		t.Errorf("err = %+v", cmdErr)
	}
}

func TestInvokeConcurrentCallsDoNotCrossTheirReplies(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	ctx := context.Background()

	a := call(func() (wire.CaptureResult, error) { return conn.Capture(ctx, wire.CaptureParams{Pane: 1}) })
	socket.waitSent(t, 2)
	idA := socket.last()["id"]
	b := call(func() (wire.CaptureResult, error) { return conn.Capture(ctx, wire.CaptureParams{Pane: 2}) })
	socket.waitSent(t, 3)
	idB := socket.last()["id"]
	if idA == idB {
		t.Fatalf("both calls got id %v", idA)
	}

	// Answer out of order, which is the normal case, since the two panes'
	// captures resolve independently server-side.
	socket.deliver(map[string]any{"t": "cmd_result", "id": idB, "ok": true, "data": map[string]any{"text": "B"}})
	socket.deliver(map[string]any{"t": "cmd_result", "id": idA, "ok": true, "data": map[string]any{"text": "A"}})
	if r := <-a; r.err != nil || r.v.Text != "A" {
		t.Errorf("a = %+v %v", r.v, r.err)
	}
	if r := <-b; r.err != nil || r.v.Text != "B" {
		t.Errorf("b = %+v %v", r.v, r.err)
	}
}

func TestInvokeEveryPendingCallFailsOnDisconnect(t *testing.T) {
	// The property that matters: a leaked waiter is a spinner that never
	// stops, and the user has no way to make it stop short of killing the app.
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	ctx := context.Background()

	a := call(func() (wire.CaptureResult, error) { return conn.Capture(ctx, wire.CaptureParams{Pane: 1}) })
	b := call(func() (wire.PaneListResult, error) { return conn.PaneList(ctx) })
	socket.waitSent(t, 3)
	socket.drop()

	var disc *DisconnectedError
	if r := <-a; !errors.As(r.err, &disc) || disc.Command != wire.CmdCapture {
		t.Errorf("a: err = %v", r.err)
	}
	if r := <-b; !errors.As(r.err, &disc) || disc.Command != wire.CmdPaneList {
		t.Errorf("b: err = %v", r.err)
	}
	if !conn.IsClosed() {
		t.Error("the connection should know it is closed")
	}
	// And a welcome nobody received fails too, rather than blocking forever.
	if _, err := conn.Welcome(ctx); !errors.As(err, &disc) {
		t.Errorf("welcome: err = %v", err)
	}
}

func TestInvokeACallTimesOutRatherThanWaitingForever(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{DefaultTimeout: 30 * time.Millisecond})
	_, err := conn.Capture(context.Background(), wire.CaptureParams{Pane: 1})
	var to *TimeoutError
	if !errors.As(err, &to) || to.Command != wire.CmdCapture {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("a timeout should also read as a deadline, for callers that already handle those")
	}
}

func TestInvokeWaitForOutputGetsTheLongDeadline(t *testing.T) {
	// app.MaxWaitTimeout is ten minutes; a client deadline shorter than that
	// would abandon a wait the server is still faithfully serving.
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{DefaultTimeout: 10 * time.Millisecond, MaxTimeout: time.Second})
	done := call(func() (wire.WaitForOutputResult, error) {
		return conn.PaneWaitForOutput(context.Background(), wire.WaitForOutputParams{Pane: 1, Pattern: "$ "})
	})
	socket.waitSent(t, 2)
	time.Sleep(40 * time.Millisecond) // well past DefaultTimeout
	socket.deliver(map[string]any{"t": "cmd_result", "id": socket.last()["id"], "ok": true,
		"data": map[string]any{"matched": true, "text": "$ "}})
	if r := <-done; r.err != nil || !r.v.Matched {
		t.Errorf("result = %+v %v", r.v, r.err)
	}
}

func TestInvokeALateReplyToATimedOutCallDoesNotBlowUp(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{DefaultTimeout: 20 * time.Millisecond})
	var seen []any
	var mu sync.Mutex
	conn.onMessage = func(m any) { mu.Lock(); seen = append(seen, m); mu.Unlock() }

	_, err := conn.Capture(context.Background(), wire.CaptureParams{Pane: 1})
	if err == nil {
		t.Fatal("expected a timeout")
	}
	socket.deliver(map[string]any{"t": "cmd_result", "id": socket.last()["id"], "ok": true, "data": map[string]any{"text": "late"}})
	// The late reply still reaches the message stream, where a debug view
	// can see it.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the late reply never reached OnMessage")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInvokeHonoursTheCallersContext(t *testing.T) {
	socket := newFakeSocket()
	conn := newTestConn(t, socket, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := call(func() (wire.CaptureResult, error) { return conn.Capture(ctx, wire.CaptureParams{Pane: 1}) })
	socket.waitSent(t, 2)
	cancel()
	if r := <-done; !errors.Is(r.err, context.Canceled) {
		t.Errorf("err = %v", r.err)
	}
	// The abandoned call left nothing behind.
	conn.mu.Lock()
	n := len(conn.pending)
	conn.mu.Unlock()
	if n != 0 {
		t.Errorf("%d pending calls after cancel", n)
	}
}

// --- message stream -----------------------------------------------------------------

func TestMessagesPublishesDecodedMessagesAndDropsUnknownOnes(t *testing.T) {
	socket := newFakeSocket()
	var mu sync.Mutex
	var seen []any
	conn := newTestConn(t, socket, Options{OnMessage: func(m any) {
		mu.Lock()
		seen = append(seen, m)
		mu.Unlock()
	}})

	socket.deliver(map[string]any{"t": "title", "title": "cats"})
	socket.deliver(map[string]any{"t": "from_a_newer_server"})
	socket.deliverRaw("{not json at all")
	socket.deliver(map[string]any{"t": "pane_exited", "pane": 2, "code": 130})
	// A sentinel the test can wait on, so it is not racing the reader.
	socket.deliver(map[string]any{"t": "shutdown"})

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("seen %d messages: %#v", len(seen), seen)
	}
	if _, ok := seen[0].(*wire.Title); !ok {
		t.Errorf("seen[0] = %T", seen[0])
	}
	if e, ok := seen[1].(*wire.PaneExited); !ok || e.Code != 130 {
		t.Errorf("seen[1] = %#v", seen[1])
	}
	if !conn.IsClosed() == false {
		t.Error("shutdown is a message, not a socket close")
	}
}

// --- Backoff -------------------------------------------------------------------------

func TestBackoffClimbsToTheCeilingAndResets(t *testing.T) {
	b := NewBackoff(rand.New(rand.NewPCG(1, 2)))
	within := func(d time.Duration, lo, hi time.Duration) bool { return d >= lo && d <= hi }
	if first := b.Next(); !within(first, 400*time.Millisecond, 600*time.Millisecond) {
		t.Errorf("first = %s", first)
	}
	for range 10 {
		b.Next()
	}
	if d := b.Next(); !within(d, 24*time.Second, 36*time.Second) {
		t.Errorf("at the ceiling = %s", d)
	}
	b.Reset()
	if d := b.Next(); !within(d, 400*time.Millisecond, 600*time.Millisecond) {
		t.Errorf("after reset = %s", d)
	}
}

func TestBackoffJitters(t *testing.T) {
	// So a network that came back does not produce a lockstep herd.
	b := NewBackoff(rand.New(rand.NewPCG(7, 7)))
	samples := map[time.Duration]bool{}
	for range 20 {
		b.Reset()
		samples[b.Next()] = true
	}
	if len(samples) <= 5 {
		t.Errorf("only %d distinct delays in 20 draws", len(samples))
	}
}

func TestBackoffWithoutASourceStillWorks(t *testing.T) {
	b := NewBackoff(nil)
	if d := b.Next(); d < 400*time.Millisecond || d > 600*time.Millisecond {
		t.Errorf("first = %s", d)
	}
}

// --- picking a window (from views_test.dart) ------------------------------------------

// connected is a connection with the welcome already delivered, since every
// follow call waits for the capability set.
func connected(t *testing.T, socket *fakeSocket, workspace string, caps ...string) *Conn {
	t.Helper()
	conn := newTestConn(t, socket, Options{Workspace: workspace})
	socket.deliver(welcomeWith(caps...))
	if _, err := conn.Welcome(context.Background()); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestPinTheHandshakeCarriesItAndOmitsItWhenThereIsNone(t *testing.T) {
	unpinned := newFakeSocket()
	newTestConn(t, unpinned, Options{})
	if _, has := unpinned.sentAt(0)["workspace"]; has {
		t.Error("absent means \"follow the primary view\"")
	}

	pinned := newFakeSocket()
	conn := newTestConn(t, pinned, Options{Workspace: "w2"})
	init := pinned.sentAt(0)
	// A reconnect lands back on the same window instead of flicking to the
	// desk's for a frame.
	if init["workspace"] != "w2" {
		t.Errorf("workspace = %v", init["workspace"])
	}
	if conn.PinnedWorkspace() != "w2" || conn.FollowsPrimaryView() {
		t.Error("the pin should be held")
	}
	// Still declares no geometry: a pin is not a size.
	if init["cols"] != float64(0) || init["rows"] != float64(0) || init["viewer"] != true {
		t.Errorf("init = %v", init)
	}
}

func TestFollowWorkspaceSendsWorkspaceFocusAndHoldsThePin(t *testing.T) {
	socket := newFakeSocket()
	conn := connected(t, socket, "", wire.CapWindow)

	done := call(func() (struct{}, error) { return struct{}{}, conn.FollowWorkspace(context.Background(), "w2") })
	socket.waitSent(t, 2)
	cmd := socket.last()
	if cmd["t"] != "cmd" || cmd["name"] != wire.CmdWorkspaceFocus {
		t.Errorf("cmd = %v", cmd)
	}
	if cmd["params"].(map[string]any)["id"] != "w2" {
		t.Errorf("params = %v", cmd["params"])
	}
	socket.deliver(map[string]any{"t": "cmd_result", "id": cmd["id"], "ok": true})
	if r := <-done; r.err != nil {
		t.Fatal(r.err)
	}
	if conn.PinnedWorkspace() != "w2" {
		t.Errorf("pin = %q", conn.PinnedWorkspace())
	}
}

func TestFollowPrimaryViewReleasesItWithAnEmptyID(t *testing.T) {
	socket := newFakeSocket()
	conn := connected(t, socket, "w2", wire.CapWindow)

	done := call(func() (struct{}, error) { return struct{}{}, conn.FollowPrimaryView(context.Background()) })
	socket.waitSent(t, 2)
	cmd := socket.last()
	if cmd["params"].(map[string]any)["id"] != "" {
		t.Error("empty is the wire form of \"follow the primary\"")
	}
	socket.deliver(map[string]any{"t": "cmd_result", "id": cmd["id"], "ok": true})
	if r := <-done; r.err != nil {
		t.Fatal(r.err)
	}
	if !conn.FollowsPrimaryView() {
		t.Error("the pin should be released")
	}
}

func TestFollowARejectedFollowLeavesThePinAlone(t *testing.T) {
	socket := newFakeSocket()
	conn := connected(t, socket, "w1", wire.CapWindow)

	done := call(func() (struct{}, error) { return struct{}{}, conn.FollowWorkspace(context.Background(), "w9") })
	socket.waitSent(t, 2)
	socket.deliver(map[string]any{"t": "cmd_result", "id": socket.last()["id"], "ok": false, "error": "unknown workspace w9"})

	var cmdErr *CommandError
	if r := <-done; !errors.As(r.err, &cmdErr) {
		t.Errorf("err = %v", r.err)
	}
	// A pin the server refused must not survive into the next handshake,
	// where it would be silently fallen back a second time.
	if conn.PinnedWorkspace() != "w1" {
		t.Errorf("pin = %q, want the old one", conn.PinnedWorkspace())
	}
}

func TestFollowRefusesAServerWithoutTheWindowCapability(t *testing.T) {
	socket := newFakeSocket()
	conn := connected(t, socket, "", wire.CapViewer, wire.CapClients)
	before := socket.sentCount()

	err := conn.FollowWorkspace(context.Background(), "w2")
	var violation *ViewerModeViolation
	if !errors.As(err, &violation) {
		t.Errorf("err = %v", err)
	}
	// Nothing went on the wire: there, workspace.focus is the old
	// session-wide switch and would move every window at the desk.
	if socket.sentCount() != before {
		t.Error("something was sent")
	}
	if !conn.FollowsPrimaryView() {
		t.Error("the pin should be untouched")
	}
}
