package catsapp

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
	"github.com/rohanthewiz/grmob/core"
)

// moreConfirm names which confirmation dialog is open, "" for none. One
// value rather than a bool per dialog: they are mutually exclusive.
type moreConfirm string

const (
	confirmNone   moreConfirm = ""
	confirmForget moreConfirm = "forget"
)

// moreScreen is the plumbing: the paired desks and which is active, the
// connection's state and capabilities, "forget this device", and the build
// identity (app version plus the cats sha the wire package was pinned at).
func moreScreen(ctx *core.Context) core.View {
	services := Get()
	conn := services.Conn

	confirm := core.NewState(ctx, confirmNone)

	info := conn.Info()
	active, _ := services.Store.Active()
	endpoints := services.Store.Endpoints()
	var viewers int
	var shutdown bool
	var update *wire.UpdateReady
	var usage *wire.Usage
	conn.Read(func(s *catsclient.Session) {
		viewers = s.ViewerCount()
		shutdown = s.ServerShutDown
		update = s.UpdateReady
		usage = s.Usage
	})

	items := []core.PropsAndChildren{}

	items = append(items, sectionHeader(ctx, "CONNECTION"))
	items = append(items, contentRow(ctx, statusGlyph(info.Status), info.Status.String(),
		joinNonEmpty(bullet, info.Endpoint.Label(), errText(info.Err)), nil,
		components.Button{Label: "Retry", Emphasis: components.EmphasisGhost, OnTap: conn.Retry,
			Style: []core.StyleProp{core.FontSize(13)}}))
	if info.Status == StatusConnected {
		items = append(items, contentRow(ctx, "", "Server capabilities",
			capsText(info.Caps, viewers), nil, nil))
	}
	if shutdown {
		items = append(items, noticeStrip(ctx, "The desk's server has shut down.", "", nil))
	}
	if update != nil {
		items = append(items, noticeStrip(ctx,
			"A cats update is ready at the desk: "+update.Version+" ("+update.Command+")", "", nil))
	}
	if line := usageLine(usage); line != "" {
		items = append(items, contentRow(ctx, "", "Usage", line, nil, nil))
	}

	items = append(items, sectionHeader(ctx, "DESKS"))
	for _, e := range endpoints {
		endpoint := e
		isActive := e.ID == active.ID
		var trailing core.View
		if isActive {
			trailing = components.Badge{Text: "Active", Variant: components.VariantSuccess}
		}
		var onTap func()
		if !isActive {
			onTap = func() {
				if err := services.Store.SetActive(endpoint.ID); err != nil {
					toast("Could not switch: " + err.Error())
					return
				}
				conn.Connect(endpoint)
			}
		}
		items = append(items, core.Keyed("ep:"+e.ID, contentRow(ctx, "🖥", e.Label(),
			joinNonEmpty(bullet, e.Kind.String(), "pin "+shortFingerprint(e.PinnedSHA256)), onTap, trailing)))
	}
	items = append(items,
		contentRow(ctx, "➕", "Pair another desk", "", func() { core.Push(ctx, pairScreen) }, nil),
		contentRow(ctx, "🗑", "Forget this device", "Clears the session and certificate pin for "+active.Label(),
			func() { confirm.Set(confirmForget) }, nil),
	)

	items = append(items, sectionHeader(ctx, "ABOUT"))
	items = append(items, contentRow(ctx, "ℹ️", "Cats "+appVersion(), buildLine(), nil, nil))
	items = append(items, core.Spacer(24))

	return core.Column(
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		core.FlexGrow(1),
		screenHeader(ctx, "More"),
		core.Scroll(core.FlexGrow(1), core.Column(append([]core.PropsAndChildren{
			core.Gap(0), core.PaddingHorizontal(0), core.PaddingVertical(0),
		}, items...)...)),
		confirmDialog(ctx, confirm.Get() == confirmForget,
			"Forget this device?",
			"The desk will no longer recognise this phone. Pair again to reconnect.",
			"Forget",
			func() {
				confirm.Set(confirmNone)
				conn.Disconnect()
				if err := services.Store.RemoveEndpoint(active.ID); err != nil {
					toast("Could not forget: " + err.Error())
				}
				if next, ok := services.Store.Active(); ok {
					conn.Connect(next)
				}
			},
			func() { confirm.Set(confirmNone) },
		),
	)
}

func statusGlyph(s Status) string {
	switch s {
	case StatusConnected:
		return "🟢"
	case StatusConnecting, StatusReconnecting:
		return "🟡"
	case StatusIdle:
		return "⚪"
	default:
		return "🔴"
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// capsText lists what the server honours, in the words a user might act on,
// plus the viewer count so "am I the only phone here" has an answer.
func capsText(caps []string, viewers int) string {
	var parts []string
	has := func(c string) bool {
		for _, x := range caps {
			if x == c {
				return true
			}
		}
		return false
	}
	if has(wire.CapViewer) {
		parts = append(parts, "viewer mode honoured")
	}
	if has(wire.CapWindow) {
		parts = append(parts, "per-window following")
	}
	if has(wire.CapKeyPane) {
		parts = append(parts, "pane-addressed input")
	}
	if len(parts) == 0 {
		parts = append(parts, "none advertised")
	}
	if viewers > 0 {
		parts = append(parts, fmt.Sprintf("%d viewer(s) connected", viewers))
	}
	return strings.Join(parts, bullet)
}

// usageLine is a one-line summary of the desk's usage message, or "" when
// nothing has arrived. The message's groups are opaque to the phone beyond
// their labels and percentages.
func usageLine(u *wire.Usage) string {
	if u == nil || len(u.Groups) == 0 {
		return ""
	}
	var parts []string
	for _, g := range u.Groups {
		for _, w := range g.Windows {
			if int(w.Pct) == wire.UsagePctUnknown {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %s %.0f%%", g.Name, w.Name, w.Pct))
		}
	}
	return strings.Join(parts, bullet)
}

// appVersion is the app's own version, from the config registered in init.
func appVersion() string { return "0.1.0" }

// buildLine names the cats sha this binary's wire package was pinned at,
// read from the module list in the build info. That is the whole lockstep
// story in one line: the phone speaks whatever cats version go.mod says.
func buildLine() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "build info unavailable"
	}
	for _, dep := range info.Deps {
		if dep.Path != "github.com/rohanthewiz/cats" {
			continue
		}
		v := dep.Version
		if dep.Replace != nil {
			v = "local checkout (" + dep.Replace.Path + ")"
		}
		return "cats wire " + v
	}
	return "cats wire: not linked"
}
