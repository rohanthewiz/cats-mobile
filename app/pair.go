package catsapp

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/grmob/comps"
	"github.com/rohanthewiz/grmob/core"
)

// pairForm is the pair screen's local state. One struct in one hook slot
// rather than a slot per field: the fields change together (a submit clears
// the error and sets busy), and a single Set is one render.
type pairForm struct {
	// URI is the cats://pair?u=&t=&f= link from `catctl pair`'s QR.
	URI string
	// Host, Port and Password are the by-hand path for an endpoint with no
	// QR to scan. No pin arrives this way; the first connect pins whatever
	// certificate it sees (trust on first use), which the screen says.
	Host     string
	Port     string
	Password string
	Busy     bool
	Err      string
	// Pinned is the fingerprint the just-completed pairing recorded, shown
	// so the user can compare it against what catway printed.
	Pinned string
}

// pairScreen is where the phone meets a catway: paste the pairing link (or
// type an address and the shared password), redeem it for a session, pin the
// certificate, and connect.
//
// # Why redeem here and not on first dial
//
// The grant in the QR is single-use and worth minutes. Spending it at POST
// /login yields the session the device actually keeps, and doing that as a
// distinct step means a failure has one clear meaning ("the code ran out;
// scan a fresh one") instead of surfacing as a reconnect loop against a
// credential that will never work.
//
// # The certificate
//
// The QR carries the certificate's fingerprint. Login dials with it as the
// stored pin, so the very first contact refuses a server whose certificate
// is not the one the QR named. A hand-typed endpoint has no pin; that is the
// one honest trust-on-first-use path (catsclient.DecideCert), and the
// fingerprint it records is shown afterwards for the user to check by eye.
//
// Camera scanning is deferred: grmob's CameraView delivers frames, and a
// pure-Go QR decoder (makiuchi-d/gozxing) is on the roadmap. Pasting the
// link works today on every target.
func pairScreen(ctx *core.Context) core.View {
	services := Get()
	theme := ctx.Theme()

	slot := core.NewState(ctx, &pairForm{Port: "8443"})
	form := slot.Get()
	// update is copy-on-write: it never mutates the struct a render pass may
	// be reading on another goroutine (the login completes off the event
	// thread). It reads the latest value, copies, mutates the copy, stores it.
	update := func(mutate func(*pairForm)) {
		next := *slot.Get()
		mutate(&next)
		slot.Set(&next)
	}

	// A link the OS handed the app arrives here rather than through a tap, and
	// lands in the field exactly as the Paste button's text does — replacing,
	// never appending, for the reason spelled out at that button. It is taken
	// during the render that follows its arrival because the hook slot belongs
	// to the render loop and the host-event goroutine that received it does
	// not; see deeplink.go, which also says why a link is not acted on.
	//
	// Not a conditional hook: takePendingPairLink reads a mutex-guarded field,
	// and the hook above it has already run unconditionally.
	if uri, ok := services.takePendingPairLink(); ok {
		update(func(f *pairForm) { f.URI = uri; f.Err = "" })
		form = slot.Get()
	}

	// submit is shared by both paths. It resolves the grant (from the URI
	// or the typed fields), then redeems it on its own goroutine: the round
	// trip must not block the event thread.
	submit := func() {
		if form.Busy {
			return
		}
		grant, err := grantFromForm(form)
		if err != nil {
			update(func(f *pairForm) { f.Err = err.Error() })
			return
		}
		update(func(f *pairForm) { f.Busy = true; f.Err = ""; f.Pinned = "" })

		go func() {
			pinned := grant.Endpoint.PinnedSHA256
			session, err := catsclient.Login(context.Background(), grant.Endpoint, grant.Token,
				catsclient.LoginOptions{
					StoredPin: pinned,
					OnCertDecision: func(d catsclient.CertDecision) {
						if d.Verdict == catsclient.VerdictFirstUse {
							pinned = d.Fingerprint
						}
					},
				})
			if err != nil {
				update(func(f *pairForm) { f.Busy = false; f.Err = pairErrorMessage(err) })
				return
			}
			endpoint := grant.Endpoint.WithPin(pinned)
			// Persist before activating: a store failure must surface here
			// as an error, not as a session the next launch cannot find.
			if err := services.Store.WritePin(endpoint.ID, pinned); err != nil {
				update(func(f *pairForm) { f.Busy = false; f.Err = "Could not save the pairing: " + err.Error() })
				return
			}
			if session != "" {
				if err := services.Store.WriteToken(endpoint.ID, session); err != nil {
					update(func(f *pairForm) { f.Busy = false; f.Err = "Could not save the pairing: " + err.Error() })
					return
				}
			} else {
				// WASM: the credential is the browser's cookie. Record that
				// the endpoint is paired with a marker the dial ignores.
				_ = services.Store.WriteToken(endpoint.ID, cookieToken)
			}
			if err := services.Store.AddEndpoint(endpoint); err != nil {
				update(func(f *pairForm) { f.Busy = false; f.Err = "Could not save the pairing: " + err.Error() })
				return
			}
			update(func(f *pairForm) {
				f.Busy = false
				f.Pinned = pinned
				f.URI, f.Password = "", ""
			})
			// The shell re-reads Paired() on the pass Connect triggers.
			services.Conn.Connect(endpoint)
		}()
	}

	return comps.Screen{
		Fill:          true,
		Scroll:        true,
		KeyboardAware: true,
		Style: []core.StyleProp{
			core.BackgroundColor(theme.Colors.Background),
		},
		Children: []core.View{
			screenHeader(ctx, "Pair with a desk"),
			core.Column(
				core.Gap(12),
				core.Padding(20),
				core.MaxWidth("480px"),

				core.Text("Run `catctl pair` at the desk and paste the link it shows.",
					core.TextColor(theme.Colors.TextPrimary)),
				comps.FormField{
					Label: "Pairing link",
					Input: core.Input(form.URI, "cats://pair?u=…", func(v string) {
						update(func(f *pairForm) { f.URI = v; f.Err = "" })
					}),
				},
				// The link is ~130 characters of random token and hex
				// fingerprint, and it is born in another app — the terminal
				// running `catctl pair`, or whatever carried it to the phone.
				// Typing it is not a real path; pasting it is, and without a
				// button the only paste is the keyboard's own, which is a
				// long-press away and absent on a hardware keyboard.
				//
				// It replaces the field rather than appending to it, which is
				// where this differs from the composer's Paste: a reply is a
				// sentence a paste adds to, but a pairing link is a whole
				// value. Appending onto a stale link could only ever build a
				// string that parses as nothing.
				//
				// Trimmed because a link copied out of a terminal usually
				// brings a trailing newline, and ParsePairURI would reject it.
				core.Row(
					core.Justify(core.JustifyEnd),
					core.Gap(8),
					core.PaddingHorizontal(0),
					core.PaddingVertical(0),
					comps.Button{
						Label:    "Paste link",
						Emphasis: comps.EmphasisGhost,
						Disabled: form.Busy,
						OnTap: func() {
							core.ReadClipboard(func(text string, ok bool) {
								switch {
								case !ok:
									toast("Could not read the clipboard.")
								case strings.TrimSpace(text) == "":
									toast("The clipboard has no text.")
								default:
									update(func(f *pairForm) {
										f.URI = strings.TrimSpace(text)
										f.Err = ""
									})
								}
							})
						},
						AccessibilityHint:  "Fills the pairing link field from the clipboard",
						AccessibilityLabel: "Paste the pairing link",
					},
				),

				mutedText(ctx, "Or enter the address and shared password by hand. "+
					"The first connection will trust whatever certificate it sees; "+
					"check the fingerprint against the desk afterwards."),
				comps.FormField{
					Label: "Host",
					Input: core.Input(form.Host, "192.168.1.20 or desk.tailnet.ts.net", func(v string) {
						update(func(f *pairForm) { f.Host = v; f.Err = "" })
					}),
				},
				comps.FormField{
					Label: "Port",
					Input: core.Input(form.Port, "8443", func(v string) {
						update(func(f *pairForm) { f.Port = v; f.Err = "" })
					}),
				},
				comps.FormField{
					Label: "Password",
					Input: core.InputPassword(form.Password, "shared password", func(v string) {
						update(func(f *pairForm) { f.Password = v; f.Err = "" })
					}),
				},

				core.If(form.Err != "",
					core.Text(form.Err, core.TextColor(theme.Colors.Error))),
				core.If(form.Pinned != "",
					mutedText(ctx, "Pinned certificate "+shortFingerprint(form.Pinned))),

				comps.Button{
					Label:     pairLabel(form),
					FullWidth: true,
					Disabled:  form.Busy,
					OnTap:     submit,
				},
			),
		},
	}
}

// cookieToken is the token the store holds for an endpoint whose credential
// lives in the browser's cookie jar (the WASM target). The dial ignores it;
// its only job is to make ReadToken report ok so the app knows it paired.
const cookieToken = "cookie"

func pairLabel(form *pairForm) string {
	if form.Busy {
		return "Pairing…"
	}
	return "Pair"
}

// grantFromForm resolves what the user gave into a PairGrant: the link if
// there is one, else the typed fields.
func grantFromForm(form *pairForm) (catsclient.PairGrant, error) {
	if uri := strings.TrimSpace(form.URI); uri != "" {
		grant, ok := catsclient.ParsePairURI(uri)
		if !ok {
			return catsclient.PairGrant{}, errors.New("That is not a cats pairing link.")
		}
		return grant, nil
	}
	host := strings.TrimSpace(form.Host)
	if host == "" {
		return catsclient.PairGrant{}, errors.New("Paste a pairing link or enter a host.")
	}
	port, err := strconv.Atoi(strings.TrimSpace(form.Port))
	if err != nil || port <= 0 || port > 65535 {
		return catsclient.PairGrant{}, errors.New("Enter a port between 1 and 65535.")
	}
	if form.Password == "" {
		return catsclient.PairGrant{}, errors.New("Enter the shared password.")
	}
	return catsclient.PairGrant{
		Endpoint: catsclient.Endpoint{
			ID:   host + ":" + strconv.Itoa(port),
			Host: host,
			Port: port,
			TLS:  true,
			Kind: catsclient.KindDirect,
		},
		Token: form.Password,
	}, nil
}

// pairErrorMessage words a redemption failure for the screen. The two
// rejections need different words: a refused credential wants a fresh QR,
// a changed certificate wants alarm, and everything else is "not reachable".
func pairErrorMessage(err error) string {
	switch {
	case errors.Is(err, catsclient.ErrUnauthorized):
		return "The desk refused that code. Pairing links expire in minutes and work once; run `catctl pair` again."
	case errors.Is(err, catsclient.ErrCertMismatch):
		return "The server's certificate is not the one in the link. Do not proceed unless you regenerated it; scan a fresh link."
	default:
		return "Could not reach the desk: " + err.Error()
	}
}
