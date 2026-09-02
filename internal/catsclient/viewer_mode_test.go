package catsclient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Layer 4 of the four-layer "the phone never resizes the desktop" guard.
//
// The other three are all inside Conn: it builds the Init itself, Send refuses
// Resize, and FollowWorkspace checks the capability. This one reads the
// source, because the failure it guards against is somebody adding a path the
// first three do not cover: a raw socket.Send of a hand-built JSON string,
// say, or a helper that "just needs" to declare a grid for one screen.
//
// It walks app/ and internal/ with go/ast rather than grepping, so it sees
// through an import alias and ignores comments for free. Test files are
// skipped: conn_test.go constructs a Resize in order to assert it is refused.

// wireImportNames returns the identifiers this file refers to the wire
// package by (usually just "wire", but an alias counts too).
func wireImportNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "github.com/rohanthewiz/cats/wire" {
			continue
		}
		if imp.Name != nil {
			names[imp.Name.Name] = true
		} else {
			names["wire"] = true
		}
	}
	return names
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

func TestNoSourceConstructsAResizeOrDeclaresAGrid(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string
	fset := token.NewFileSet()

	for _, sub := range []string{"app", "internal"} {
		dir := filepath.Join(root, sub)
		if _, err := os.Stat(dir); err != nil {
			continue // app/ does not exist until phase 4
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			wireNames := wireImportNames(f)
			rel, _ := filepath.Rel(root, path)
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if sel, ok := lit.Type.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && wireNames[pkg.Name] && sel.Sel.Name == "Resize" {
						offenders = append(offenders, rel+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+": constructs a wire.Resize")
					}
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok || (key.Name != "Cols" && key.Name != "Rows") {
						continue
					}
					if v, ok := kv.Value.(*ast.BasicLit); ok && v.Kind == token.INT && v.Value != "0" {
						offenders = append(offenders, rel+":"+strconv.Itoa(fset.Position(kv.Pos()).Line)+": declares "+key.Name+": "+v.Value)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("A cats client is a viewer. catway takes the session grid from the first init that "+
			"declares one and shares it with every connection, so a phone announcing its own size "+
			"reflows the desktop's panes for everybody.\n%s", strings.Join(offenders, "\n"))
	}
}
