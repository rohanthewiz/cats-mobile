package catsapp

import (
	"strings"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
)

// Deep links: a cats://pair link the OS hands this app because something else
// — the terminal running `catctl pair`, a message, a QR reader — asked for it
// to be opened.
//
//	OS ──HostEvent("deeplink", {url})──▶ core.OnDeepLink ──▶ receiveDeepLink
//	                                                             │ claims it
//	                                                             ▼
//	                                                        pendingLink
//	                                                             │ next render
//	                                                             ▼
//	                                              pairScreen's "Pairing link" field
//
// The scheme is declared by the shells (scripts/lib.sh stamps `cats` onto the
// AndroidManifest and ios/project.yml of the shell copy this app builds
// against), and grmob forwards the URL verbatim — deliberately, because what a
// URL means is the app's business and not the shell's. So this file is where
// cats-mobile decides what it answers to.
//
// # Why the link is filled in and not acted on
//
// Redeeming a pairing grant is not a navigation: it spends a single-use token,
// pins a TLS certificate, and leaves the phone connected to whatever server the
// link named. A deep link can be sent by any app on the device — a custom
// scheme is claimable by anyone and verified by no one — so a link that paired
// on arrival would let another app choose this phone's desk without the person
// holding it ever seeing an address.
//
// Filling the field is the same thing the Paste button does, and it leaves the
// two things that make the decision safe on screen: the address, and a Pair
// button the person has to press. That is the whole of the difference between
// this and auto-pairing, and it is the reason for it.
//
// # Why a paired phone ignores one
//
// The field only exists on the pair screen, which the shell shows until an
// endpoint is stored (root.go). A link arriving afterwards has nowhere to land,
// and silently re-pointing a paired phone at a new desk is exactly the move the
// paragraph above refuses. Re-pairing is a deliberate path: forget the device
// under More, then pair again.

// receiveDeepLink is the subscriber Services.Bind installs. It runs on the
// host-event goroutine, which is why the link is parked for the next render
// pass rather than written into a screen's hook slot from here — the slot
// belongs to the render loop, and this is not it.
func (s *Services) receiveDeepLink(url string) {
	link := strings.TrimSpace(url)
	// Claimed only if it actually parses as a pairing link. A shell forwards
	// every URL it is handed, so anything else — another app's scheme, this
	// scheme with some other path, a string that is not a URL — has to be
	// dropped here. Storing an unparseable one would put "That is not a cats
	// pairing link." on screen for a link the user never pasted.
	if _, ok := catsclient.ParsePairURI(link); !ok {
		return
	}
	if s.Paired() {
		return
	}
	// The raw link is kept, not the parsed grant: the field shows the person
	// the address they are about to trust, and submit re-parses it the same
	// way it parses a pasted one. One parser, one meaning.
	s.linkMu.Lock()
	s.pendingLink = link
	s.linkMu.Unlock()
	// Without this the link sits until something else causes a pass. A cold
	// launch renders right after this arrives and would pick it up anyway; a
	// link arriving at an app already sitting on the pair screen would not.
	if s.requestRender != nil {
		s.requestRender()
	}
}

// takePendingPairLink hands the parked link to the pair screen, once. Draining
// on read is what keeps the screen editable: a link that stayed put would be
// re-applied on every render pass and overwrite whatever the person typed or
// pasted over it.
func (s *Services) takePendingPairLink() (string, bool) {
	s.linkMu.Lock()
	defer s.linkMu.Unlock()
	link := s.pendingLink
	s.pendingLink = ""
	return link, link != ""
}
