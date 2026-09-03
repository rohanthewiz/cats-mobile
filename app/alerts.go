package catsapp

import (
	"context"
	"fmt"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
	"github.com/rohanthewiz/grmob/core"
)

// alertsScreen is what the desk wanted the phone to know: the notification
// history, the macro recorder's state, and the runbook runs in flight.
//
// # The record indicator is nullable, and null draws nothing
//
// Session.Record is nil until the server pushes one, which a server too old
// to send `record` never does. Nil means "unknown", not "idle": drawing an
// unlit indicator would vouch for a state the phone cannot know. So the
// block is absent until the first push, then shows exactly what the server
// said.
//
// # Notification actions are declared effects
//
// A notification's buttons carry what to do (send this text to that pane),
// not a callback. Tapping one issues ui.action with the notification's id,
// and catway performs the send and announces it; the phone never injects
// the text itself. That keeps the "answered once" guarantee server-side,
// where a browser toast and this phone showing the same buttons cannot both
// answer.
func alertsScreen(ctx *core.Context) core.View {
	services := Get()
	conn := services.Conn

	var (
		record   *wire.Record
		runs     []wire.RunbookRun
		notifies []wire.Notify
		errs     []wire.Error
	)
	conn.Read(func(s *catsclient.Session) {
		record = s.Record
		runs = s.RunbookRuns
		notifies = s.Notifications
		errs = s.Errors
	})

	header := screenHeader(ctx, "Alerts")
	items := []core.PropsAndChildren{}

	if record != nil && record.Recording {
		items = append(items, sectionHeader(ctx, "RECORDING"),
			contentRow(ctx, "⏺", fmt.Sprintf("Recording %d steps", record.Steps),
				joinNonEmpty(bullet, agoLabel("started", record.StartedAt), record.Note), nil, nil))
	}

	if len(runs) > 0 {
		items = append(items, sectionHeader(ctx, "RUNBOOKS IN FLIGHT"))
		for _, run := range runs {
			items = append(items, core.Keyed("run:"+run.Name,
				contentRow(ctx, "▶", run.Name, runbookSubtitle(run), nil, nil)))
		}
	}

	if len(errs) > 0 {
		last := errs[len(errs)-1]
		items = append(items, noticeStrip(ctx, "Server error: "+last.Msg, "", nil))
	}

	items = append(items, sectionHeader(ctx, "NOTIFICATIONS"))
	if len(notifies) == 0 {
		items = append(items, emptyNote(ctx, "Nothing yet. Agents that block or finish show up here."))
	}
	// Newest first: the history is kept newest-last, and the phone wants
	// the latest at the top.
	for i := len(notifies) - 1; i >= 0; i-- {
		items = append(items, notifyRow(ctx, i, notifies[i]))
	}
	items = append(items, core.Spacer(24))
	return scrollColumn(ctx, header, items...)
}

func agoLabel(verb, rfc3339 string) string {
	if age := startedAgo(rfc3339); age != "" {
		return verb + " " + age + " ago"
	}
	return ""
}

// runbookSubtitle says where a run is and who started it. Step 0 is a run
// that has its slot and has not reached its first step, a real state a
// triggered run passes through.
func runbookSubtitle(run wire.RunbookRun) string {
	progress := "starting"
	if run.Step > 0 {
		progress = fmt.Sprintf("step %d of %d", run.Step, run.Steps)
	}
	source := ""
	switch run.Source {
	case "trigger":
		source = "triggered by " + run.Trigger
	case "control":
		source = "started by hand"
	}
	return joinNonEmpty(bullet, progress, source, agoLabel("started", run.StartedAt))
}

// notifyRow is one notification: glyph by kind, the message, the body and
// pane, then its action buttons when it has any. Tapping the row opens the
// pane it names.
func notifyRow(ctx *core.Context, index int, n wire.Notify) core.View {
	theme := ctx.Theme()
	var onTap func()
	if n.Pane != 0 {
		pane, pub := n.Pane, n.Pub
		onTap = func() { core.Push(ctx, paneScreen(pane, pub)) }
	}
	row := contentRow(ctx, notifyGlyph(n.Kind), n.Message, joinNonEmpty(bullet, n.Pub, n.Body), onTap, nil)
	if len(n.Actions) == 0 || n.ID == "" {
		return core.Keyed(fmt.Sprintf("n:%d", index), row)
	}
	buttons := []core.PropsAndChildren{
		core.Gap(8),
		core.FlexWrap(true),
		core.PaddingHorizontal(16),
		core.PaddingVertical(4),
	}
	for _, a := range n.Actions {
		id, action, label := n.ID, a.ID, a.Label
		buttons = append(buttons, components.Button{
			Label:    label,
			Emphasis: components.EmphasisOutlined,
			Style:    []core.StyleProp{core.FontSize(13)},
			OnTap: func() {
				go runCommand("answer "+label, func(c *catsclient.Conn) error {
					_, err := catsclient.Call[struct{}](context.Background(), c, wire.CmdUIAction,
						wire.UIActionParams{ID: id, Action: action})
					if err == nil {
						// Accepted: the buttons are spent everywhere now, and
						// this phone is the one client that knows without a
						// round trip. See Session.AnswerNotify.
						Get().Conn.Update(func(s *catsclient.Session) { s.AnswerNotify(id) })
					}
					return err
				})
			},
		})
	}
	return core.Keyed(fmt.Sprintf("n:%d", index), core.Column(
		core.Gap(0),
		core.PaddingHorizontal(0),
		core.PaddingVertical(0),
		core.BackgroundColor(theme.Colors.Background),
		row,
		core.Row(buttons...),
	))
}
