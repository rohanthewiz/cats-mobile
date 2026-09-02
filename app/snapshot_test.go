package catsapp

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
)

// Screen snapshots: each screen's rendered markup pinned byte for byte with
// grmob's htmlout exporter, against a fixed transcript fed through the fake
// desk.
//
// app_test.go asserts that a screen shows the right content. These assert
// that it shows it in the same shape: the same nesting, the same styles, the
// same accessibility labels. A diff here is not automatically a bug; it is
// a prompt to look, and if the change is intended, to re-record with
//
//	go test ./app -update

var update = flag.Bool("update", false,
	"rewrite the testdata golden files from the current render")

// snapshot mounts the app paired, feeds the fixture transcript, runs the
// given taps to reach the screen, and compares the export to
// testdata/<name>.html.
func snapshot(t *testing.T, name string, reach func(h *harness)) {
	t.Helper()
	h := newHarness(t, true)
	s := h.connect()
	s.deliver(map[string]any{"t": "title", "title": "cats · desk"})
	s.deliver(map[string]any{"t": "pane_title", "pane": 3, "title": "vim README.md"})
	s.deliver(map[string]any{"t": "pane_cwd", "pane": 3, "cwd": "~/projs/cats"})
	s.deliver(map[string]any{"t": "pane_branch", "pane": 3, "branch": "main"})
	s.deliver(map[string]any{"t": "pane_agent", "pane": 3, "agent": "claude", "state": "blocked",
		"model": "claude-opus-5 · high", "seen": true})
	s.deliver(frameFor(3, "$ make check"))
	s.deliver(map[string]any{"t": "clients", "total": 2, "sizers": 1, "cols": 200, "rows": 60, "views": []any{
		map[string]any{"workspace": "w1", "cols": 200, "rows": 60, "focused": true, "primary": true},
		map[string]any{"workspace": "w1", "viewer": true},
	}})
	s.deliver(map[string]any{"t": "layout", "workspaces": []any{
		map[string]any{"id": "w1", "name": "cats", "active": true},
	}, "tabs": []any{}, "panes": []any{}, "borders": []any{}})
	s.deliver(map[string]any{"t": "notify", "kind": "attention", "message": "Claude needs input",
		"pane": 3, "pub": "w1:p3", "id": "n1", "actions": []any{
			map[string]any{"id": "yes", "label": "Yes", "send": "y", "submit": true},
			map[string]any{"id": "no", "label": "No", "send": "n", "submit": true},
		}})
	s.deliver(map[string]any{"t": "record", "recording": true, "steps": 2})
	h.waitFor("claude-opus-5")
	h.settle()
	if reach != nil {
		reach(h)
		h.settle()
	}

	got := stabilize(h.html())
	golden := filepath.Join("testdata", name+".html")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s", golden)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v\n\nRecord it with: go test ./app -update", err)
	}
	if got != string(want) {
		t.Errorf("%s does not match the golden file.\n\n"+
			"If the change is intended, re-record with: go test ./app -update\n\n"+
			"--- got ---\n%s", name, got)
	}
}

// stabilize removes what legitimately differs between runs: the agent ages
// tick with the clock, and the build line names a local checkout path.
func stabilize(rendered string) string {
	return ageToken.ReplaceAllString(rendered, "${1}AGE")
}

// ageToken matches an age label ("2m", "12s", "1h 03m", "3d") where the
// roster and alerts render one: after a bullet, or after "started ", and
// before the closing tag or " ago". htmlout escapes the bullet's middle dot,
// so both spellings are accepted.
var ageToken = regexp.MustCompile(`((?:·|&middot;|&#183;|&#xb7;) |started )\d+(?:s|m|d|h \d\dm)`)

func TestSnapshotAgents(t *testing.T) { snapshot(t, "agents", nil) }
func TestSnapshotWindows(t *testing.T) {
	snapshot(t, "windows", func(h *harness) { h.tap("Windows") })
}
func TestSnapshotAlerts(t *testing.T) {
	snapshot(t, "alerts", func(h *harness) { h.tap("Alerts") })
}
func TestSnapshotMore(t *testing.T) {
	snapshot(t, "more", func(h *harness) { h.tap("More") })
}
func TestSnapshotPane(t *testing.T) {
	snapshot(t, "pane", func(h *harness) { h.tap("claude-opus-5"); h.waitFor("$ make check") })
}
func TestSnapshotPair(t *testing.T) {
	// The pair screen is reached through More rather than by starting
	// unpaired, so one fixture serves every snapshot.
	snapshot(t, "pair", func(h *harness) { h.tap("More"); h.tap("Pair another desk") })
}

// Every golden file must be reachable from a test, or a screen can be
// deleted and its snapshot silently kept forever.
func TestEveryGoldenFileHasATest(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Skip("no testdata yet; record with: go test ./app -update")
	}
	source, err := os.ReadFile("snapshot_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".html")
		if !strings.Contains(string(source), `"`+name+`"`) {
			t.Errorf("testdata/%s has no snapshot test; delete it or add one", entry.Name())
		}
	}
}

// Guard against the theme being anything but the app's own: the wire theme
// applied in App must reach the tree, and this pins the mapping once.
func TestServerThemeReachesTheTree(t *testing.T) {
	theme := themeFor(&wire.Theme{Colors: map[string]string{
		"bg": "#1A1B26", "fg": "#C0CAF5", "accent": "#7AA2F7", "err": "#F7768E", "bogus": "not-a-colour",
	}})
	if theme.Colors.Background != "#1a1b26" || theme.Colors.TextPrimary != "#c0caf5" {
		t.Errorf("server palette not applied: %+v", theme.Colors)
	}
	if theme.Colors.Primary != "#7aa2f7" || theme.Components.Button.Background != "#7aa2f7" {
		t.Errorf("accent did not reach both the palette and the button base: %+v", theme.Colors)
	}
	// Keys the server did not send keep the fallback.
	if theme.Colors.Warning != darkWarn {
		t.Errorf("unsent key lost its fallback: %q", theme.Colors.Warning)
	}
	_ = core.DefaultTheme
}
