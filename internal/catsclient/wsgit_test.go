package catsclient

import (
	"testing"

	"github.com/rohanthewiz/cats/wire"
)

// The two surfaces the 2026-09-16 cats bump added: the workspace git-sync
// rollup (ws_git), and the plugin half of the agents rollup. Both are folded
// through the real decoder, so a fixture written as JSON here is exactly the
// bytes the server puts on the wire.

// --- workspace git rollup -------------------------------------------------------

func TestWorkspaceGitFoldsByWorkspaceAndReplacesWholesale(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, map[string]any{
		"t": "ws_git",
		"workspaces": []any{
			map[string]any{"ws": "w1", "sync": "ahead", "branch": "main", "remote": "origin", "ahead": 2},
			map[string]any{"ws": "w2", "sync": "synced", "branch": "master", "remote": "upstream"},
		},
	}))

	g, ok := s.GitSync("w1")
	if !ok || g.Sync != wire.GitAhead || g.Ahead != 2 || g.Branch != "main" || g.Remote != "origin" {
		t.Fatalf("w1 folded wrong: %+v (ok=%v)", g, ok)
	}
	// A workspace the sweep had nothing to say about is absent rather than
	// present with an empty state: the client draws the plain dot either way,
	// and conflating the two is how a "we have not asked yet" becomes a claim.
	if _, ok := s.GitSync("w3"); ok {
		t.Error("a workspace the sweep never mentioned must be absent, not present-and-empty")
	}

	// The next sweep drops w2 — its checkout moved, or the workspace closed. It
	// has to leave with the rollup rather than linger in the map it was written
	// into, which is the whole reason the arm rebuilds instead of patching.
	s.Apply(decode(t, map[string]any{
		"t":          "ws_git",
		"workspaces": []any{map[string]any{"ws": "w1", "sync": "synced"}},
	}))
	if _, ok := s.GitSync("w2"); ok {
		t.Error("w2 outlived the rollup that dropped it")
	}
	if g, _ := s.GitSync("w1"); g.Sync != wire.GitSynced || g.Ahead != 0 {
		t.Errorf("w1 not replaced wholesale: %+v", g)
	}
}

func TestWorkspaceGitDoesNotOutliveItsSocket(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, map[string]any{
		"t":          "ws_git",
		"workspaces": []any{map[string]any{"ws": "w1", "sync": "ahead", "ahead": 1}},
	}))

	s.ResetForNewSocket()

	// Unlike the agents and hosts rollups, which every connect pushes
	// unconditionally, the server sends this one only when a sweep has already
	// produced rows. So nothing is guaranteed to overwrite it, and a survivor
	// would colour the next socket's workspace dots with the last one's answers.
	if _, ok := s.GitSync("w1"); ok {
		t.Error("stale git state survived a reconnect: the next socket may be another server, or one that has not swept yet")
	}
}

// --- plugin panes ---------------------------------------------------------------

func TestAgentsRollupKeepsPluginsOutOfTheRoster(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, map[string]any{
		"t": "agents",
		"items": []any{
			map[string]any{"pub": "w1:p1", "workspace": "w1", "tab": 1, "agent": "claude", "state": "blocked", "seen": true},
		},
		"plugins": []any{
			map[string]any{
				"pane": 7, "pub": "w1:p7", "workspace": "w1", "tab": 1,
				"plugin": "rohanthewiz.cats-todo", "title": "todo: cats (3)",
			},
		},
	}))

	// A plugin pane has no agent state, so it must not reach the roster's state
	// grouping — nor the attention watch that reads the roster off the session.
	if got := pubs(s.Roster()); !equalStrings(got, []string{"w1:p1"}) {
		t.Errorf("a plugin pane reached the roster: %v", got)
	}
	if len(s.Plugins) != 1 {
		t.Fatalf("plugins not folded: %+v", s.Plugins)
	}
	// Title is the channel a plugin actually speaks on, so it is the one field
	// worth asserting beyond identity.
	if p := s.Plugins[0]; p.Plugin != "rohanthewiz.cats-todo" || p.Title != "todo: cats (3)" || p.Pane != 7 {
		t.Errorf("plugin pane folded wrong: %+v", p)
	}
}
