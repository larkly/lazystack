package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowedKeyLiterals are the only keys that may be compared as literals:
// esc keeps a fixed meaning in every view and ctrl+c always quits.
var allowedKeyLiterals = map[string]bool{"esc": true, "ctrl+c": true}

// TestNoHardcodedKeyLiterals keeps every action key rebindable: comparing
// msg.String() to a literal (directly, through strings.ToLower, or as a
// switch case) bypasses the keybinding config. Use key.Matches with a
// shared.Keys binding instead.
func TestNoHardcodedKeyLiterals(t *testing.T) {
	// go test runs in the package directory, so these are src/internal/app
	// and src/internal/ui.
	for _, root := range []string{".", filepath.Join("..", "ui")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			for _, v := range keyLiteralViolations(t, path) {
				t.Errorf("%s: compares msg.String() to %q; use key.Matches with a shared.Keys binding", v.pos, v.lit)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

type keyLiteralViolation struct {
	pos string
	lit string
}

func keyLiteralViolations(t *testing.T, path string) []keyLiteralViolation {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []keyLiteralViolation
	report := func(lit *ast.BasicLit) {
		s, err := strconv.Unquote(lit.Value)
		if err != nil || allowedKeyLiterals[s] {
			return
		}
		out = append(out, keyLiteralViolation{fset.Position(lit.Pos()).String(), s})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if n.Op != token.EQL && n.Op != token.NEQ {
				return true
			}
			if lit, ok := stringLit(n.Y); ok && callsMsgString(n.X) {
				report(lit)
			}
			if lit, ok := stringLit(n.X); ok && callsMsgString(n.Y) {
				report(lit)
			}
		case *ast.SwitchStmt:
			if n.Tag == nil || !callsMsgString(n.Tag) {
				return true
			}
			for _, stmt := range n.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					if lit, ok := stringLit(e); ok {
						report(lit)
					}
				}
			}
		}
		return true
	})
	return out
}

func stringLit(e ast.Expr) (*ast.BasicLit, bool) {
	lit, ok := e.(*ast.BasicLit)
	return lit, ok && lit.Kind == token.STRING
}

// callsMsgString reports whether e contains a call to msg.String().
func callsMsgString(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "String" {
			return !found
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "msg" {
			found = true
		}
		return !found
	})
	return found
}
