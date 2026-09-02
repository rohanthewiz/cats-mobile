package catsclient

import (
	"cmp"
	"slices"

	"github.com/rohanthewiz/cats/wire"
)

// Session is the client's fold of the down-message stream into state.
//
// Every screen reads from here; nothing here knows about grmob. Keeping the
// fold in the protocol package is what lets a recorded transcript drive the
// same state a live socket does, which is what makes screen tests and demo
// mode honest rather than approximate.
//
// # Roster ordering is a product decision, not a data one
//
// The agents rollup is the one message that spans EVERY workspace (frames
// stream only for visible panes, but agent chrome is global), so it, not the
// layout, is the backbone of the home screen. It is grouped by state rather
// than by workspace because on a phone "what needs me" beats "where it lives".
//
// # Locking
//
// Session has no mutex. Apply runs on the connection's reader goroutine and
// every read runs on a render pass, so the app holds the Session behind a
// pointer with its own lock and takes it around both. Every method here is a
// plain read or write under that lock.
type Session struct {
	// Agents is every pane with an agent, across every workspace.
	Agents []wire.AgentItem

	// Layout is the active workspace's structure, as built for THIS
	// connection's view. A viewer renders it; it never causes it, since
	// nothing in this type sends anything. Nil until the first layout.
	Layout *wire.Layout

	// Grids is per-pane, created lazily. Only panes the server actually
	// streams frames for appear here.
	Grids map[uint32]*PaneGrid

	// Per-pane chrome, keyed by pane id.
	Titles     map[uint32]string
	Cwds       map[uint32]string
	Branches   map[uint32]string
	PaneAgents map[uint32]wire.PaneAgent
	Modes      map[uint32]wire.PaneModes
	ExitCodes  map[uint32]int

	// Clients is the connected-client census. Nil until the server pushes
	// one, which a server without wire.CapClients never does, so nil means
	// "unknown", not "nobody".
	Clients *wire.Clients

	// Hosts is the cathost roster; nil until pushed.
	Hosts []wire.HostItem

	Usage    *wire.Usage
	Theme    *wire.Theme
	AppTitle string

	// Notifications is the history, newest last. Backs the Alerts screen.
	Notifications []wire.Notify

	// Errors is non-fatal server errors, for a toast and a debug view.
	Errors []wire.Error

	ServerShutDown bool
	UpdateReady    *wire.UpdateReady

	// Record is the macro recorder's state, server-authoritative. Nil until
	// the server pushes one, which a server too old to send record never does,
	// so nil means "unknown", not "idle", and the phone should draw no
	// indicator rather than an unlit one it cannot vouch for.
	Record *wire.Record

	// RunbookRuns is the runs in flight, whatever started them. Replaced
	// wholesale on every push: the message carries the entire set, so a run
	// that has finished is absent from the next one rather than marked done.
	RunbookRuns []wire.RunbookRun
}

func NewSession() *Session {
	return &Session{
		Grids:      map[uint32]*PaneGrid{},
		Titles:     map[uint32]string{},
		Cwds:       map[uint32]string{},
		Branches:   map[uint32]string{},
		PaneAgents: map[uint32]wire.PaneAgent{},
		Modes:      map[uint32]wire.PaneModes{},
		ExitCodes:  map[uint32]int{},
	}
}

// Apply folds one decoded down-message, as wire.DecodeDown returns it: a
// pointer to the concrete struct. Unknown types never reach here; the decoder
// reports them and Conn drops them, per the protocol's own rule.
//
// Every wire.Type has an arm here or is on ignoredDownTypes, and
// TestEveryDownTypeHasAnArm enforces that after each cats bump: the compiler
// adds the types, it has no opinion about what the session does with one.
func (s *Session) Apply(msg any) {
	switch m := msg.(type) {
	case *wire.Agents:
		s.Agents = m.Items
	case *wire.Layout:
		s.Layout = m
	case *wire.PaneTitle:
		s.Titles[m.Pane] = m.Title
	case *wire.PaneCwd:
		s.Cwds[m.Pane] = m.Cwd
	case *wire.PaneBranch:
		s.Branches[m.Pane] = m.Branch
	case *wire.PaneAgent:
		s.PaneAgents[m.Pane] = *m
		s.patchRoster(m)
	case *wire.PaneModes:
		s.Modes[m.Pane] = *m
	case *wire.PaneExited:
		s.ExitCodes[m.Pane] = m.Code
	case *wire.PaneRespawned:
		// A pane's death is remembered here, not re-derived from the layout,
		// so nothing else would ever take the "exited (N)" back off. Dropping
		// the key rather than storing a sentinel keeps "is this pane dead" a
		// plain map lookup, the same question it was before the pane came
		// back.
		delete(s.ExitCodes, m.Pane)
	case *wire.PaneFrame:
		s.GridFor(m.Pane).ApplyFrame(m)
	case *wire.PaneDiff:
		s.GridFor(m.Pane).ApplyDiff(m)
	case *wire.Clients:
		s.Clients = m
	case *wire.Hosts:
		s.Hosts = m.Items
	case *wire.Usage:
		s.Usage = m
	case *wire.Theme:
		s.Theme = m
	case *wire.Title:
		s.AppTitle = m.Title
	case *wire.Notify:
		s.Notifications = append(s.Notifications, *m)
	case *wire.Error:
		s.Errors = append(s.Errors, *m)
	case *wire.Shutdown:
		s.ServerShutDown = true
	case *wire.UpdateReady:
		s.UpdateReady = m
	case *wire.Record:
		s.Record = m
	case *wire.RunbookRuns:
		s.RunbookRuns = m.Runs
		// Welcome and CmdResult are the connection's business, not the
		// session's. Clipboard, History and the chat surface are ignored on
		// purpose; see ignoredDownTypes.
	}
}

// ignoredDownTypes are the down-messages Apply deliberately has no arm for,
// each with the reason. The coverage test reads this list, so adding a type
// here is an explicit decision rather than a forgotten case.
var ignoredDownTypes = map[wire.Type]string{
	wire.MsgWelcome:      "settled by Conn: the handshake, the caps and the version check",
	wire.MsgCmdResult:    "settled by Conn: it answers the pending Invoke",
	wire.MsgClipboard:    "an OSC 52 write from a pane; a viewer does not own the desktop's clipboard",
	wire.MsgHistory:      "the command ledger, a desktop-only section for now",
	wire.MsgChatState:    "the ACP chat surface is out of scope for the phone's first release",
	wire.MsgChatSnapshot: "the ACP chat surface is out of scope for the phone's first release",
	wire.MsgChatRow:      "the ACP chat surface is out of scope for the phone's first release",
	wire.MsgChatDelta:    "the ACP chat surface is out of scope for the phone's first release",
	wire.MsgChatPerm:     "the ACP chat surface is out of scope for the phone's first release",
}

// patchRoster patches the rollup in place from a pane_agent. The server
// re-sends the whole rollup on any change too, but this arrives first and the
// roster should not lag a round trip behind the pane it is describing.
func (s *Session) patchRoster(m *wire.PaneAgent) {
	for i, a := range s.Agents {
		if a.Pane != m.Pane {
			continue
		}
		a.Agent = m.Agent
		a.State = m.State
		// Taken from the message, not carried over from the old item: an
		// agent whose model stopped resolving reports it absent, and a stale
		// model must not outlive the report it came from.
		a.Model = m.Model
		a.Seen = m.Seen
		a.SinceMs = 0
		s.Agents[i] = a
	}
}

// GridFor is the pane's grid, created on first use.
func (s *Session) GridFor(pane uint32) *PaneGrid {
	g, ok := s.Grids[pane]
	if !ok {
		g = NewPaneGrid(pane)
		s.Grids[pane] = g
	}
	return g
}

// ResetForNewSocket discards every grid. Call on a NEW socket, before the
// first message.
//
// This is not tidiness. Diff indices are relative to the last full frame's
// width, and def_fg/def_bg are per connection, so carrying a grid across a
// reconnect is corruption: cells patched at indices computed for another
// geometry, in colours resolved against another frame's defaults. catway's
// registerConn guarantees a full resync per visible pane on connect, so there
// is nothing to lose by dropping them.
func (s *Session) ResetForNewSocket() {
	clear(s.Grids)
	s.Layout = nil
	s.Clients = nil
}

// --- windows -----------------------------------------------------------------
//
// A connection is a VIEW, not a mirror: each desktop window shows one
// workspace, and a viewer follows the primary view (whichever desktop window
// the user touched last). Two questions follow from that, and the phone is the
// client that most needs both answered on screen:
//
//	"whose window am I looking through?"  -> ViewWorkspace, FollowedWindow
//	"what else could I look through?"     -> DesktopWindows
//
// Both are folds of what the server already sends. The clients census carries
// one entry per connection with the workspace it RESOLVED to, and the layout
// this connection receives is built for its own view, so the active flag in it
// is the server's own answer to "what am I showing", rather than something
// reconstructed here from a pin that may have gone stale. Deriving it twice is
// how a client ends up disagreeing with the server about what is on its own
// screen.

// DesktopWindow is one desktop window as a phone sees it: the census entry
// joined to the name the layout gives its workspace.
//
// It is deliberately a window and not a workspace. A workspace with no window
// on it is still running and still in the sidebar, but it is not something to
// "follow": following means seeing what somebody at the desk is seeing.
type DesktopWindow struct {
	// WorkspaceID is the workspace this window is showing, resolved by the
	// server (a window on a closed workspace reports the one it fell back to,
	// not the stale id).
	WorkspaceID string
	// WorkspaceName is the display name, or "" when no layout has named it.
	WorkspaceName string
	// Cols and Rows are the window's grid in cells. Useful as a label
	// ("200x60") when two windows sit on the same workspace and the name alone
	// cannot tell them apart.
	Cols, Rows uint16
	// Focused: its OS window is in the foreground.
	Focused bool
	// Primary: it is the primary view, the most recently focused desktop
	// window, which is what an unpinned viewer follows and what catctl acts on.
	Primary bool
	// Followed: this connection is currently showing this window's workspace.
	Followed bool
}

// Label stays useful before the first layout arrives.
func (w DesktopWindow) Label() string {
	if w.WorkspaceName != "" {
		return w.WorkspaceName
	}
	return w.WorkspaceID
}

// ViewWorkspace is the workspace this connection is being shown, per the
// server. Read off the layout's active flag rather than from any local pin: a
// pin naming a workspace that has since been closed falls back server-side,
// and a viewer with no pin at all resolves to a workspace it never named.
// Empty until the first layout arrives.
func (s *Session) ViewWorkspace() string {
	if s.Layout == nil {
		return ""
	}
	for _, w := range s.Layout.Workspaces {
		if w.Active {
			return w.ID
		}
	}
	return ""
}

// DesktopWindows is the desktop windows currently connected, in census order.
//
// Viewers are left out: another phone is not something this one can look
// through. A window's Followed is workspace equality, not connection identity;
// the census gives connections no ids, and two windows on one workspace mirror
// anyway, so "following that window" honestly means "showing what it shows".
func (s *Session) DesktopWindows() []DesktopWindow {
	names := map[string]string{}
	if s.Layout != nil {
		for _, w := range s.Layout.Workspaces {
			names[w.ID] = w.Name
		}
	}
	showing := s.ViewWorkspace()
	var out []DesktopWindow
	if s.Clients == nil {
		return out
	}
	for _, v := range s.Clients.Views {
		if v.Viewer {
			continue
		}
		out = append(out, DesktopWindow{
			WorkspaceID: v.Workspace,
			// "" when the layout has not named it. That happens legitimately
			// (a census can arrive before the first layout), so a UI shows
			// the id rather than treating it as an error.
			WorkspaceName: names[v.Workspace],
			Cols:          v.Cols,
			Rows:          v.Rows,
			Focused:       v.Focused,
			Primary:       v.Primary,
			Followed:      v.Workspace != "" && v.Workspace == showing,
		})
	}
	return out
}

// FollowedWindow is the window this connection is currently looking through,
// or nil when the census has not arrived or no desktop window shows this
// workspace (the desktop quit and left the session running, say).
func (s *Session) FollowedWindow() *DesktopWindow {
	for _, w := range s.DesktopWindows() {
		if w.Followed {
			return &w
		}
	}
	return nil
}

// PrimaryWindow is the primary view: the desktop window every view-less
// caller and every unpinned viewer resolves through. Nil before the first
// census.
func (s *Session) PrimaryWindow() *DesktopWindow {
	for _, w := range s.DesktopWindows() {
		if w.Primary {
			return &w
		}
	}
	return nil
}

// ViewerCount is how many connections are viewers (phones, tablets), this one
// included. Total minus Sizers says the same thing without needing Views;
// this reads it off the views when they are there.
func (s *Session) ViewerCount() int {
	c := s.Clients
	if c == nil {
		return 0
	}
	if len(c.Views) == 0 {
		return c.Total - c.Sizers
	}
	n := 0
	for _, v := range c.Views {
		if v.Viewer {
			n++
		}
	}
	return n
}

// Roster is the agents in the order the home screen shows them.
//
// Grouped by what needs attention, not by where it lives. Within a group, the
// longest-waiting first: an agent that has been blocked for twenty minutes
// outranks one that blocked ten seconds ago, and since_ms is exactly that
// number. Ties break on the public handle so the order is total and stable
// across renders.
func (s *Session) Roster() []wire.AgentItem {
	sorted := slices.Clone(s.Agents)
	slices.SortFunc(sorted, func(a, b wire.AgentItem) int {
		return cmp.Or(
			cmp.Compare(groupRank(a), groupRank(b)),
			cmp.Compare(b.SinceMs, a.SinceMs),
			cmp.Compare(a.Pub, b.Pub),
		)
	})
	return sorted
}

// groupRank: blocked → working → done-unseen → idle → anything else.
func groupRank(item wire.AgentItem) int {
	// seen: false is "finished while you were away" and outranks idle: it is
	// the whole reason somebody picks up the phone. It matches how the web UI
	// renders the same flag as "Done". A blocked agent stays first even when
	// unseen: "finished" is a nudge, "blocked" is a request.
	if !item.Seen && item.State != wire.AgentBlocked {
		return 2
	}
	switch item.State {
	case wire.AgentBlocked:
		return 0
	case wire.AgentWorking:
		return 1
	case wire.AgentIdle:
		return 3
	default:
		return 4
	}
}
