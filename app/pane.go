package catsapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
	"github.com/rohanthewiz/grmob/core"
)

// paneView is the pane screen's local state: the composer text and which
// confirmation is open. One slot, one struct (see pairForm).
type paneView struct {
	Draft string
	Busy  bool
	// ConfirmReveal is the "move the desktop's focus to this pane" dialog.
	// It is the one action on this screen that changes something at the
	// desk, and it never happens without this.
	ConfirmReveal bool
}

// paneScreen is the pane viewer: chrome (title, cwd, branch, agent state,
// exit code), the terminal grid, and the reply composer.
//
// # Read-only by default
//
// The grid is a rendering of frames the server streams for panes visible
// at the desk. Nothing on this screen sends a keystroke as a side effect of
// looking: no focus report (Conn.Send refuses it), no resize (the four
// layers), no mouse. The composer is the only way text leaves the phone,
// and it is pane-addressed.
//
// # Why the composer uses pane.send_input
//
// The plan's draft had the composer send Paste and Enter send Key. Both
// need CapKeyPane to be addressed to THIS pane; without it they ride the
// desktop's shared focus and land wherever the person at the desk last
// clicked. pane.send_input is the same thing done as a command: addressed by
// construction, paste-encoded server-side against the pane's live modes,
// with Submit as the Enter. One round trip, one ok, and no way to type into
// the wrong pane.
//
// # Panes the desk is not showing
//
// Frames stream only for visible panes. An agent in the roster whose pane
// is on another tab has chrome (title, state) but no grid, and the screen
// says so rather than showing an empty box. Following that pane's window
// (Windows tab) is how to see it.
func paneScreen(pane uint32, pub string) func(*core.Context) core.View {
	return func(ctx *core.Context) core.View {
		services := Get()
		conn := services.Conn
		theme := ctx.Theme()

		slot := core.NewState(ctx, &paneView{})
		view := slot.Get()
		// update is copy-on-write: a send completes on its own goroutine
		// while a render pass may be reading the current struct, so it
		// copies the latest value, mutates the copy, and stores that.
		update := func(mutate func(*paneView)) {
			next := *slot.Get()
			mutate(&next)
			slot.Set(&next)
		}

		// Everything the pass needs from the session, read once under the
		// lock, then rendered outside it.
		var (
			title, cwd, branch string
			agent              wire.PaneAgent
			hasAgent           bool
			exit               int
			exited             bool
			modes              wire.PaneModes
			rows               []core.GridRow
			hasFrame           bool
			defFg, defBg       string
			dropped            int
		)
		conn.Read(func(s *catsclient.Session) {
			title = s.Titles[pane]
			cwd = s.Cwds[pane]
			branch = s.Branches[pane]
			agent, hasAgent = s.PaneAgents[pane]
			if !hasAgent {
				// pane_agent is sent on a change; on connect the rollup is
				// what names every agent. Fall back to it so a pane opened
				// from the roster shows the state the roster showed.
				for _, item := range s.Agents {
					if item.Pane == pane {
						agent = wire.PaneAgent{Pane: pane, Agent: item.Agent, State: item.State,
							Model: item.Model, Seen: item.Seen}
						hasAgent = true
						break
					}
				}
			}
			exit, exited = s.ExitCodes[pane]
			modes = s.Modes[pane]
			if g, ok := s.Grids[pane]; ok && g.HasFullFrame() {
				hasFrame = true
				rows = gridRows(g)
				defFg, defBg = cssColor(g.DefFg()), cssColor(g.DefBg())
				dropped = g.DroppedCells
			}
		})
		if title == "" {
			title = pub
		}

		send := func(submit bool) {
			text := view.Draft
			if view.Busy || (text == "" && !submit) {
				return
			}
			update(func(v *paneView) { v.Busy = true })
			go func() {
				err := func() error {
					live := conn.Live()
					if live == nil {
						return fmt.Errorf("not connected")
					}
					return live.PaneSendInput(context.Background(), wire.SendInputParams{
						Pane: pane, Text: text, Submit: submit,
					})
				}()
				if err != nil {
					update(func(v *paneView) { v.Busy = false })
					toast("Could not send: " + err.Error())
					return
				}
				update(func(v *paneView) { v.Busy = false; v.Draft = "" })
			}()
		}

		reveal := func() {
			update(func(v *paneView) { v.ConfirmReveal = false })
			go runCommand("reveal the pane at the desk", func(c *catsclient.Conn) error {
				_, err := catsclient.Call[struct{}](context.Background(), c, wire.CmdPaneFocus, wire.PaneParams{Pane: pane})
				return err
			})
		}

		var body core.View
		switch {
		case hasFrame:
			body = core.Scroll(
				core.FlexGrow(1),
				core.Padding(8),
				core.TextGrid(rows,
					core.FontSize(12),
					core.TextColor(defFg),
					core.Background(defBg),
					core.AccessibilityLabel("Terminal output for "+title),
				),
			)
		default:
			body = core.Box(core.FlexGrow(1),
				emptyNote(ctx, "This pane is not on screen at the desk, so no output is streaming. "+
					"Follow its window from the Windows tab to watch it."))
		}

		return core.Column(
			core.Gap(0),
			core.PaddingHorizontal(0),
			core.PaddingVertical(0),
			core.FlexGrow(1),
			// The composer is docked below the scrolling grid, outside it, so it
			// is exactly the thing a software keyboard would cover. Lifting the
			// whole column keeps the reply bar in view while typing; the grid
			// above it shrinks, which is what a chat-shaped screen wants.
			core.KeyboardAware(),
			core.BackgroundColor(theme.Colors.Background),
			screenHeader(ctx, title,
				headerAction("👁", "Reveal this pane at the desk", func() {
					update(func(v *paneView) { v.ConfirmReveal = true })
				}),
			),
			paneChrome(ctx, pub, cwd, branch, agent, hasAgent, exit, exited, modes, dropped),
			body,
			composer(ctx, view, func(v string) { update(func(p *paneView) { p.Draft = v }) }, send),
			confirmDialog(ctx, view.ConfirmReveal,
				"Reveal at the desk?",
				"This moves the desktop's focus to "+pub+". Whoever is at the desk will see it switch.",
				"Reveal",
				reveal,
				func() { update(func(v *paneView) { v.ConfirmReveal = false }) },
			),
		)
	}
}

// paneChrome is the facts strip under the header: handle, cwd, branch, the
// agent's state and model, the exit code, and the mode hints a reader of a
// terminal wants (alt screen, mouse).
func paneChrome(ctx *core.Context, pub, cwd, branch string, agent wire.PaneAgent, hasAgent bool,
	exit int, exited bool, modes wire.PaneModes, dropped int) core.View {
	theme := ctx.Theme()
	items := []core.PropsAndChildren{
		core.Gap(4),
		core.PaddingHorizontal(12),
		core.PaddingVertical(8),
		core.BackgroundColor(theme.Colors.Surface),
	}
	line1 := []core.PropsAndChildren{
		core.Gap(8),
		core.AlignItemsProp(core.AlignItemsCenter),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		monoText(ctx, pub),
	}
	if hasAgent && agent.Agent != "" {
		line1 = append(line1, stateBadge(ctx, agent.State, agent.Seen),
			mutedText(ctx, joinNonEmpty(bullet, agent.Agent, agent.Model)))
	}
	if exited {
		line1 = append(line1, components.Badge{
			Text:    fmt.Sprintf("exited %d", exit),
			Variant: exitVariant(exit),
		})
	}
	items = append(items, core.Row(line1...))
	if facts := joinNonEmpty(bullet, cwd, branchText(branch), modeText(modes)); facts != "" {
		items = append(items, mutedText(ctx, facts))
	}
	if dropped > 0 {
		// Cells the grid refused: a diff before the first full frame, or an
		// index off the grid. Shown because a debug view is where this
		// number is useful and this is the closest thing to one.
		items = append(items, mutedText(ctx, fmt.Sprintf("%d cells dropped awaiting a full frame", dropped)))
	}
	return core.Column(items...)
}

func exitVariant(code int) components.Variant {
	if code == 0 {
		return components.VariantSuccess
	}
	return components.VariantError
}

func branchText(branch string) string {
	if branch == "" {
		return ""
	}
	return "⎇ " + branch
}

func modeText(m wire.PaneModes) string {
	var parts []string
	if m.AltScreen {
		parts = append(parts, "alt screen")
	}
	if m.Mouse {
		parts = append(parts, "mouse")
	}
	return strings.Join(parts, ", ")
}

// composer is the reply bar: a text area, "Type" (stage the text without
// Enter, for a prompt the user wants to review at the desk or a program that
// filters as you type) and "Send" (text followed by Enter). An empty draft
// with Send is just an Enter, which is how you answer a "press enter to
// continue".
func composer(ctx *core.Context, view *paneView, onChange func(string), send func(submit bool)) core.View {
	theme := ctx.Theme()
	return core.Column(
		core.Gap(6),
		core.PaddingHorizontal(12),
		core.PaddingVertical(8),
		core.BackgroundColor(theme.Colors.Surface),
		core.TextArea(view.Draft, onChange, 2,
			core.AccessibilityLabel("Reply to the pane"),
		),
		core.Row(
			core.Justify(core.JustifyEnd),
			core.Gap(8),
			core.PaddingHorizontal(0),
			core.PaddingVertical(0),
			components.Button{
				Label:              "Type",
				Emphasis:           components.EmphasisOutlined,
				Disabled:           view.Busy || view.Draft == "",
				OnTap:              func() { send(false) },
				AccessibilityHint:  "Types the text into the pane without pressing Enter",
				AccessibilityLabel: "Type without Enter",
			},
			components.Button{
				Label:             sendLabel(view),
				Disabled:          view.Busy,
				OnTap:             func() { send(true) },
				AccessibilityHint: "Types the text into the pane and presses Enter",
			},
		),
	)
}

func sendLabel(view *paneView) string {
	switch {
	case view.Busy:
		return "Sending…"
	case view.Draft == "":
		return "Enter"
	default:
		return "Send"
	}
}
