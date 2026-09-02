package catsapp

import (
	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
)

// agentsScreen is the roster: every agent across every workspace, grouped by
// what needs attention and, within a group, longest-waiting first. This is
// the product; the other tabs exist so this one has somewhere to go.
//
// The ordering is Session.Roster's (a product decision recorded there); this
// screen only draws the section breaks where the state changes. It reads
// the session on every pass rather than subscribing: every message triggers
// a render through the connection's notify, so the roster is never more than
// one pass behind the wire.
func agentsScreen(ctx *core.Context) core.View {
	services := Get()
	conn := services.Conn

	var roster []wire.AgentItem
	var title string
	conn.Read(func(s *catsclient.Session) {
		roster = s.Roster()
		title = s.AppTitle
	})
	if title == "" {
		title = "Agents"
	}

	header := screenHeader(ctx, title)
	if len(roster) == 0 {
		return scrollColumn(ctx, header, emptyNote(ctx, rosterEmptyText(conn.Info().Status)))
	}

	items := make([]core.PropsAndChildren, 0, len(roster)*2)
	lastState := ""
	for _, item := range roster {
		if item.State != lastState {
			items = append(items, sectionHeader(ctx, stateGroupTitle(item.State)))
			lastState = item.State
		}
		items = append(items, agentRow(ctx, conn, item))
	}
	items = append(items, core.Spacer(24))
	return scrollColumn(ctx, header, items...)
}

// rosterEmptyText says why there is nothing, which differs by connection
// state: "no agents" is only true once we are connected and the rollup has
// arrived.
func rosterEmptyText(status Status) string {
	switch status {
	case StatusConnected:
		return "No agents running. Panes with an agent show up here as soon as one starts."
	case StatusConnecting, StatusReconnecting:
		return "Waiting for the desk…"
	default:
		return "Not connected."
	}
}

// agentRow is one roster line: the model or agent, the pane handle and
// workspace, the state badge, and how long it has been in that state. Tap
// opens the pane.
func agentRow(ctx *core.Context, conn *Connection, item wire.AgentItem) core.View {
	pane := item.Pane
	subtitle := joinNonEmpty(bullet, item.Pub, item.Workspace, flagText(item.FlagInfo))
	if age := ago(conn.AgentAge(item)); age != "" {
		subtitle = joinNonEmpty(bullet, subtitle, age)
	}
	return core.Keyed(item.Pub, contentRow(ctx, "", agentTitle(item), subtitle,
		func() { core.Push(ctx, paneScreen(pane, item.Pub)) },
		stateBadge(ctx, item.State, item.Seen),
	))
}

// flagText renders a pane flag as the roster shows it: the glyph or kind,
// with its note when there is one.
func flagText(f wire.FlagInfo) string {
	if f.Flag == "" {
		return ""
	}
	return joinNonEmpty(" ", f.Flag, f.FlagNote)
}
