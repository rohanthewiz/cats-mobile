package catsapp

import (
	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/comps"
	"github.com/rohanthewiz/grmob/core"
)

// agentsScreen is the roster: every agent across every workspace, grouped by
// what needs attention and, within a group, longest-waiting first. This is
// the product; the other tabs exist so this one has somewhere to go.
//
// Below the agents go the plugin panes, split the way the desktop sidebar
// splits them (Session.SplitPlugins):
//
//	NEEDS YOU / WORKING / IDLE   the roster, by state
//	AGENT PLUGINS                plugins whose manifest says type "agent"
//	PLUGINS                      every other plugin pane (editors, backlogs…)
//
// Agent plugins get a heading of their own rather than trailing the last
// state group, as they do on the desktop: here every run of rows sits under a
// state heading, so a headless block after IDLE would read as idle agents,
// and a plugin has no state to claim. The PLUGINS section is drawn only when
// there is something in it, as on the desktop: most sessions run no plugin,
// and an always-empty heading is clutter.
//
// The ordering is Session.Roster's (a product decision recorded there); this
// screen only draws the section breaks where the state changes. It reads
// the session on every pass rather than subscribing: every message triggers
// a render through the connection's notify, so the roster is never more than
// one pass behind the wire.
func agentsScreen(ctx *core.Context) core.View {
	services := Get()
	conn := services.Conn

	var (
		roster                []wire.AgentItem
		agentPlugs, toolPlugs []wire.PluginPane
		title                 string
	)
	conn.Read(func(s *catsclient.Session) {
		roster = s.Roster()
		agentPlugs, toolPlugs = s.SplitPlugins()
		title = s.AppTitle
	})
	if title == "" {
		title = "Agents"
	}

	header := screenHeader(ctx, title)
	if len(roster) == 0 && len(agentPlugs) == 0 && len(toolPlugs) == 0 {
		return scrollColumn(ctx, header, emptyNote(ctx, rosterEmptyText(conn.Info().Status)))
	}

	items := make([]core.PropsAndChildren, 0, (len(roster)+len(agentPlugs)+len(toolPlugs))*2)
	// The agents half says "none" only when neither detected agents nor
	// agent plugins are running, the same test the desktop's AGENTS section
	// uses. Tool plugins alone still leave it empty: a backlog open in a pane
	// is not an agent, and the note is what tells the user so.
	if len(roster) == 0 && len(agentPlugs) == 0 {
		items = append(items, emptyNote(ctx, rosterEmptyText(conn.Info().Status)))
	}
	lastState := ""
	for _, item := range roster {
		if item.State != lastState {
			items = append(items, sectionHeader(ctx, stateGroupTitle(item.State)))
			lastState = item.State
		}
		items = append(items, agentRow(ctx, conn, item))
	}
	if len(agentPlugs) > 0 {
		items = append(items, sectionHeader(ctx, "AGENT PLUGINS"))
		items = appendPluginBlocks(ctx, items, agentPlugs)
	}
	if len(toolPlugs) > 0 {
		items = append(items, sectionHeader(ctx, "PLUGINS"))
		items = appendPluginBlocks(ctx, items, toolPlugs)
	}
	items = append(items, core.Spacer(24))
	return scrollColumn(ctx, header, items...)
}

// appendPluginBlocks appends one row per plugin pane, with a hairline wherever
// the plugin id changes, so two panes of the same plugin read as one block and
// a different plugin starts a new one. The rollup arrives grouped by plugin
// id and SplitPlugins keeps that order, so the cut is a single walk comparing
// each row with the one before it — no sorting or map here.
//
// The rule goes only between blocks, not under the last one as on the
// desktop: there the rule closes a block against whatever sidebar section
// follows, while here the next thing is a section heading or the end of the
// scroll, both of which already end the block.
func appendPluginBlocks(ctx *core.Context, items []core.PropsAndChildren, plugs []wire.PluginPane) []core.PropsAndChildren {
	for i, p := range plugs {
		if i > 0 && p.Plugin != plugs[i-1].Plugin {
			items = append(items, comps.Separator{})
		}
		items = append(items, pluginRow(ctx, p))
	}
	return items
}

// pluginRow is one plugin-pane line: the plugin's short name, then the
// pane's live terminal title, handle and workspace, with the plugin's type
// where an agent row has its state badge. Tap opens the pane, as on an agent
// row: "take me to it" is the same job.
//
// The terminal title leads the subtitle because it is the plugin's one
// channel to the user (cats-todo puts its open count there, an editor the
// file it has open); the handle and workspace are how to find it.
//
// There is no state badge and no age. A plugin is a program, not something
// taking turns, so it is never idle, working or blocked, and the server sends
// no instant for it. Drawing "Unknown" there would make every plugin row look
// like a broken agent; the type badge says what the row actually knows.
func pluginRow(ctx *core.Context, p wire.PluginPane) core.View {
	pane, pub := p.Pane, p.Pub
	subtitle := joinNonEmpty(bullet, p.Title, p.Pub, p.Workspace, flagText(p.FlagInfo))
	var trailing core.View
	if label := pluginTypeLabel(p.Type); label != "" {
		trailing = comps.Badge{Text: label}
	}
	// The key is prefixed because the same pane can, in principle, be in both
	// lists (an agent plugin whose agent cats also detects), and keys must be
	// unique among siblings for the reconciler to keep row state straight.
	return core.Keyed("plugin:"+pub, contentRow(ctx, "", pluginShort(p.Plugin), subtitle,
		func() { core.Push(ctx, paneScreen(pane, pub)) },
		trailing,
	))
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
