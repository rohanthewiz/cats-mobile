package catsclient

import (
	"encoding/json"
	"testing"

	"github.com/rohanthewiz/cats/wire"
)

// Ported from the Dart suite's session_test.dart (deleted in phase 6) and the fold half of
// views_test.dart.

func agent(pub, state string, opts ...func(*wire.AgentItem)) wire.AgentItem {
	a := wire.AgentItem{Pub: pub, Workspace: "w1", Tab: 1, Agent: "claude", State: state, Seen: true}
	for _, o := range opts {
		o(&a)
	}
	return a
}

func unseen(a *wire.AgentItem)             { a.Seen = false }
func since(ms int64) func(*wire.AgentItem) { return func(a *wire.AgentItem) { a.SinceMs = ms } }
func onPane(p uint32) func(*wire.AgentItem) {
	return func(a *wire.AgentItem) { a.Pane = p }
}

func pubs(items []wire.AgentItem) []string {
	out := make([]string, len(items))
	for i, a := range items {
		out[i] = a.Pub
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// decode is the real decoder, so a fixture written as JSON is exactly what
// the server would send.
func decode(t *testing.T, m map[string]any) any {
	t.Helper()
	raw, _ := json.Marshal(m)
	msg, err := wire.DecodeDown(raw)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// --- roster ordering -----------------------------------------------------------

func TestRosterWhatNeedsMeBeatsWhereItLives(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{
		agent("w3:p1", wire.AgentIdle),
		agent("w1:p2", wire.AgentWorking),
		agent("w2:p9", wire.AgentBlocked),
		agent("w1:p5", wire.AgentIdle, unseen),
	}})
	got := pubs(s.Roster())
	if !equalStrings(got, []string{"w2:p9", "w1:p2", "w1:p5", "w3:p1"}) {
		t.Errorf("roster = %v; want blocked → working → done-unseen → idle, not grouped by workspace", got)
	}
}

func TestRosterWithinAGroupTheLongestWaitComesFirst(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{
		agent("w1:p1", wire.AgentBlocked, since(5000)),
		agent("w1:p2", wire.AgentBlocked, since(900000)),
	}})
	if got := s.Roster()[0].Pub; got != "w1:p2" {
		t.Errorf("first = %s", got)
	}
}

func TestRosterABlockedAgentStaysFirstEvenWhenUnseen(t *testing.T) {
	// "Finished while you were away" is a nudge; "blocked" is a request. The
	// request wins.
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{
		agent("w1:p1", wire.AgentIdle, unseen),
		agent("w1:p2", wire.AgentBlocked, unseen),
	}})
	if got := s.Roster()[0].Pub; got != "w1:p2" {
		t.Errorf("first = %s", got)
	}
}

func TestRosterDoesNotReorderTheRollupItself(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{
		agent("w3:p1", wire.AgentIdle), agent("w2:p9", wire.AgentBlocked),
	}})
	_ = s.Roster()
	if s.Agents[0].Pub != "w3:p1" {
		t.Error("Roster must sort a copy; Agents is the server's order")
	}
}

// --- fold ------------------------------------------------------------------------

func TestFoldPaneAgentPatchesTheRollupWithoutWaitingForTheNextOne(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{agent("w1:p1", wire.AgentWorking, onPane(1))}})
	s.Apply(&wire.PaneAgent{Pane: 1, Agent: "claude", State: wire.AgentBlocked, Seen: true})
	if got := s.Roster()[0].State; got != wire.AgentBlocked {
		t.Errorf("roster state = %s", got)
	}
	if got := s.PaneAgents[1].State; got != wire.AgentBlocked {
		t.Errorf("paneAgents state = %s", got)
	}
}

func TestFoldThePatchCarriesTheModelAndClearsItWhenAbsent(t *testing.T) {
	// The rows name the model rather than the agent, so a patch that dropped
	// it would blank the label for the round trip until the next rollup: the
	// exact lag the patch exists to avoid.
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{agent("w1:p1", wire.AgentIdle, onPane(1))}})
	s.Apply(&wire.PaneAgent{Pane: 1, Agent: "copilot", State: wire.AgentWorking, Model: "gpt-5-mini · medium", Seen: true})
	if got := s.Roster()[0].Model; got != "gpt-5-mini · medium" {
		t.Errorf("model = %q", got)
	}
	// An agent whose model stops resolving reports it absent; the old one
	// must not survive as a stale label.
	s.Apply(&wire.PaneAgent{Pane: 1, Agent: "copilot", State: wire.AgentIdle, Seen: true})
	if got := s.Roster()[0].Model; got != "" {
		t.Errorf("model = %q, want cleared", got)
	}
}

func TestFoldChromeLandsInItsOwnMapsKeyedByPane(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.PaneTitle{Pane: 4, Title: "vim"})
	s.Apply(&wire.PaneCwd{Pane: 4, Cwd: "/Users/ro/projs/go/cats"})
	s.Apply(&wire.PaneBranch{Pane: 4, Branch: "main"})
	s.Apply(&wire.PaneModes{Pane: 4, Mouse: true, AltScreen: true})
	s.Apply(&wire.PaneExited{Pane: 4, Code: 130})
	if s.Titles[4] != "vim" || s.Cwds[4] != "/Users/ro/projs/go/cats" || s.Branches[4] != "main" {
		t.Errorf("chrome = %v %v %v", s.Titles, s.Cwds, s.Branches)
	}
	if !s.Modes[4].AltScreen {
		t.Error("alt_screen should be recorded")
	}
	if s.ExitCodes[4] != 130 {
		t.Errorf("exit = %d", s.ExitCodes[4])
	}
}

func TestFoldARespawnedPaneStopsBeingAnExitedOne(t *testing.T) {
	// The death is remembered, never re-derived: a client that only ever adds
	// to ExitCodes would keep drawing "exited (130)" over a live shell for the
	// rest of the connection, and a reconnect is the only thing that clears it.
	s := NewSession()
	s.Apply(&wire.PaneExited{Pane: 4, Code: 130})
	s.Apply(&wire.PaneRespawned{Pane: 4})
	if _, dead := s.ExitCodes[4]; dead {
		t.Error("the pane is alive again")
	}
}

func TestFoldTheRecorderAndTheRunsInFlightAreServerAuthoritative(t *testing.T) {
	s := NewSession()
	// Nil, not idle: a server too old to send record sends nothing, and an
	// unlit indicator would be a claim this client cannot make.
	if s.Record != nil || len(s.RunbookRuns) != 0 {
		t.Fatal("nothing has arrived yet")
	}
	s.Apply(&wire.Record{Recording: true, Steps: 3})
	s.Apply(&wire.RunbookRuns{Runs: []wire.RunbookRun{{Name: "deploy", Source: "control", Step: 2, Steps: 5}}})
	if !s.Record.Recording || s.Record.Steps != 3 {
		t.Errorf("record = %+v", s.Record)
	}
	if len(s.RunbookRuns) != 1 || s.RunbookRuns[0].Name != "deploy" || s.RunbookRuns[0].Step != 2 {
		t.Errorf("runs = %+v", s.RunbookRuns)
	}
	// Replaced wholesale: a finished run is absent from the next push rather
	// than marked done in it, so folding must not merge with what came before.
	s.Apply(&wire.RunbookRuns{Runs: []wire.RunbookRun{}})
	if len(s.RunbookRuns) != 0 {
		t.Errorf("runs = %+v, want none", s.RunbookRuns)
	}
}

func TestFoldNotificationsAccumulateForTheInbox(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Notify{Kind: "attention", Message: "claude is blocked", Pub: "w1:p3"})
	s.Apply(&wire.Notify{Kind: "finished", Message: "run complete", Pub: "w2:p1"})
	if len(s.Notifications) != 2 || s.Notifications[1].Pub != "w2:p1" {
		t.Errorf("notifications = %+v", s.Notifications)
	}
}

func TestFoldClientsStaysNilUntilTheServerPushesACensus(t *testing.T) {
	// Nil means "unknown", not "nobody": a server without CapClients never
	// sends one, and rendering that as "you are alone" would be a lie.
	s := NewSession()
	if s.Clients != nil {
		t.Fatal("nothing has arrived")
	}
	s.Apply(&wire.Clients{Total: 2, Sizers: 1, Cols: 200, Rows: 60})
	if s.Clients.Sizers != 1 {
		t.Errorf("clients = %+v", s.Clients)
	}
	if s.ViewerCount() != 1 {
		t.Errorf("viewers = %d, want total - sizers", s.ViewerCount())
	}
}

func TestFoldAppLevelMessages(t *testing.T) {
	s := NewSession()
	s.Apply(&wire.Title{Title: "cats"})
	s.Apply(&wire.Error{Msg: "boom", Pane: 2})
	s.Apply(&wire.Shutdown{})
	s.Apply(&wire.UpdateReady{Version: "1.2.3", Command: "cats update"})
	s.Apply(&wire.Theme{Name: "dark"})
	s.Apply(&wire.Usage{})
	s.Apply(&wire.Hosts{Items: []wire.HostItem{{}}})
	if s.AppTitle != "cats" || len(s.Errors) != 1 || !s.ServerShutDown {
		t.Errorf("session = %+v", s)
	}
	if s.UpdateReady == nil || s.UpdateReady.Version != "1.2.3" || s.Theme == nil || s.Usage == nil {
		t.Error("update_ready, theme and usage should be recorded")
	}
	if len(s.Hosts) != 1 {
		t.Error("hosts should be recorded")
	}
}

func TestFoldIgnoresWhatItDeliberatelyIgnores(t *testing.T) {
	// Not a crash, not a state change.
	s := NewSession()
	s.Apply(&wire.Welcome{})
	s.Apply(&wire.CmdResult{})
	s.Apply(&wire.Clipboard{})
	s.Apply(&wire.History{})
	s.Apply(&wire.ChatDelta{})
	s.Apply("not even a message")
}

// --- resetForNewSocket ------------------------------------------------------------

func TestResetForNewSocketDiscardsEveryGrid(t *testing.T) {
	// Not tidiness: diff indices are relative to the last full frame's width
	// and def_fg/def_bg are per connection, so a carried-over grid is
	// corruption. registerConn resyncs every visible pane, so nothing is lost.
	s := NewSession()
	s.Apply(&wire.PaneFrame{Pane: 1, W: 2, H: 1, DefFg: 1, DefBg: 2, Cells: []wire.Cell{{S: "a"}, {S: "b"}}})
	s.Apply(&wire.Layout{})
	s.Apply(&wire.Clients{Total: 1})
	if len(s.Grids) == 0 || s.Layout == nil {
		t.Fatal("fixture did not land")
	}
	s.ResetForNewSocket()
	if len(s.Grids) != 0 || s.Layout != nil || s.Clients != nil {
		t.Error("grids, layout and census should be gone")
	}
}

func TestResetForNewSocketKeepsTheRoster(t *testing.T) {
	// The rollup spans every workspace and survives a reconnect until the
	// server sends a fresh one.
	s := NewSession()
	s.Apply(&wire.Agents{Items: []wire.AgentItem{agent("w1:p1", wire.AgentBlocked)}})
	s.ResetForNewSocket()
	if len(s.Agents) != 1 {
		t.Error("the roster is not connection-scoped")
	}
}

// --- the windows a phone can look through (views_test.dart) -------------------------

// layoutShowing is a layout as the server builds it FOR THIS CONNECTION: the
// active flag is its own view's workspace, not the session's.
func layoutShowing(activeWs string) map[string]any {
	return map[string]any{
		"t": "layout",
		"workspaces": []any{
			map[string]any{"id": "w1", "name": "cats", "active": activeWs == "w1"},
			map[string]any{"id": "w2", "name": "gonotes", "active": activeWs == "w2"},
		},
		"tabs": []any{}, "panes": []any{}, "borders": []any{},
	}
}

// censusOf is a census with the given views.
func censusOf(views ...map[string]any) map[string]any {
	sizers := 0
	vs := make([]any, len(views))
	for i, v := range views {
		vs[i] = v
		if v["viewer"] != true {
			sizers++
		}
	}
	return map[string]any{"t": "clients", "total": len(views), "sizers": sizers, "cols": 200, "rows": 60, "views": vs}
}

var (
	windowOnW1 = map[string]any{"workspace": "w1", "cols": 200, "rows": 60, "focused": true, "primary": true}
	windowOnW2 = map[string]any{"workspace": "w2", "cols": 120, "rows": 40}
	thisPhone  = map[string]any{"workspace": "w1", "viewer": true}
)

func TestWindowsFoldsTheCensusViewersLeftOut(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, layoutShowing("w1")))
	s.Apply(decode(t, censusOf(windowOnW1, windowOnW2, thisPhone)))

	windows := s.DesktopWindows()
	if len(windows) != 2 {
		t.Fatalf("%d windows; the phone is not a window", len(windows))
	}
	if windows[0].WorkspaceID != "w1" || windows[1].WorkspaceID != "w2" {
		t.Errorf("windows = %+v", windows)
	}
	// Joined to the layout's names: the census carries ids only.
	if windows[0].Label() != "cats" || windows[1].Label() != "gonotes" {
		t.Errorf("labels = %s, %s", windows[0].Label(), windows[1].Label())
	}
	if windows[0].Cols != 200 {
		t.Errorf("cols = %d", windows[0].Cols)
	}
	if s.ViewerCount() != 1 {
		t.Errorf("viewers = %d", s.ViewerCount())
	}
}

func TestWindowsViewWorkspaceIsTheServersAnswerNotALocalGuess(t *testing.T) {
	s := NewSession()
	if s.ViewWorkspace() != "" {
		t.Error("nothing has arrived yet")
	}
	s.Apply(decode(t, layoutShowing("w2")))
	if s.ViewWorkspace() != "w2" {
		t.Errorf("view = %q", s.ViewWorkspace())
	}
}

func TestWindowsFollowedMarksTheWindowWhoseWorkspaceThisViewShows(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, layoutShowing("w2")))
	s.Apply(decode(t, censusOf(windowOnW1, windowOnW2)))

	f := s.FollowedWindow()
	if f == nil || f.WorkspaceID != "w2" || f.Label() != "gonotes" {
		t.Errorf("followed = %+v", f)
	}
	// The primary is a different window: this phone pinned the other one.
	p := s.PrimaryWindow()
	if p == nil || p.WorkspaceID != "w1" || !p.Focused {
		t.Errorf("primary = %+v", p)
	}
}

func TestWindowsAnUnpinnedViewerFollowsThePrimarySoBothAgree(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, layoutShowing("w1")))
	s.Apply(decode(t, censusOf(windowOnW1, windowOnW2)))
	if s.FollowedWindow().WorkspaceID != s.PrimaryWindow().WorkspaceID {
		t.Error("followed and primary should be the same window")
	}
}

func TestWindowsACensusBeforeTheFirstLayoutStillListsWindows(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, censusOf(windowOnW1)))
	// No names and nothing followed yet: a UI shows ids rather than treating
	// the ordering of two server messages as an error.
	w := s.DesktopWindows()
	if len(w) != 1 || w[0].WorkspaceName != "" || w[0].Label() != "w1" {
		t.Errorf("windows = %+v", w)
	}
	if s.FollowedWindow() != nil {
		t.Error("nothing is followed before a layout")
	}
}

func TestWindowsResetForNewSocketDropsThemWithTheRestOfTheFold(t *testing.T) {
	s := NewSession()
	s.Apply(decode(t, layoutShowing("w1")))
	s.Apply(decode(t, censusOf(windowOnW1)))
	s.ResetForNewSocket()
	if len(s.DesktopWindows()) != 0 || s.ViewWorkspace() != "" {
		t.Error("windows and view should be gone")
	}
}
