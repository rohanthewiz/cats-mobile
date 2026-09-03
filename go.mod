module github.com/rohanthewiz/cats-mobile

go 1.26.1

// cats is the wire contract, and this line IS the pin (there is no CATS_REV
// or generated mirror any more; the phone imports `wire` directly). Bump it with
//
//	go get github.com/rohanthewiz/cats@<sha> && go mod tidy
//
// and then diff wire's down-message list against session.go's switch — the
// compiler adds the types, it has no opinion about what the session does with
// one (TestEveryDownTypeHasAnArm makes that mechanical).
require (
	github.com/coder/websocket v1.8.14
	github.com/rohanthewiz/cats v0.2.3-0.20260903033745-d58ce46e8ac7
)

require (
	github.com/rohanthewiz/bytdb v0.11.0
	github.com/rohanthewiz/grmob v0.2.4
)

require (
	github.com/rohanthewiz/btypedb v0.7.0 // indirect
	github.com/rohanthewiz/element v0.7.0 // indirect
	github.com/rohanthewiz/serr v1.4.0 // indirect
	github.com/tidwall/btype v0.3.0 // indirect
	golang.org/x/mobile v0.0.0-20251021151156-188f512ec823 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/tools v0.38.0 // indirect
)

// gobind and gomobile are not imported by any Go file; the tool block holds
// them so `go mod tidy` keeps x/mobile, pinned to the version grmob's own
// tool block names. scripts/build-android.sh and build-ios.sh bind from this
// module (gobind only sees the module it runs in), so the pin has to live
// here, not only in grmob.
tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
