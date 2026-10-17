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

// TestNoHardcodedKeyLiterals keeps every action key rebindable: comparing a
// key's String() to a literal bypasses the keybinding config. A key string
// is the String() of a key message (a tea.KeyMsg/KeyPressMsg/Key parameter,
// or msg, the conventional binding of the Update type switch), the result
// of a same-file helper that returns one (keyText), or a variable assigned
// from either within the function (s := msg.String()). Any ==/!= or switch
// case comparing a key string with a literal other than "esc" or "ctrl+c"
// fails. Use key.Matches with a shared.Keys binding, or shared.TextKeys for
// filter editing keys. String() calls on other values (strings.Builder)
// are not key strings and are not flagged.
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
				t.Errorf("%s: compares a key string to %q; use key.Matches with a shared.Keys binding", v.pos, v.lit)
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
	helpers := keyStringHelpers(f)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ks := keyScope{keys: keyParams(fn), vars: map[string]bool{}, helpers: helpers}
		ks.keys["msg"] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
				for i, rhs := range as.Rhs {
					if id, ok := as.Lhs[i].(*ast.Ident); ok && ks.isKeyString(rhs) {
						ks.vars[id.Name] = true
					}
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.BinaryExpr:
				if n.Op != token.EQL && n.Op != token.NEQ {
					return true
				}
				if lit, ok := stringLit(n.Y); ok && ks.isKeyString(n.X) {
					report(lit)
				}
				if lit, ok := stringLit(n.X); ok && ks.isKeyString(n.Y) {
					report(lit)
				}
			case *ast.SwitchStmt:
				if n.Tag == nil || !ks.isKeyString(n.Tag) {
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
	}
	return out
}

func stringLit(e ast.Expr) (*ast.BasicLit, bool) {
	lit, ok := e.(*ast.BasicLit)
	return lit, ok && lit.Kind == token.STRING
}

// isKeyType reports whether e is tea.KeyMsg, tea.KeyPressMsg,
// tea.KeyReleaseMsg or tea.Key.
func isKeyType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "tea" {
		return false
	}
	switch sel.Sel.Name {
	case "KeyMsg", "KeyPressMsg", "KeyReleaseMsg", "Key":
		return true
	}
	return false
}

// keyParams returns the names of fn's key-typed parameters.
func keyParams(fn *ast.FuncDecl) map[string]bool {
	keys := map[string]bool{}
	for _, field := range fn.Type.Params.List {
		if isKeyType(field.Type) {
			for _, name := range field.Names {
				keys[name.Name] = true
			}
		}
	}
	return keys
}

// keyStringHelpers returns the file's functions that take a key and return
// its String(), like func keyText(msg tea.KeyMsg) string { return msg.String() }.
func keyStringHelpers(f *ast.File) map[string]bool {
	helpers := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 1 {
			continue
		}
		ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		ks := keyScope{keys: keyParams(fn)}
		if len(ks.keys) > 0 && ks.isKeyString(ret.Results[0]) {
			helpers[fn.Name.Name] = true
		}
	}
	return helpers
}

// keyScope knows which identifiers hold keys or key strings in a function.
type keyScope struct {
	keys    map[string]bool // key values: String() on them is a key string
	vars    map[string]bool // variables assigned from a key string
	helpers map[string]bool // same-file helpers returning a key string
}

// isKeyString reports whether e is, or contains, a key string.
func (ks keyScope) isKeyString(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.Ident:
			found = ks.vars[n.Name]
		case *ast.CallExpr:
			switch fun := n.Fun.(type) {
			case *ast.SelectorExpr:
				if id, ok := fun.X.(*ast.Ident); ok && len(n.Args) == 0 && fun.Sel.Name == "String" && ks.keys[id.Name] {
					found = true
				}
			case *ast.Ident:
				found = ks.helpers[fun.Name]
			}
		}
		return !found
	})
	return found
}
