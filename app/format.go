package catsapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/components"
)

// Formatting helpers: the wording every screen shares.

// stateLabel is the text and colour role for an agent state. Seen is the
// rollup's "has the user looked since it changed" bit; an unseen finished
// agent reads "Done" (its output is waiting), which matters more on a phone
// than "idle" does.
func stateLabel(state string, seen bool) (string, components.Variant) {
	switch state {
	case wire.AgentBlocked:
		return "Needs you", components.VariantError
	case wire.AgentWorking:
		return "Working", components.VariantWarning
	case wire.AgentIdle:
		if !seen {
			return "Done", components.VariantSuccess
		}
		return "Idle", ""
	default:
		return "Unknown", ""
	}
}

// stateGroupTitle is the roster's section heading for a state. The order
// the groups appear in is Session.Roster's, so this only names them.
func stateGroupTitle(state string) string {
	switch state {
	case wire.AgentBlocked:
		return "NEEDS YOU"
	case wire.AgentWorking:
		return "WORKING"
	case wire.AgentIdle:
		return "IDLE"
	default:
		return "UNKNOWN"
	}
}

// ago renders a duration as the coarse age a list row wants: "12s", "4m",
// "2h 05m", "3d". Negative means the age was never published and renders
// as "" so the row says nothing rather than something false.
func ago(d time.Duration) string {
	if d < 0 {
		return ""
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		return fmt.Sprintf("%dh %02dm", h, m)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// agentTitle is the roster row's primary line: the model when the server
// resolved one (every row shares the agent's own name, so the model is what
// tells rows apart), else the agent, else "shell".
func agentTitle(item wire.AgentItem) string {
	switch {
	case item.Model != "":
		return item.Model
	case item.Agent != "":
		return item.Agent
	default:
		return "shell"
	}
}

// joinNonEmpty joins the non-empty parts with sep, for subtitles built from
// optional facts.
func joinNonEmpty(sep string, parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

const bullet = " · "

// shortFingerprint is the first and last few hex digits of a certificate
// fingerprint, enough to compare against what catway printed by eye.
func shortFingerprint(fp string) string {
	fp = strings.ToLower(strings.ReplaceAll(fp, ":", ""))
	if len(fp) <= 16 {
		return fp
	}
	return fp[:8] + "…" + fp[len(fp)-8:]
}

// notifyGlyph is the leading glyph for a notification kind.
func notifyGlyph(kind string) string {
	switch kind {
	case wire.NotifyKindAttention:
		return "🔴"
	case wire.NotifyKindFinished:
		return "✅"
	default:
		return "ℹ️"
	}
}

// startedAgo turns an RFC3339 timestamp into an age label, "" when it will
// not parse. Record.StartedAt and RunbookRun.StartedAt both carry one.
func startedAgo(rfc3339 string) string {
	if rfc3339 == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return ago(time.Since(t))
}
