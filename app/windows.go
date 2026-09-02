package catsapp

import (
	"context"
	"fmt"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
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
	conn.Read(func(s *catsclient.Session) {
		windows = s.DesktopWindows()
		followed = s.FollowedWindow()
		census = s.Clients
		viewers = s.ViewerCount()
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

	items := []core.PropsAndChildren{
		core.Row(
			core.PaddingHorizontal(16),
			core.PaddingVertical(8),
			mutedText(ctx, censusLine(census, viewers, followed)),
		),
	}
	if !canFollow {
		items = append(items, noticeStrip(ctx,
			"This desk's server does not support per-window following; the phone shows the primary view.", "", nil))
	}
	for _, w := range windows {
		items = append(items, windowRow(ctx, conn, w, canFollow))
	}
	if following && canFollow {
		items = append(items, core.Row(
			core.Padding(16),
			components.Button{
				Label:     "Follow the primary view",
				Emphasis:  components.EmphasisOutlined,
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
func windowRow(ctx *core.Context, conn *Connection, w catsclient.DesktopWindow, canFollow bool) core.View {
	var badges []core.PropsAndChildren
	badges = append(badges, core.Gap(4), core.PaddingHorizontal(0), core.PaddingVertical(0))
	if w.Followed {
		badges = append(badges, components.Badge{Text: "Showing", Variant: components.VariantSuccess})
	}
	if w.Primary {
		badges = append(badges, components.Badge{Text: "Primary"})
	}
	if w.Focused {
		badges = append(badges, components.Badge{Text: "Focused", Variant: components.VariantWarning})
	}
	subtitle := joinNonEmpty(bullet, w.WorkspaceID, fmt.Sprintf("%d×%d", w.Cols, w.Rows))

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
