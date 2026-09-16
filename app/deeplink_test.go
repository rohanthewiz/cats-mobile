package catsapp

import (
	"testing"

	"github.com/rohanthewiz/grmob/core"
)

// Deep links (deeplink.go). Delivered the way a native shell delivers one —
// core.ReceiveHostEvent with the URL verbatim and unparsed — so these tests
// exercise the exact seam MainActivity.reportDeepLink and GrMobApp's
// .onOpenURL push through.

// deepLink is the shells' inbound path: a URL, not a parsed grant.
func deepLink(url string) {
	core.ReceiveHostEvent("deeplink", map[string]any{"url": url})
}

// testPairLink is shaped like `catctl pair`'s output: an encoded URL, a
// single-use token, and the certificate fingerprint that becomes the pin.
const testPairLink = "cats://pair?u=https%3A%2F%2Fdesk.test%3A8443&t=tok123&f=aa11bb22"

// emptyPairField is the pairing link input as it renders untouched. Matched as
// one fragment so the assertion cannot pass on some other empty input.
const emptyPairField = `value="" placeholder="cats://pair?u=…"`

func TestDeepLinkFillsThePairingLinkField(t *testing.T) {
	h := newHarness(t, false)
	if !shows(h.html(), emptyPairField) {
		t.Fatalf("the pairing field should start empty:\n%s", h.html())
	}

	deepLink(testPairLink)
	h.waitFor(`value="` + testPairLink + `"`)
}

// A shell forwards every URL it is handed, so the app is the thing that
// decides what it answers to. Anything it does not claim must leave the screen
// alone — in particular it must not land in the field and render as "That is
// not a cats pairing link." for a link the user never pasted.
func TestDeepLinkIgnoresWhatItDoesNotClaim(t *testing.T) {
	h := newHarness(t, false)

	for _, url := range []string{
		"grmob://lesson/4.12",          // the shell's own scheme
		"cats://settings",              // this scheme, another feature
		"cats://pair",                  // no grant in it
		"cats://pair?t=tok",            // a token but no address
		"https://desk.test/pair?t=tok", // another scheme entirely
		"not a url at all",
		"",
	} {
		deepLink(url)
		h.settle()
		if !shows(h.html(), emptyPairField) {
			t.Errorf("%q should have been dropped, but the field changed:\n%s", url, h.html())
		}
	}
}

// A paired phone has no pair screen for a link to land on, and silently
// re-pointing it at another desk is the move deeplink.go refuses.
func TestDeepLinkIsIgnoredOncePaired(t *testing.T) {
	newHarness(t, true)

	// ReceiveHostEvent dispatches synchronously, so the handler has run by the
	// time this returns; nothing to wait for.
	deepLink(testPairLink)

	if link, ok := services.takePendingPairLink(); ok {
		t.Fatalf("a paired phone parked a deep link: %q", link)
	}
}

// The link is taken once. A link that stayed parked would be re-applied on
// every render pass, so anything typed or pasted over it would be overwritten
// on the next repaint — the field would be uneditable for as long as the app
// stayed on that screen.
func TestDeepLinkIsAppliedOnceAndNotReappliedOverAnEdit(t *testing.T) {
	h := newHarness(t, false)

	deepLink(testPairLink)
	h.waitFor(`value="` + testPairLink + `"`)

	h.typeInto("cats://pair?u=…", "typed over it")
	h.settle()
	if !shows(h.html(), `value="typed over it"`) {
		t.Fatalf("a drained link was re-applied over a later edit:\n%s", h.html())
	}
}
