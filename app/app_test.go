package catsapp

import (
	"context"
	"encoding/json"
	"errors"
	gohtml "html"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats-mobile/internal/store"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
	"github.com/rohanthewiz/grmob/htmlout"
	"github.com/rohanthewiz/grmob/mobile"
	"github.com/rohanthewiz/grmob/render"
)

// Tests that drive the app the way the native shells do (render.New,
// RenderInitial, DispatchCallback) against a fake desk: a Dialer that hands
// out in-memory sockets the test pushes wire messages through. Every screen
// therefore runs the real fold (catsclient.Session) over a real transcript,
// which is the whole reason the Socket seam exists.

// fakeSocket is one in-memory transport; the desk hands out one per dial.
type fakeSocket struct {
	in   chan string
	once sync.Once
	mu   sync.Mutex
	sent []map[string]any
}

func newFakeSocket() *fakeSocket { return &fakeSocket{in: make(chan string, 64)} }

func (s *fakeSocket) Recv() (string, error) {
	text, ok := <-s.in
	if !ok {
		return "", io.EOF
	}
	return text, nil
}

func (s *fakeSocket) Send(text string) error {
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return err
	}
	s.mu.Lock()
	s.sent = append(s.sent, m)
	s.mu.Unlock()
	return nil
}

func (s *fakeSocket) Close() error {
	s.once.Do(func() { close(s.in) })
	return nil
}

// drop simulates the network going away under the connection.
func (s *fakeSocket) drop() { _ = s.Close() }

func (s *fakeSocket) deliver(m map[string]any) {
	raw, _ := json.Marshal(m)
	s.in <- string(raw)
}

// sentNamed returns every sent message whose "t" (or, for commands, whose
// "name") matches.
func (s *fakeSocket) sentNamed(name string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, m := range s.sent {
		if m["t"] == name || m["name"] == name {
			out = append(out, m)
		}
	}
	return out
}

// fakeDesk is the Dialer: it records every dial and can be told to fail.
type fakeDesk struct {
	mu      sync.Mutex
	sockets []*fakeSocket
	fail    error
	opts    []catsclient.DialOptions
}

func (d *fakeDesk) dial(ctx context.Context, _ catsclient.Endpoint, opts catsclient.DialOptions) (catsclient.Socket, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opts = append(d.opts, opts)
	if d.fail != nil {
		return nil, d.fail
	}
	s := newFakeSocket()
	d.sockets = append(d.sockets, s)
	return s, nil
}

func (d *fakeDesk) dials() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.opts)
}

// socket waits for the n-th dial (0-based) and returns its socket.
func (d *fakeDesk) socket(t *testing.T, n int) *fakeSocket {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if len(d.sockets) > n {
			s := d.sockets[n]
			d.mu.Unlock()
			return s
		}
		d.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("dial %d never happened", n)
	return nil
}

// --- wire fixtures ------------------------------------------------------------

func welcome(caps ...string) map[string]any {
	c := make([]any, len(caps))
	for i, s := range caps {
		c[i] = s
	}
	return map[string]any{"t": "welcome", "v": wire.ProtocolVersion, "caps": c}
}

func agentsRollup() map[string]any {
	return map[string]any{"t": "agents", "items": []any{
		map[string]any{"pane": 3, "pub": "w1:p3", "workspace": "w1", "tab": 1,
			"agent": "claude", "model": "claude-opus-5 · high", "state": "blocked", "seen": true, "since_ms": 120000},
		map[string]any{"pane": 5, "pub": "w1:p5", "workspace": "w1", "tab": 1,
			"agent": "codex", "state": "working", "seen": true, "since_ms": 3000},
		map[string]any{"pane": 7, "pub": "w2:p7", "workspace": "w2", "tab": 2,
			"agent": "claude", "state": "idle", "seen": false, "since_ms": 60000},
	}}
}

// frameFor is a 12×2 frame whose first row reads `text` (padded) in the
// defaults and whose second row is blank.
func frameFor(pane int, text string) map[string]any {
	const w = 12
	cells := make([]any, 0, w*2)
	padded := text + strings.Repeat(" ", w-len(text))
	for _, r := range padded {
		cells = append(cells, map[string]any{"s": string(r)})
	}
	for i := 0; i < w; i++ {
		cells = append(cells, map[string]any{"s": " "})
	}
	return map[string]any{"t": "pane_frame", "pane": pane, "w": w, "h": 2,
		"cur":    map[string]any{"x": 0, "y": 0, "vis": true, "shape": 0},
		"def_fg": 0x02e6e6e6, "def_bg": 0x02101010, "cells": cells}
}

func diffFor(pane int, index int, text string) map[string]any {
	cells := make([]any, 0, len(text))
	for i, r := range text {
		cells = append(cells, map[string]any{"i": index + i, "s": string(r)})
	}
	return map[string]any{"t": "pane_diff", "pane": pane, "cells": cells}
}

// --- harness ------------------------------------------------------------------

type harness struct {
	t       *testing.T
	ctx     *core.Context
	manager *render.Manager
	desk    *fakeDesk
	tree    map[string]any
}

var testEndpoint = catsclient.Endpoint{ID: "desk", Host: "desk.test", Port: 8443, TLS: true,
	PinnedSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}

// resetServicesForTest points the process-wide services at a fresh store in
// a temp data directory and a fake desk. The services are a singleton by
// design (one app per process); a test binary runs many, so each starts
// clean. Lives in a _test.go file so nothing but a test can reach it.
func resetServicesForTest(t *testing.T) (*fakeDesk, *store.Store) {
	t.Helper()
	mobile.SetDataDir(t.TempDir())
	store.Close()
	st := store.Open()
	desk := &fakeDesk{}

	servicesOnce = sync.Once{}
	services = newServices(st, desk.dial)
	servicesOnce.Do(func() {})

	t.Cleanup(func() {
		services.Conn.Disconnect()
		servicesOnce = sync.Once{}
		services = nil
		store.Close()
		mobile.SetDataDir("")
	})
	return desk, st
}

// newHarness mounts the app. paired pre-stores an endpoint and a token, so
// the boot sequence dials the fake desk.
func newHarness(t *testing.T, paired bool) *harness {
	t.Helper()
	desk, st := resetServicesForTest(t)
	if paired {
		if err := st.AddEndpoint(testEndpoint); err != nil {
			t.Fatal(err)
		}
		if err := st.WriteToken(testEndpoint.ID, "1700000000.abcd"); err != nil {
			t.Fatal(err)
		}
	}
	ctx := core.NewContext()
	manager := render.New(ctx, App)
	t.Cleanup(manager.Close)

	h := &harness{t: t, ctx: ctx, manager: manager, desk: desk}
	h.decode(manager.RenderInitial())
	return h
}

// connect is the fixture's connect burst: a welcome with the caps the
// screens gate on, then the roster.
func (h *harness) connect() *fakeSocket {
	h.t.Helper()
	s := h.desk.socket(h.t, 0)
	s.deliver(welcome(wire.CapViewer, wire.CapWindow, wire.CapClients, wire.CapKeyPane))
	s.deliver(agentsRollup())
	return s
}

// settle waits for the rendered output to stop changing. It polls the
// output rather than ctx.IsDirty, which the pump goroutine owns and clears
// on its own schedule.
func (h *harness) settle() {
	h.t.Helper()
	const stableRuns = 3
	previous, stable := "", 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.decode(h.manager.RenderAgain())
		current := h.html()
		if current == previous {
			if stable++; stable >= stableRuns {
				return
			}
		} else {
			stable, previous = 0, current
		}
		time.Sleep(15 * time.Millisecond)
	}
	h.t.Fatal("app never settled: the tree was still changing after 5s")
}

// waitFor settles repeatedly until the output shows want, so a test can wait
// on a message that has not landed yet without a fixed sleep.
func (h *harness) waitFor(want string) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.decode(h.manager.RenderAgain())
		if shows(h.html(), want) {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	h.t.Fatalf("never saw %q:\n%s", want, h.html())
}

func (h *harness) decode(payload string) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return // patches decode as an array; expected
	}
	if _, isNode := parsed["Type"]; isNode {
		h.tree = parsed
	}
}

func (h *harness) node() *core.Node {
	return core.Render(h.ctx, core.ComponentFunc(func(ctx *core.Context) *core.Node {
		return App(ctx).Render(ctx)
	}))
}

func (h *harness) html() string { return htmlout.ExportHTML(h.node()) }

// shows reports whether the rendered screen contains want as a reader would
// see it (htmlout entity-escapes text, so "·" and "&" are unescaped first).
func shows(rendered, want string) bool {
	return strings.Contains(gohtml.UnescapeString(rendered), want)
}

// tap dispatches the callback of the innermost node whose text or
// accessibility label contains label.
func (h *harness) tap(label string) {
	h.t.Helper()
	node := h.node()
	id := findCallbackFor(node, label, "onClick", "onTap")
	if id == "" {
		h.t.Fatalf("no tappable node labelled %q in:\n%s", label, htmlout.ExportHTML(node))
	}
	h.manager.DispatchCallback(id)
}

// typeInto sets the value of the input whose accessibility label contains
// label, the way a native text change event does.
func (h *harness) typeInto(label, value string) {
	h.t.Helper()
	node := h.node()
	id := findCallbackFor(node, label, "onChange", "onInput")
	if id == "" {
		h.t.Fatalf("no input labelled %q in:\n%s", label, htmlout.ExportHTML(node))
	}
	h.manager.DispatchTextCallback(id, value)
}

func findCallbackFor(node *core.Node, label string, keys ...string) string {
	if node == nil {
		return ""
	}
	for _, child := range node.Children {
		if id := findCallbackFor(child, label, keys...); id != "" {
			return id
		}
	}
	if !strings.Contains(textOf(node), label) {
		return ""
	}
	for _, key := range keys {
		if id, ok := node.Props[key].(string); ok && id != "" {
			return id
		}
	}
	return ""
}

func textOf(node *core.Node) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	for _, key := range []string{"content", "label", "placeholder"} {
		if s, ok := node.Props[key].(string); ok {
			b.WriteString(s)
			b.WriteString(" ")
		}
	}
	if node.Style != nil && node.Style.AccessibilityLabel != "" {
		b.WriteString(node.Style.AccessibilityLabel)
		b.WriteString(" ")
	}
	for _, child := range node.Children {
		b.WriteString(textOf(child))
	}
	return b.String()
}

// --- the tests ------------------------------------------------------------------

func TestUnpairedAppShowsThePairScreen(t *testing.T) {
	h := newHarness(t, false)
	h.settle()
	out := h.html()
	if !shows(out, "Pair with a desk") || !shows(out, "Pairing link") {
		t.Errorf("the pair screen did not render:\n%s", out)
	}
	if shows(out, "Agents, selected") {
		t.Errorf("the tab shell rendered without an endpoint:\n%s", out)
	}
	if h.desk.dials() != 0 {
		t.Errorf("an unpaired app dialled %d times", h.desk.dials())
	}
}

// The first message on the socket is the handshake, and it is a viewer's:
// no grid, Viewer set, the bearer from the store on the dial. This is the
// app-level half of the "phone never resizes the desktop" guard.
func TestBootDialsTheActiveEndpointAsAViewer(t *testing.T) {
	h := newHarness(t, true)
	s := h.desk.socket(t, 0)

	deadline := time.Now().Add(2 * time.Second)
	for len(s.sentNamed("init")) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	inits := s.sentNamed("init")
	if len(inits) != 1 {
		t.Fatalf("want one init, got %d", len(inits))
	}
	init := inits[0]
	if init["viewer"] != true {
		t.Errorf("init is not a viewer: %v", init)
	}
	for _, key := range []string{"cols", "rows"} {
		if v, ok := init[key]; ok && v != float64(0) {
			t.Errorf("init declared a grid: %s=%v", key, v)
		}
	}
	h.desk.mu.Lock()
	opts := h.desk.opts[0]
	h.desk.mu.Unlock()
	if opts.Token != "1700000000.abcd" {
		t.Errorf("the stored token did not reach the dial: %q", opts.Token)
	}
	if opts.StoredPin != testEndpoint.PinnedSHA256 {
		t.Errorf("the pairing pin did not reach the dial: %q", opts.StoredPin)
	}
}

func TestRosterGroupsAgentsByStateOnceTheRollupLands(t *testing.T) {
	h := newHarness(t, true)
	h.connect()
	h.waitFor("claude-opus-5")
	h.settle()

	out := gohtml.UnescapeString(h.html())
	for _, want := range []string{"NEEDS YOU", "WORKING", "IDLE", "claude-opus-5 · high", "codex", "w2:p7", "Done"} {
		if !strings.Contains(out, want) {
			t.Errorf("roster is missing %q:\n%s", want, out)
		}
	}
	// Grouped by attention: the blocked section precedes the working one,
	// which precedes idle.
	blocked, working, idle := strings.Index(out, "NEEDS YOU"), strings.Index(out, "WORKING"), strings.Index(out, "IDLE")
	if !(blocked < working && working < idle) {
		t.Errorf("roster groups out of order: blocked=%d working=%d idle=%d", blocked, working, idle)
	}
	// Connected: no banner.
	if strings.Contains(out, "Connecting to") || strings.Contains(out, "Reconnecting") {
		t.Errorf("the connection banner is still up while connected:\n%s", out)
	}
}

func TestOpeningAPaneShowsItsGridAndADiffUpdatesIt(t *testing.T) {
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(map[string]any{"t": "pane_title", "pane": 3, "title": "vim README.md"})
	s.deliver(map[string]any{"t": "pane_cwd", "pane": 3, "cwd": "~/projs/cats"})
	s.deliver(frameFor(3, "$ make check"))
	h.waitFor("claude-opus-5")

	h.tap("claude-opus-5")
	h.waitFor("$ make check")
	out := gohtml.UnescapeString(h.html())
	for _, want := range []string{"vim README.md", "~/projs/cats", "w1:p3", "Needs you", "Reply to the pane"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane screen is missing %q:\n%s", want, out)
		}
	}
	// The grid rendered as TextGrid rows, and the blank second row is an
	// empty row rather than a missing one.
	if !strings.Contains(out, "<pre") {
		t.Errorf("no TextGrid in the pane screen:\n%s", out)
	}

	// A diff overwrites "make" with "test": only that row changes.
	s.deliver(diffFor(3, 2, "test "))
	h.waitFor("$ test check")

	h.tap("Back")
	h.waitFor("NEEDS YOU")
}

func TestReplySendsPaneAddressedInput(t *testing.T) {
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(frameFor(3, "> "))
	h.waitFor("claude-opus-5")
	h.tap("claude-opus-5")
	h.waitFor("Reply to the pane")

	h.typeInto("Reply to the pane", "yes, go ahead")
	h.settle()
	h.tap("Send")

	// The command lands on the socket; answer it so the composer clears.
	deadline := time.Now().Add(2 * time.Second)
	var cmds []map[string]any
	for time.Now().Before(deadline) {
		if cmds = s.sentNamed("pane.send_input"); len(cmds) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(cmds) != 1 {
		t.Fatalf("want one pane.send_input, got %d", len(cmds))
	}
	params, _ := cmds[0]["params"].(map[string]any)
	if params["pane"] != float64(3) || params["text"] != "yes, go ahead" || params["submit"] != true {
		t.Errorf("send_input params = %v", params)
	}
	s.deliver(map[string]any{"t": "cmd_result", "id": cmds[0]["id"], "ok": true})
	h.settle()

	// Nothing else left the phone: no key, no paste, no focus, no resize.
	for _, forbidden := range []string{"key", "paste", "focus", "resize", "mouse"} {
		if n := len(s.sentNamed(forbidden)); n != 0 {
			t.Errorf("the phone sent %d %q message(s)", n, forbidden)
		}
	}
}

func TestSocketDropReconnectsWithAFreshHandshake(t *testing.T) {
	h := newHarness(t, true)
	s1 := h.connect()
	h.waitFor("claude-opus-5")

	s1.drop()
	h.waitFor("Reconnecting")

	// The ladder's first step is ~500 ms; the second dial arrives on its own.
	s2 := h.desk.socket(t, 1)
	s2.deliver(welcome(wire.CapViewer, wire.CapWindow))
	s2.deliver(map[string]any{"t": "agents", "items": []any{
		map[string]any{"pane": 9, "pub": "w1:p9", "workspace": "w1", "agent": "claude",
			"model": "claude-sonnet-5", "state": "idle", "seen": true, "since_ms": 0},
	}})
	h.waitFor("claude-sonnet-5")
	out := h.html()
	if shows(out, "Reconnecting") {
		t.Errorf("banner still up after reconnect:\n%s", out)
	}
	// The old roster is gone: the new socket's state replaced it.
	if shows(out, "claude-opus-5") {
		t.Errorf("the previous socket's roster survived the reconnect:\n%s", out)
	}
	if n := len(s2.sentNamed("init")); n != 1 {
		t.Errorf("the second socket got %d init messages", n)
	}
}

// A pin mismatch is a hard stop: one dial, a banner that names the problem,
// and no ladder. Retrying a wrong certificate is how people get trained to
// tap through warnings.
func TestCertificateMismatchIsAHardStop(t *testing.T) {
	desk, st := resetServicesForTest(t)
	_ = st.AddEndpoint(testEndpoint)
	_ = st.WriteToken(testEndpoint.ID, "tok")
	desk.fail = errors.Join(catsclient.ErrCertMismatch, errors.New("tls: handshake failure"))

	ctx := core.NewContext()
	manager := render.New(ctx, App)
	t.Cleanup(manager.Close)
	h := &harness{t: t, ctx: ctx, manager: manager, desk: desk}
	h.decode(manager.RenderInitial())

	h.waitFor("certificate changed")
	time.Sleep(700 * time.Millisecond) // past the first backoff step
	if desk.dials() != 1 {
		t.Errorf("a certificate mismatch was retried: %d dials", desk.dials())
	}
	h.tap("More")
	h.waitFor("Certificate changed")
	if !shows(h.html(), "Forget this device") {
		t.Errorf("the More tab offers no way out:\n%s", h.html())
	}
}

func TestWindowsTabFollowsAWindowThroughTheCapability(t *testing.T) {
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(map[string]any{"t": "clients", "total": 3, "sizers": 2, "cols": 200, "rows": 60, "views": []any{
		map[string]any{"workspace": "w1", "cols": 200, "rows": 60, "focused": true, "primary": true},
		map[string]any{"workspace": "w2", "cols": 120, "rows": 40},
		map[string]any{"workspace": "w1", "viewer": true},
	}})
	s.deliver(map[string]any{"t": "layout", "workspaces": []any{
		map[string]any{"id": "w1", "name": "cats", "active": true},
		map[string]any{"id": "w2", "name": "grmob"},
	}, "tabs": []any{}, "panes": []any{}, "borders": []any{}})
	h.waitFor("claude-opus-5")

	h.tap("Windows")
	h.waitFor("grmob")
	out := gohtml.UnescapeString(h.html())
	for _, want := range []string{"cats", "grmob", "Showing", "Primary", "3 connected", "1 viewing"} {
		if !strings.Contains(out, want) {
			t.Errorf("windows tab is missing %q:\n%s", want, out)
		}
	}

	h.tap("grmob")
	deadline := time.Now().Add(2 * time.Second)
	var cmds []map[string]any
	for time.Now().Before(deadline) {
		if cmds = s.sentNamed("workspace.focus"); len(cmds) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(cmds) != 1 {
		t.Fatalf("want one workspace.focus, got %d", len(cmds))
	}
	params, _ := cmds[0]["params"].(map[string]any)
	if params["id"] != "w2" {
		t.Errorf("workspace.focus params = %v", params)
	}
}

func TestAlertsShowNotificationsNewestFirstAndNoRecordUntilTold(t *testing.T) {
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(map[string]any{"t": "notify", "kind": "attention", "message": "Claude needs input", "pane": 3, "pub": "w1:p3"})
	s.deliver(map[string]any{"t": "notify", "kind": "finished", "message": "Build finished", "body": "exit 0"})
	h.waitFor("claude-opus-5")

	h.tap("Alerts")
	h.waitFor("Build finished")
	out := gohtml.UnescapeString(h.html())
	if strings.Index(out, "Build finished") > strings.Index(out, "Claude needs input") {
		t.Errorf("notifications are not newest first:\n%s", out)
	}
	if strings.Contains(out, "Recording") {
		t.Errorf("a record indicator was drawn before the server sent one:\n%s", out)
	}

	s.deliver(map[string]any{"t": "record", "recording": true, "steps": 4})
	h.waitFor("Recording 4 steps")
}

// Each keystroke arrives as the input's whole value and re-renders the
// form; the value must survive that round trip, and a keystroke in one
// field must not reset another. This is the copy-on-write update helper's
// contract, and the bug a stale closure over the form struct would cause.
func TestTypedInputPersistsAcrossKeystrokes(t *testing.T) {
	h := newHarness(t, false)
	h.settle()
	h.typeInto("192.168.1.20", "1")
	h.settle()
	if !strings.Contains(h.html(), `value="1"`) {
		t.Fatalf("after the first keystroke:\n%s", h.html())
	}
	h.typeInto("192.168.1.20", "12")
	h.settle()
	if !strings.Contains(h.html(), `value="12"`) {
		t.Fatalf("after the second keystroke:\n%s", h.html())
	}
	h.typeInto("shared password", "s")
	h.settle()
	out := h.html()
	if !strings.Contains(out, `value="12"`) || !strings.Contains(out, `value="s"`) {
		t.Fatalf("a keystroke in one field disturbed another:\n%s", out)
	}
}
