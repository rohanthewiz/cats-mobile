package catsapp

import (
	gohtml "html"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
)

// Tests for attention.go and the composer's Paste: the three places this app
// talks to the phone rather than to the desk. They record grmob's system
// events the way a native shell would receive them, and answer a clipboard
// read the way a shell does, through the host-event channel.

// systemEvents records every system event the app sends. clipboardText and
// clipboardOK are what a clipboard read is answered with.
type systemEvents struct {
	mu            sync.Mutex
	events        []recordedEvent
	clipboardText string
	clipboardOK   bool
}

type recordedEvent struct {
	name string
	data map[string]any
}

// recordSystemEvents installs the recorder as the process's system-event
// handler for the test. A clipboard read is answered synchronously, inside
// the send, which is the hardest case for the app: the reply lands while the
// tap handler that asked is still running.
func recordSystemEvents(t *testing.T) *systemEvents {
	t.Helper()
	r := &systemEvents{clipboardOK: true}
	core.SetSystemEventHandler(func(name string, data map[string]any) {
		r.mu.Lock()
		r.events = append(r.events, recordedEvent{name, data})
		text, ok := r.clipboardText, r.clipboardOK
		r.mu.Unlock()
		if name == "clipboard" && data["command"] == "read" {
			core.ReceiveHostEvent("clipboard", map[string]any{"id": data["id"], "text": text, "ok": ok})
		}
	})
	t.Cleanup(func() { core.SetSystemEventHandler(nil) })
	return r
}

// named returns the payloads of the recorded events called name for which
// match (when given) is true.
func (r *systemEvents) named(name string, match func(map[string]any) bool) []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for _, e := range r.events {
		if e.name == name && (match == nil || match(e.data)) {
			out = append(out, e.data)
		}
	}
	return out
}

// waitNamed polls until at least n matching events have been recorded. The
// announcement is sent from the socket's reader goroutine, so a test that
// delivers a rollup cannot read the recorder straight after.
func (r *systemEvents) waitNamed(t *testing.T, n int, name string, match func(map[string]any) bool) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.named(name, match); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited for %d %q event(s); recorded %v", n, name, r.named(name, nil))
	return nil
}

// withCommand and withID are match functions for waitNamed.
func withCommand(command string) func(map[string]any) bool {
	return func(d map[string]any) bool { return d["command"] == command }
}

func withID(command, id string) func(map[string]any) bool {
	return func(d map[string]any) bool { return d["command"] == command && d["id"] == id }
}

// rollupWith is agentsRollup with some agents' states replaced, keyed by
// handle. The fixture's own states are w1:p3 blocked, w1:p5 working and
// w2:p7 idle.
func rollupWith(states map[string]string) map[string]any {
	msg := agentsRollup()
	for _, raw := range msg["items"].([]any) {
		item := raw.(map[string]any)
		if state, ok := states[item["pub"].(string)]; ok {
			item["state"] = state
		}
	}
	return msg
}

// The watch is silent on the rollup that seeds it, then reports exactly the
// transitions, sorted, and forgets everything on reset.
func TestBlockedWatchSeedsThenReportsOnlyChanges(t *testing.T) {
	item := func(pub, state string) wire.AgentItem { return wire.AgentItem{Pub: pub, State: state} }
	var w blockedWatch

	became, cleared := w.observe([]wire.AgentItem{item("w1:p3", wire.AgentBlocked), item("w1:p5", wire.AgentWorking)})
	if len(became) != 0 || len(cleared) != 0 {
		t.Fatalf("the seeding rollup reported became=%v cleared=%v", became, cleared)
	}

	became, cleared = w.observe([]wire.AgentItem{
		item("w1:p3", wire.AgentBlocked), // still blocked: not news
		item("w2:p9", wire.AgentBlocked),
		item("w1:p5", wire.AgentBlocked),
	})
	if len(became) != 2 || became[0].Pub != "w1:p5" || became[1].Pub != "w2:p9" || len(cleared) != 0 {
		t.Errorf("became=%v cleared=%v, want [w1:p5 w2:p9] and none", became, cleared)
	}

	// w1:p3 finished and w2:p9 left the rollup entirely: both are cleared.
	became, cleared = w.observe([]wire.AgentItem{item("w1:p3", wire.AgentIdle), item("w1:p5", wire.AgentBlocked)})
	if len(became) != 0 || len(cleared) != 2 || cleared[0] != "w1:p3" || cleared[1] != "w2:p9" {
		t.Errorf("became=%v cleared=%v, want none and [w1:p3 w2:p9]", became, cleared)
	}

	w.reset()
	if became, _ := w.observe([]wire.AgentItem{item("w3:p1", wire.AgentBlocked)}); len(became) != 0 {
		t.Errorf("the first rollup after a reset announced %v", became)
	}
}

// An agent that blocks buzzes the phone; it posts a notification only while
// the app is not in the foreground, and the notification comes down when the
// agent unblocks. The agent that was already blocked when the app connected
// is never announced.
func TestABlockedAgentBuzzesAndNotifiesOnlyInTheBackground(t *testing.T) {
	h := newHarness(t, true)
	rec := recordSystemEvents(t)
	s := h.connect()
	h.waitFor("claude-opus-5")
	h.settle()
	if n := len(rec.named("haptic", nil)); n != 0 {
		t.Fatalf("the connect rollup buzzed %d time(s) for an agent that was already blocked", n)
	}

	// In the foreground: a buzz, no banner.
	s.deliver(rollupWith(map[string]string{"w1:p5": "blocked"}))
	buzz := rec.waitNamed(t, 1, "haptic", nil)
	if buzz[0]["kind"] != string(core.HapticWarning) {
		t.Errorf("buzz kind = %v, want warning", buzz[0]["kind"])
	}
	h.settle()
	if posts := rec.named("notification", withCommand("post")); len(posts) != 0 {
		t.Errorf("posted %v while the app was in the foreground", posts)
	}

	// In the background: a buzz and a banner, addressed by the pane's handle.
	t.Cleanup(func() { core.ReceiveLifecycle(core.LifecycleActive) })
	core.ReceiveLifecycle(core.LifecycleBackground)
	s.deliver(rollupWith(map[string]string{"w1:p5": "blocked", "w2:p7": "blocked"}))
	posts := rec.waitNamed(t, 1, "notification", withID("post", "w2:p7"))
	if title, _ := posts[0]["title"].(string); title != "claude needs you" {
		t.Errorf("notification title = %q", title)
	}
	if body, _ := posts[0]["body"].(string); !strings.Contains(body, "w2:p7") {
		t.Errorf("notification body = %q, want the pane handle in it", body)
	}
	if n := len(rec.named("haptic", nil)); n != 2 {
		t.Errorf("buzzed %d times after two blocking rollups", n)
	}

	// w2:p7 goes back to idle: its banner is taken down.
	s.deliver(rollupWith(map[string]string{"w1:p5": "blocked"}))
	rec.waitNamed(t, 1, "notification", withID("cancel", "w2:p7"))
}

// Tapping a notification opens the pane it names, with the roster underneath
// it; a handle the session no longer knows opens nothing.
func TestTappingANotificationOpensThePane(t *testing.T) {
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(frameFor(3, "$ make check"))
	h.waitFor("claude-opus-5")

	tap := func(id string) {
		// A native shell delivers a host event inside the render manager's
		// dispatch, which is what this reproduces.
		h.manager.Dispatch("notification tap", func() {
			core.ReceiveHostEvent("notification_tap", map[string]any{"id": id})
		})
	}

	tap("w9:p9")
	h.settle()
	if shows(h.html(), "Reply to the pane") {
		t.Fatalf("a tap on an unknown handle opened a pane:\n%s", h.html())
	}

	tap("w1:p3")
	h.waitFor("$ make check")
	h.waitFor("Reply to the pane")

	h.tap("Back")
	h.waitFor("NEEDS YOU")
}

// Paste appends the clipboard to the draft; a refused read leaves the draft
// alone and says why.
func TestPasteAppendsTheClipboardToTheDraft(t *testing.T) {
	h := newHarness(t, true)
	rec := recordSystemEvents(t)
	s := h.connect()
	s.deliver(frameFor(3, "> "))
	h.waitFor("claude-opus-5")
	h.tap("claude-opus-5")
	h.waitFor("Reply to the pane")

	h.typeInto("Reply to the pane", "see ")
	h.settle()
	rec.mu.Lock()
	rec.clipboardText = "/tmp/build.log"
	rec.mu.Unlock()
	h.tap("Paste from the clipboard")
	h.waitFor("see /tmp/build.log")

	rec.mu.Lock()
	rec.clipboardOK = false
	rec.mu.Unlock()
	h.tap("Paste from the clipboard")
	rec.waitNamed(t, 1, "toast", func(d map[string]any) bool {
		msg, _ := d["message"].(string)
		return strings.Contains(msg, "Could not read the clipboard")
	})
	h.settle()
	out := gohtml.UnescapeString(h.html())
	if strings.Count(out, "/tmp/build.log") != 1 {
		t.Errorf("a refused read changed the draft:\n%s", out)
	}
}
