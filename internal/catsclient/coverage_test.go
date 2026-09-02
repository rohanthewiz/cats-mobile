package catsclient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rohanthewiz/cats/wire"
)

// The lesson the lockstep session closed on: the compiler adds the types; it
// has no opinion about what the session does with one. After every
// `go get cats@<sha>` a new down-message compiles fine and is silently dropped
// by Session.Apply. This test enumerates wire's down-message decoder and
// asserts each type has an arm in Apply or is on the explicit ignore list.
//
// It reads the decoder rather than a hand-kept list of Msg* constants because
// the decoder is what actually decides whether a message reaches Apply: a
// constant with no decode arm never does.

// wireDir is where the pinned cats/wire package lives, per the Go toolchain
// (which honours the replace directive during development).
func wireDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}", "github.com/rohanthewiz/cats/wire").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// decodedDownTypes is every Go type DecodeDown can return, read off its
// `case MsgX: return decodeAs[X](data)` arms.
func decodedDownTypes(t *testing.T) map[string]wire.Type {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(wireDir(t), "proto.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]wire.Type{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
						consts[n.Name] = wire.Type(strings.Trim(lit.Value, `"`))
					}
				}
			}
		}
	}

	types := map[string]wire.Type{}
	var decodeDown *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "DecodeDown" {
			decodeDown = fd
		}
	}
	if decodeDown == nil {
		t.Fatal("wire/proto.go has no DecodeDown")
	}
	ast.Inspect(decodeDown, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok || len(cc.List) != 1 {
			return true
		}
		msgConst, ok := cc.List[0].(*ast.Ident)
		if !ok {
			return true
		}
		// return decodeAs[X](data)
		for _, stmt := range cc.Body {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			call, ok := ret.Results[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			idx, ok := call.Fun.(*ast.IndexExpr)
			if !ok {
				continue
			}
			if typ, ok := idx.Index.(*ast.Ident); ok {
				types[typ.Name] = consts[msgConst.Name]
			}
		}
		return true
	})
	if len(types) < 20 {
		t.Fatalf("read only %d decode arms from DecodeDown; the parser is probably out of step with proto.go", len(types))
	}
	return types
}

// applyArms is every `case *wire.X:` type in Session.Apply.
func applyArms(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	arms := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "Apply" {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				star, ok := e.(*ast.StarExpr)
				if !ok {
					continue
				}
				if sel, ok := star.X.(*ast.SelectorExpr); ok {
					arms[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	return arms
}

func TestEveryDownTypeHasAnArm(t *testing.T) {
	decoded := decodedDownTypes(t)
	arms := applyArms(t)
	for typ, msg := range decoded {
		_, ignored := ignoredDownTypes[msg]
		switch {
		case arms[typ] && ignored:
			t.Errorf("%s (%q) is both folded by Apply and on ignoredDownTypes; pick one", typ, msg)
		case !arms[typ] && !ignored:
			t.Errorf("wire.%s (%q) reaches Session.Apply and is silently dropped: add an arm, or add it to ignoredDownTypes with a reason", typ, msg)
		}
	}
	// And the ignore list must not name something that no longer exists,
	// which is how a stale reason outlives the type it explained.
	known := map[wire.Type]bool{}
	for _, msg := range decoded {
		known[msg] = true
	}
	for msg := range ignoredDownTypes {
		if !known[msg] {
			t.Errorf("ignoredDownTypes names %q, which DecodeDown no longer produces", msg)
		}
	}
}
