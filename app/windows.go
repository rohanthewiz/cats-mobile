package catsapp

import (
	"context"
	"fmt"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/comps"
	"github.com/rohanthewiz/grmob/core"
)

// windowsScreen is the census of desktop windows: which workspace each one
// shows, which is primary, which is focused, and which this phone is
// currently looking through. Tapping a window follows it; a button releases
// the pin back to the primary view.
//
// # What "following" means
//
// A viewer sees the workspace of one desktop window. With wire.CapWindow the
// server treats workspace.focus from this connection as a change to THIS
// connection's view only, so following is viewer-safe and needs no
// confirmation (catsclient.Conn.FollowWorkspace holds the capability check
// and refuses on an older server). The "Followed" mark comes from the
// server's own layout flag, not from the pin we asked for: a pin naming a
// workspace that has since closed falls back server-side, and the honest
// answer is what the server shows, not what we requested.
//
// Other viewers are left out of the list on purpose: another phone is not
// something this one can look through.
func windowsScreen(ctx *core.Context) core.View {
	services := Get()
	conn := services.Conn

	var windows []catsclient.DesktopWindow
	var followed *catsclient.DesktopWindow
	var census *wire.Clients
	var viewers int
	gitByWS := map[string]wire.WorkspaceGitInfo{}
	conn.Read(func(s *catsclient.Session) {
		windows = s.DesktopWindows()
		followed = s.FollowedWindow()
		census = s.Clients
		viewers = s.ViewerCount()
		// Copied out row by row rather than carried as a map reference. The
		// session's map happens to be replaced wholesale on every sweep rather
		// than mutated, so the reference would in fact stay safe — but only by
		// an invariant that is written in the fold, not visible here, and a
		// later patch-in-place there would silently make this a data race. A
		// handful of copies under the lock costs nothing and keeps the screen's
		// data its own.
		for _, w := range windows {
			if g, ok := s.GitSync(w.WorkspaceID); ok {
				gitByWS[w.WorkspaceID] = g
			}
		}
	})

	header := screenHeader(ctx, "Windows")
	if census == nil {
		return scrollColumn(ctx, header,
			emptyNote(ctx, "The desk has not sent its window list yet."))
	}
	if len(windows) == 0 {
		return scrollColumn(ctx, header,
			emptyNote(ctx, "No desktop window is open. The session is running with nobody at the desk."))
	}

	live := conn.Live()
	canFollow := live != nil && live.HasCap(wire.CapWindow)
	following := live != nil && !live.FollowsPrimaryView()
	// The "not supported" notice is a statement about the server, so it needs
	// a server to have said so. While the socket is down there is no live
	// connection to ask, and the answer is "not right now", which the
	// reconnect banner already says; blaming the desk's version for it would
	// be wrong and alarming.
	unsupported := live != nil && !canFollow

	items := []core.PropsAndChildren{
		core.Row(
			core.PaddingHorizontal(16),
			core.PaddingVertical(8),
			mutedText(ctx, censusLine(census, viewers, followed)),
		),
	}
	if unsupported {
		items = append(items, noticeStrip(ctx,
			"This desk's server does not support per-window following; the phone shows the primary view.", "", nil))
	}
	for _, w := range windows {
		git, known := gitByWS[w.WorkspaceID]
		items = append(items, windowRow(ctx, conn, w, canFollow, git, known))
	}
	if following && canFollow {
		items = append(items, core.Row(
			core.Padding(16),
			comps.Button{
				Label:     "Follow the primary view",
				Emphasis:  comps.EmphasisOutlined,
				FullWidth: true,
				OnTap: func() {
					go runCommand("follow the primary view", func(c *catsclient.Conn) error {
						return c.FollowPrimaryView(context.Background())
					})
				},
			},
		))
	}
	items = append(items, core.Spacer(24))
	return scrollColumn(ctx, header, items...)
}

// censusLine is the one-line summary above the list: how many connections,
// how many of them are phones, and which window this one is looking through.
func censusLine(c *wire.Clients, viewers int, followed *catsclient.DesktopWindow) string {
	line := fmt.Sprintf("%d connected", c.Total)
	if viewers > 0 {
		line += fmt.Sprintf(", %d viewing", viewers)
	}
	if c.Sizers == 0 {
		line += bullet + "no desktop is driving the layout"
	}
	if followed != nil {
		line += bullet + "showing " + followed.Label()
	}
	return line
}

// windowRow is one desktop window. The trailing badges say what the server
// says about it; the tap follows it when the server allows that.
//
// The badges divide into two kinds, and the git dot leads them because it is
// the odd one out: Showing, Primary and Focused are all facts about who is
// looking at this window, while the dot is a fact about the work inside it.
func windowRow(ctx *core.Context, conn *Connection, w catsclient.DesktopWindow, canFollow bool,
	git wire.WorkspaceGitInfo, known bool) core.View {
	var badges []core.PropsAndChildren
	badges = append(badges, core.Gap(4), core.PaddingHorizontal(0), core.PaddingVertical(0),
		gitSyncDot(ctx, git, known))
	if w.Followed {
		badges = append(badges, comps.Badge{Text: "Showing", Variant: comps.VariantSuccess})
	}
	if w.Primary {
		badges = append(badges, comps.Badge{Text: "Primary"})
	}
	if w.Focused {
		badges = append(badges, comps.Badge{Text: "Focused", Variant: comps.VariantWarning})
	}
	subtitle := joinNonEmpty(bullet, w.WorkspaceID, fmt.Sprintf("%d×%d", w.Cols, w.Rows),
		gitSyncPhrase(git, known))

	var onTap func()
	if canFollow && !w.Followed {
		id := w.WorkspaceID
		onTap = func() {
			go runCommand("follow "+id, func(c *catsclient.Conn) error {
				return c.FollowWorkspace(context.Background(), id)
			})
		}
	}
	return core.Keyed("win:"+w.WorkspaceID, contentRow(ctx, "🖥", w.Label(), subtitle, onTap, core.Row(badges...)))
}

// gitSyncDot is the mark at the head of a window row's badges: is this
// workspace's trunk level with its remote?
//
// # Colour is reinforcement, never the message
//
// The dot carries four states in three colours — level is green, not-level is
// amber whichever way it leans, and no answer is the muted ink of the subtitle
// beside it. Which way it leans is deliberately not in the colour at all.
// A phone has readers who cannot separate the tints and readers who hear the
// row rather than see it, and the words gitSyncPhrase puts in the subtitle are
// what serve both (grmob's comps.Badge states the same rule for the same
// reason: WCAG 1.4.1). So the dot is marked AccessibilityHidden — the subtitle
// has already said what it means, and a reader announcing both would say it
// twice.
//
// # Why "no answer" still draws a dot
//
// The server omits every workspace it has nothing to say about: not a
// checkout, no trunk, no remote, unreachable, or simply a sweep that has not
// run yet (the first is ten seconds behind the connect, then every two
// minutes). So "we do not know" and "we have not asked" arrive identically,
// and neither is a claim about the tree. Drawing the plain dot for them is
// what the desk draws, and it keeps the column from jumping sideways as rows
// gain and lose an answer between sweeps.
func gitSyncDot(ctx *core.Context, g wire.WorkspaceGitInfo, known bool) core.View {
	colors := ctx.Theme().Colors
	// The muted ink is the fallback in two cases, not one: no answer at all,
	// and a Sync value this build has no case for. A newer server naming a
	// fourth state should leave the row uncoloured rather than have it guess.
	ink := colors.TextSecondary
	if known {
		switch g.Sync {
		case wire.GitSynced:
			ink = colors.Success
		case wire.GitAhead, wire.GitBehind:
			ink = colors.Warning
		}
	}
	return core.Text("●",
		core.FontSize(10),
		core.TextColor(ink),
		core.AccessibilityHidden(),
	)
}

// gitSyncPhrase is the same answer the dot colours, said in words at the end
// of the row's subtitle. It is the only channel that separates ahead from
// behind, so it is not decoration.
//
// The branch and the remote are named rather than assumed. The server compares
// whichever trunk and remote the checkout actually has, so a tree still on
// master, or one whose main pushes to a fork instead of to origin, would
// otherwise report a state its owner cannot account for. Either field may be
// missing from the rollup, so each phrase degrades to the bare state instead
// of trailing a dangling preposition.
//
// Behind carries no count on purpose: the server's poll does not fetch, so the
// commits on the other side were never counted, and "behind by some" would be
// a number nobody published.
func gitSyncPhrase(g wire.WorkspaceGitInfo, known bool) string {
	if !known {
		return ""
	}
	switch g.Sync {
	case wire.GitSynced:
		if g.Remote == "" {
			return joinNonEmpty(" ", g.Branch, "in sync")
		}
		return joinNonEmpty(" ", g.Branch, "in sync with", g.Remote)
	case wire.GitAhead:
		// The count is omitted when the server sent none, rather than shown as
		// "0 ahead", which would contradict the state in the same breath.
		lead := "ahead"
		if g.Ahead > 0 {
			lead = fmt.Sprintf("%d ahead", g.Ahead)
		}
		if g.Remote == "" {
			return joinNonEmpty(" ", g.Branch, lead)
		}
		return joinNonEmpty(" ", g.Branch, lead, "of", g.Remote)
	case wire.GitBehind:
		return joinNonEmpty(" ", g.Branch, "behind", g.Remote)
	default:
		// A state this build does not know. The dot stays muted and the row
		// says nothing, rather than putting a newer server's word on screen as
		// though this one understood it.
		return ""
	}
}

// runCommand issues one command against the live connection and reports a
// failure as a toast. Run on a goroutine: commands block for their reply.
// The server's state message (a new layout, a new census) is what updates
// the screen; the command's own ok carries nothing a screen needs.
func runCommand(what string, fn func(*catsclient.Conn) error) {
	live := Get().Conn.Live()
	if live == nil {
		toast("Not connected; could not " + what + ".")
		return
	}
	if err := fn(live); err != nil {
		toast("Could not " + what + ": " + err.Error())
	}
}
