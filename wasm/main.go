//go:build js && wasm

// Command wasm is the browser host for the cats app: the same Go code the
// native shells run, mounted into the DOM through grmob's WASM runtime.
//
// It exists so a change can be seen in seconds without a simulator or a
// device:
//
//	scripts/wasm.sh          # build + serve on :8080
//
// This file is host wiring only: the JS bindings, the render manager, and
// the event bridge. Everything the app does lives in the catsapp package,
// whose init registers the root view. It is a near-copy of church_mobile's
// host (itself a near-copy of grmob's), deliberately rather than an import:
// that file is `package main` with the mounted app chosen by a dot-import, so
// it cannot be reused by another module.
//
// # What the browser target can and cannot do
//
// The WebSocket upgrade cannot carry a bearer, so the pair screen's POST
// /login sets a cookie and the socket rides that (catsclient/dial_js.go).
// The cookie is same-site strict, which means this page must be served from
// the catway's own origin, or the browser must be pointed at a catway on
// localhost with the page on localhost too. TLS is the browser's: a
// self-signed catway has to be trusted in the browser first.
package main

import (
	"encoding/json"
	"log"
	"syscall/js"

	catsapp "github.com/rohanthewiz/cats-mobile/app"
	"github.com/rohanthewiz/grmob/core"
	"github.com/rohanthewiz/grmob/render"
)

// The context is built here rather than reusing the one catsapp's init
// registered with the mobile bridge: that one belongs to the bridge, and the
// browser host drives its own render manager.
var ctx = core.NewContext()

var manager *render.Manager

func renderInitial(this js.Value, args []js.Value) any {
	if manager != nil {
		manager.Close()
	}
	manager = render.New(ctx, catsapp.App)
	// Push channel: when the page defines GrMobApplyPatches, async state
	// changes (a frame arriving on the socket) are pushed to it directly
	// instead of waiting for the IsDirty poll, which rides
	// requestAnimationFrame and is fully suspended in a hidden tab.
	if js.Global().Get("GrMobApplyPatches").Type() == js.TypeFunction {
		manager.SetListener(jsPatchListener{})
	}
	return js.ValueOf(manager.RenderInitial())
}

func renderAgain(this js.Value, args []js.Value) any {
	return js.ValueOf(manager.RenderAgain())
}

func isDirty(this js.Value, args []js.Value) any {
	return js.ValueOf(ctx.IsDirty())
}

// receiveEvent is the one entry point for every user interaction. The
// runtime calls it as ReceiveEvent(id, JSON.stringify(payload)).
func receiveEvent(this js.Value, args []js.Value) any {
	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		log.Printf("cats wasm: dropping an event with %d arguments", len(args))
		return nil
	}
	id := args[0].String()

	var payload map[string]any
	if err := json.Unmarshal([]byte(args[1].String()), &payload); err != nil {
		log.Printf("cats wasm: dropping malformed payload for %s: %v", id, err)
		return nil
	}

	// Guarded because this host dispatches directly rather than through
	// render.Manager's Dispatch* (which carry the same guard): a panic
	// escaping a handler here would abort the Go runtime and the page's app
	// with it.
	if rerr := core.Guard(func() {
		ctx.ReceiveEventPayload(map[string]any{
			"callback": id,
			"value":    payload["value"],
		})
	}); rerr != nil {
		log.Printf("cats wasm: recovered panic in handler %s: %v\n%s", id, rerr.Value, rerr.Stack)
	}
	return nil
}

// jsPatchListener forwards pushed patches to the page. ApplyPatches runs on
// the pump goroutine, which on js/wasm is scheduled cooperatively on the
// single JS thread, so calling into JS here needs no marshalling.
type jsPatchListener struct{}

func (jsPatchListener) ApplyPatches(patches string) {
	js.Global().Call("GrMobApplyPatches", patches)
}

// registerSystemEvents wires core's system-event channel to the page: this
// is what makes the app's toasts visible in the browser preview.
func registerSystemEvents() {
	if js.Global().Get("GrMobSystemEvent").Type() != js.TypeFunction {
		return
	}
	core.SetSystemEventHandler(func(name string, data map[string]any) {
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("cats wasm: dropping system event %q: %v", name, err)
			return
		}
		js.Global().Call("GrMobSystemEvent", name, string(payload))
	})
}

// hostEvent is the page's entry point for host→app traffic that answers no
// registered callback. The cats app has none yet; the entry point is kept so
// the page contract matches grmob's runtime.
func hostEvent(this js.Value, args []js.Value) any {
	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return nil
	}
	name := args[0].String()
	var payload map[string]any
	if err := json.Unmarshal([]byte(args[1].String()), &payload); err != nil {
		return nil
	}
	if rerr := core.Guard(func() { core.ReceiveHostEvent(name, payload) }); rerr != nil {
		log.Printf("cats wasm: recovered panic in host event %s: %v\n%s", name, rerr.Value, rerr.Stack)
	}
	return nil
}

func registerCallbacks() {
	js.Global().Set("GrMobWASM", map[string]any{
		"RenderInitial": js.FuncOf(renderInitial),
		"RenderAgain":   js.FuncOf(renderAgain),
		"ReceiveEvent":  js.FuncOf(receiveEvent),
		"IsDirty":       js.FuncOf(isDirty),
		"HostEvent":     js.FuncOf(hostEvent),
	})
}

func main() {
	registerCallbacks()
	registerSystemEvents()
	println("Cats app (WASM) ready.")
	// Block forever: the JS callbacks above are the program from here on.
	select {}
}
