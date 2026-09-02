package catsapp

import (
	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
	"github.com/rohanthewiz/grmob/core"
)

// App is the root view grmob renders. Registered with the mobile bridge in
// register.go's init.
//
// Two wrappers, in this order:
//
//	WithTheme    the server's palette once the `theme` message lands, the
//	             dark fallback before
//	  Navigator  the route stack, seeded with the shell
//
// The theme is outside the Navigator so a pushed pane screen wears it too.
func App(ctx *core.Context) core.View {
	services := Get()
	// Wires ctx.RequestRender and dials the active endpoint. Idempotent:
	// this runs on every pass, and only the first does anything.
	services.Bind(ctx)

	var serverTheme *wire.Theme
	services.Conn.Read(func(s *catsclient.Session) { serverTheme = s.Theme })
	return core.WithTheme(themeFor(serverTheme), core.Navigator(shell))
}

// tab is one entry in the bottom navigation.
type tab struct {
	// Key names the tab's context scope. Stable and distinct: the scope is
	// where the tab's hook slots live.
	Key    string
	Glyph  string
	Label  string
	Screen func(*core.Context) core.View
}

// tabs is the bottom nav, in order. Agents first because it is the product:
// "see what every agent is doing". Windows is how the phone chooses what to
// look through; Alerts is what the desk wanted the phone to know; More is
// the plumbing.
var tabs = []tab{
	{Key: "agents", Glyph: "🐾", Label: "Agents", Screen: agentsScreen},
	{Key: "windows", Glyph: "🪟", Label: "Windows", Screen: windowsScreen},
	{Key: "alerts", Glyph: "🔔", Label: "Alerts", Screen: alertsScreen},
	{Key: "more", Glyph: "☰", Label: "More", Screen: moreScreen},
}

// shell is the Navigator's root route.
//
// Until the phone has paired, the shell IS the pair screen: there is no
// session to show tabs over, and a pair screen pushed as a frame would
// invite backing out of it into an empty app. Once an endpoint exists the
// shell is the selected tab above the bottom bar, with a connection banner
// between them whenever the socket is not up.
//
// Hand-rolled rather than core.TabView for church_mobile's reasons: TabView
// is a top bar on the natives, the WASM runtime does not implement its
// selection, and it drops per-tab state on switch. Each tab renders into its
// own ctx.Scope so an unrendered tab keeps its state.
func shell(ctx *core.Context) core.View {
	// Hooks first, unconditionally, before the branch below.
	selected := core.NewState(ctx, 0)
	services := Get()

	if !services.Paired() {
		return pairScreen(ctx.Scope("pair"))
	}

	index := selected.Get()
	if index < 0 || index >= len(tabs) {
		index = 0
	}
	active := tabs[index]

	return components.Screen{
		Fill: true,
		Children: []core.View{
			connectionBanner(ctx, services.Conn),
			// FlexGrow(1) is what pushes the nav bar to the bottom edge: the
			// content box claims the leftover height.
			core.Box(
				core.FlexGrow(1),
				core.Overflow("hidden"),
				active.Screen(ctx.Scope("tab:"+active.Key)),
			),
			navBar(ctx, index, selected.Set),
		},
		Style: []core.StyleProp{
			core.Gap(0),
			core.PaddingHorizontal(0),
			core.PaddingVertical(0),
			core.BackgroundColor(ctx.Theme().Colors.Background),
		},
	}
}

// connectionBanner is the strip under the status bar that says why the
// screen might be stale. Nil (no node at all) while connected.
//
// The hard stops get their own wording because each asks the user for a
// different thing: a changed certificate is the one that deserves alarm and
// a "forget device" path; an expired session wants a fresh QR; a protocol
// mismatch wants an app update. Reconnecting gets a "Retry" that resets the
// backoff ladder.
func connectionBanner(ctx *core.Context, conn *Connection) core.View {
	info := conn.Info()
	switch info.Status {
	case StatusConnected:
		return nil
	case StatusConnecting:
		return noticeStrip(ctx, "Connecting to "+info.Endpoint.String()+"…", "", nil)
	case StatusReconnecting:
		return noticeStrip(ctx, "Reconnecting… showing the last known state", "Retry", conn.Retry)
	case StatusCertMismatch:
		return noticeStrip(ctx,
			"This is not the server you paired with: its certificate changed ("+
				shortFingerprint(info.CertSeen)+"). Forget the device under More and pair again if you trust it.",
			"", nil)
	case StatusUnauthorized:
		return noticeStrip(ctx, "Your session has expired. Pair again from More.", "", nil)
	case StatusVersionMismatch:
		return noticeStrip(ctx, "This app is too old for that server. Update the app.", "", nil)
	default:
		return noticeStrip(ctx, "Not connected.", "Connect", conn.Retry)
	}
}

// navBar draws the four destinations. The selected one takes the theme's
// Primary ink; the rest are muted.
func navBar(ctx *core.Context, selected int, onSelect func(int)) core.View {
	theme := ctx.Theme()
	items := []core.PropsAndChildren{
		core.BackgroundColor(theme.Colors.Background),
		core.Justify(core.JustifyAround),
		core.AlignItemsProp(core.AlignItemsCenter),
		core.PaddingHorizontal(4),
		core.PaddingVertical(6),
		core.Gap(0),
	}
	for i, t := range tabs {
		active := i == selected
		ink := theme.Colors.TextSecondary
		if active {
			ink = theme.Colors.Primary
		}
		index := i
		items = append(items, core.Keyed(t.Key, core.Box(
			core.OnClick(func() { onSelect(index) }),
			core.FlexGrow(1),
			core.AlignItemsProp(core.AlignItemsCenter),
			core.PaddingVertical(2),
			core.AccessibilityLabel(navLabel(t.Label, active)),
			core.Column(
				core.AlignItemsProp(core.AlignItemsCenter),
				core.Gap(2),
				core.PaddingHorizontal(0),
				core.PaddingVertical(0),
				core.Text(t.Glyph, core.FontSize(20)),
				core.Text(t.Label, core.FontSize(11), core.TextColor(ink)),
			),
		)))
	}
	return core.Column(
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		components.Separator{},
		core.Row(items...),
	)
}

// navLabel spells the selection out for a screen reader; the visual cue is a
// colour change, which assistive tech cannot see.
func navLabel(label string, selected bool) string {
	if selected {
		return label + ", selected"
	}
	return label
}
