package catsapp

import (
	"slices"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/comps"
	"github.com/rohanthewiz/grmob/core"
	"github.com/rohanthewiz/grmob/hooks"
	"github.com/rohanthewiz/grmob/permission"
)

// Attention: telling the person holding the phone that an agent needs them.
//
// The roster already sorts a blocked agent to the top, but only for someone
// looking at it. This file turns the change — an agent that was not blocked
// in the last rollup and is in this one — into the two things a phone can do
// about it without being looked at:
//
//	agents rollup ──▶ Connection.onMessage ──▶ blockedWatch.observe
//	                                              │ became blocked / cleared
//	                                              ▼
//	                                           announce (outside the lock)
//	                                    ┌─────────┼──────────────────────┐
//	                           core.Haptic   core.PostNotification   core.CancelNotification
//	                            (always)     (only when not active)   (when it unblocks)
//
//	notification tap ──▶ core.OnNotificationTap ──▶ Services.openFromNotification
//	                                                   └──▶ core.Push(paneScreen)
//
// # Why the buzz always and the banner only in the background
//
// A phone in the hand is looking at the app: the roster has already moved the
// agent to "Needs you", and a banner over it — which iOS draws for a
// foreground post — would say the same thing on top of itself. The buzz is
// what reaches a phone lying on the desk with the screen on. A phone in a
// pocket or on another app gets the banner, and the banner is taken down
// again when the agent unblocks, so the notification list never holds a
// request nobody can answer any more.
//
// # Why the first rollup after a pairing is silent
//
// The first rollup describes the desk as it already is; announcing every
// agent that happened to be blocked when the app opened would buzz for news
// that is on screen. So the first observation only seeds the set. Connect
// resets the watch (a different desk is a different set), but a socket drop
// does not: the rollup after a reconnect is compared with the one before the
// drop, so an agent that blocked during the gap is announced and one that was
// already blocked is not.
//
// # Identity
//
// An agent is its pane's public handle (item.Pub, "w1:p3"). That is also the
// notification id, so a second block of the same pane replaces its banner
// rather than stacking one, and a tap names the pane to open.

// blockedWatch remembers which agents were blocked in the last rollup. It has
// no lock of its own: Connection holds it under mu, beside the session it
// shadows.
type blockedWatch struct {
	seeded  bool
	blocked map[string]wire.AgentItem
}

// observe folds one rollup's agents into the watch and reports what changed:
// the agents that are blocked now and were not before, and the handles of
// agents that were blocked and are not any more (finished, answered, or gone
// from the rollup entirely). Both lists are sorted by handle, so a rollup
// that blocks two agents announces them in a stable order.
//
// The first call after a reset reports nothing; see the file comment.
func (w *blockedWatch) observe(items []wire.AgentItem) (became []wire.AgentItem, cleared []string) {
	next := make(map[string]wire.AgentItem, len(items))
	for _, item := range items {
		if item.State == wire.AgentBlocked && item.Pub != "" {
			next[item.Pub] = item
		}
	}
	if w.seeded {
		for pub, item := range next {
			if _, was := w.blocked[pub]; !was {
				became = append(became, item)
			}
		}
		for pub := range w.blocked {
			if _, still := next[pub]; !still {
				cleared = append(cleared, pub)
			}
		}
		slices.SortFunc(became, func(a, b wire.AgentItem) int {
			if a.Pub < b.Pub {
				return -1
			}
			if a.Pub > b.Pub {
				return 1
			}
			return 0
		})
		slices.Sort(cleared)
	}
	w.blocked = next
	w.seeded = true
	return became, cleared
}

// reset forgets everything, so the next rollup seeds again. Connect calls it
// for a new pairing or endpoint.
func (w *blockedWatch) reset() {
	w.seeded = false
	w.blocked = nil
}

// announce plays what observe found. It sends system events, which reach a
// native shell synchronously, so Connection calls it after releasing its
// lock rather than while a render pass might be waiting on it.
//
// One buzz per rollup however many agents blocked in it: three pulses for
// three agents arriving together would read as an error, not as "look".
func announce(became []wire.AgentItem, cleared []string) {
	for _, pub := range cleared {
		core.CancelNotification(pub)
	}
	if len(became) == 0 {
		return
	}
	core.Haptic(core.HapticWarning)
	if core.CurrentLifecycle() == core.LifecycleActive {
		return
	}
	for _, item := range became {
		core.PostNotification(core.LocalNotification{
			ID:    item.Pub,
			Title: agentTitle(item) + " needs you",
			Body:  joinNonEmpty(bullet, item.Pub, item.Workspace),
		})
	}
}

// SetNavigator records the navigator's root context. The shell calls it on
// every pass; only the latest matters, and the context is stable for the life
// of the Navigator in practice, so this is a cheap overwrite.
func (s *Services) SetNavigator(ctx *core.Context) {
	s.navMu.Lock()
	s.navCtx = ctx
	s.navMu.Unlock()
}

// openFromNotification opens the pane a tapped notification named.
//
// The stack is cleared first so back from the pane lands on the roster, which
// is where a person who came in from a notification expects "back" to go —
// not on whatever screen the app happened to be showing before it was
// backgrounded. A handle the session no longer knows (the pane closed while
// the banner sat in the list) opens nothing: the app has already come
// forward, and the roster is the honest place to land.
//
// It runs on the host-event goroutine, which on the natives is inside the
// render manager's dispatch, the same place a tap handler runs, so pushing a
// route from here is what agentRow's onTap does.
func (s *Services) openFromNotification(pub string) {
	var pane uint32
	found := false
	s.Conn.Read(func(sess *catsclient.Session) {
		for _, item := range sess.Agents {
			if item.Pub == pub {
				pane, found = item.Pane, true
				return
			}
		}
	})
	s.navMu.Lock()
	ctx := s.navCtx
	s.navMu.Unlock()
	if !found || ctx == nil {
		return
	}
	core.PopToRoot(ctx)
	core.Push(ctx, paneScreen(pane, pub))
}

// notificationsRow is More's control for the one permission this app asks
// for. It holds a hook (hooks.UsePermission, which checks on mount), so
// moreScreen calls it on every pass in the same place.
//
// The ask lives here, behind a button with the reason next to it, rather than
// at the first blocked agent: a prompt that appears because something
// happened at the desk while the user was looking elsewhere is a prompt that
// gets refused, and iOS never asks twice.
func notificationsRow(ctx *core.Context) core.View {
	status := hooks.UsePermission(ctx, permission.Notifications)
	var subtitle string
	var action core.View
	switch status {
	case permission.Granted:
		subtitle = "On · a blocked agent notifies you while the app is in the background"
	case permission.Prompt:
		subtitle = "Off · allow them to hear about a blocked agent while the app is in the background"
		action = comps.Button{Label: "Allow", Emphasis: comps.EmphasisGhost,
			OnTap: func() { permission.Request(permission.Notifications) },
			Style: []core.StyleProp{core.FontSize(13)}}
	case permission.Denied:
		subtitle = "Turned off in this phone's settings"
	case permission.Unavailable:
		subtitle = "Not available here"
	default:
		// permission.Unknown: the check has been sent and not answered yet.
		subtitle = "Checking…"
	}
	return contentRow(ctx, "🔔", "Notifications", subtitle, nil, action)
}
