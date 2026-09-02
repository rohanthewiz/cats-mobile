module github.com/rohanthewiz/cats-mobile

go 1.26.1

// cats is the wire contract, and this line IS the pin: it replaces CATS_REV and
// tool/regen.sh from the Dart days. Bump it with
//
//	go get github.com/rohanthewiz/cats@<sha> && go mod tidy
//
// and then diff wire's down-message list against session.go's switch — the
// compiler adds the types, it has no opinion about what the session does with
// one (TestEveryDownTypeHasAnArm makes that mechanical).
require (
	github.com/coder/websocket v1.8.14
	github.com/rohanthewiz/cats v0.0.0-20260902185955-c0a250f03f01
)

// Development only, while the wire package lives on a cats branch rather than
// main. Drop this and pin a real sha before the README claims gate 2 is real.
replace github.com/rohanthewiz/cats => ../cats
