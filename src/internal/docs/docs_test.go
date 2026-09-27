// Package docs holds consistency tests between the repository documentation
// (README.md, PRD.md, INDEX.md, docs/index.html) and the code it describes.
package docs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// root is the repository root relative to this package directory.
const root = "../../.."

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// section returns the text of a markdown section starting at the given
// heading line, up to the next heading of the same or higher level.
func section(t *testing.T, doc, heading string) string {
	t.Helper()
	i := strings.Index(doc, "\n"+heading+"\n")
	if i < 0 {
		t.Fatalf("heading %q not found", heading)
	}
	level := strings.Index(heading, " ")
	rest := doc[i+len(heading)+2:]
	re := regexp.MustCompile(`(?m)^#{1,` + strconv.Itoa(level) + `} `)
	if loc := re.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
}

// tableRows parses "| key | action |" markdown rows into key -> action.
func tableRows(sec string) map[string]string {
	rows := map[string]string{}
	for _, line := range strings.Split(sec, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 2 || strings.HasPrefix(strings.TrimSpace(cells[0]), "-") {
			continue
		}
		rows[strings.TrimSpace(cells[0])] = strings.TrimSpace(cells[1])
	}
	return rows
}

// TestNoUnsupportedGoInstall: the module lives in src/, so `go install
// github.com/larkly/lazystack/...` cannot resolve and must not be published.
func TestNoUnsupportedGoInstall(t *testing.T) {
	blocks := map[string]*regexp.Regexp{
		"README.md":       regexp.MustCompile("(?s)```[a-z]*\n(.*?)```"),
		"docs/index.html": regexp.MustCompile(`(?s)<pre>(.*?)</pre>`),
	}
	for f, re := range blocks {
		for _, m := range re.FindAllStringSubmatch(read(t, f), -1) {
			if strings.Contains(m[1], "go install") {
				t.Errorf("%s shows a go install command, which cannot resolve the src/ module: %s", f, strings.TrimSpace(m[1]))
			}
		}
	}
	readme := section(t, read(t, "README.md"), "### From source")
	for _, want := range []string{"git clone https://github.com/larkly/lazystack.git", "cd lazystack/src", "make build"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README From source is missing %q", want)
		}
	}
}

// TestBinaryDownloadCommandsFetchAssets: download commands must fetch a
// concrete release asset over HTTPS rather than the release HTML page.
func TestBinaryDownloadCommandsFetchAssets(t *testing.T) {
	curlRe := regexp.MustCompile(`curl [^\n<]*`)
	for _, f := range []string{"README.md", "docs/index.html"} {
		found := false
		for _, cmd := range curlRe.FindAllString(read(t, f), -1) {
			if !strings.Contains(cmd, "larkly/lazystack/releases") {
				continue
			}
			found = true
			if !strings.Contains(cmd, "https://") {
				t.Errorf("%s: download without explicit https: %s", f, cmd)
			}
			if !strings.Contains(cmd, "releases/latest/download/") || !strings.Contains(cmd, "-f") {
				t.Errorf("%s: download does not fetch a release asset with -f: %s", f, cmd)
			}
		}
		if !found {
			t.Errorf("%s has no binary download command", f)
		}
	}
}

// TestCloudsYamlSearchOrderDocumented compares the documented clouds.yaml
// search order with the order the cloud package searches.
func TestCloudsYamlSearchOrderDocumented(t *testing.T) {
	order := []string{"OS_CLIENT_CONFIG_FILE", "XDG_CONFIG_HOME", "/etc/openstack/clouds.yaml", "./clouds.yaml"}
	docs := map[string]string{
		"README.md": section(t, read(t, "README.md"), "### clouds.yaml"),
		"PRD.md":    section(t, read(t, "PRD.md"), "#### Cloud Connection"),
	}
	for f, sec := range docs {
		last := -1
		for _, tok := range order {
			i := strings.Index(sec, tok)
			if i < 0 {
				t.Errorf("%s clouds.yaml search order does not mention %s", f, tok)
				continue
			}
			if i < last {
				t.Errorf("%s lists %s out of order (want %v)", f, tok, order)
			}
			last = i
		}
		if !strings.Contains(sec, "no fallback") {
			t.Errorf("%s does not say an explicit OS_CLIENT_CONFIG_FILE has no fallback", f)
		}
		if !strings.Contains(sec, "no clouds") {
			t.Errorf("%s does not describe how files without clouds are handled", f)
		}
	}
}

// flagNames extracts the flag names registered in cmd/lazystack/main.go.
func flagNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(root, "src/cmd/lazystack/main.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "flag" {
			return true
		}
		switch sel.Sel.Name {
		case "Bool", "String", "Int", "Duration":
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				name, _ := strconv.Unquote(lit.Value)
				names = append(names, name)
			}
		}
		return true
	})
	if len(names) == 0 {
		t.Fatal("no flags found in main.go")
	}
	return names
}

func TestEveryFlagDocumented(t *testing.T) {
	flags := flagNames(t)
	docs := map[string]string{
		"README.md": section(t, read(t, "README.md"), "### CLI flags"),
		"PRD.md":    section(t, read(t, "PRD.md"), "### CLI Flags"),
	}
	for f, sec := range docs {
		rows := tableRows(sec)
		documented := map[string]bool{}
		for k := range rows {
			if m := regexp.MustCompile("^`--([a-z-]+)").FindStringSubmatch(k); m != nil {
				documented[m[1]] = true
			}
		}
		for _, name := range flags {
			if !documented[name] {
				t.Errorf("%s CLI flag table is missing --%s", f, name)
			}
		}
		for name := range documented {
			found := false
			for _, n := range flags {
				found = found || n == name
			}
			if !found {
				t.Errorf("%s documents --%s, which main.go does not define", f, name)
			}
		}
	}
}

func TestAppConfigDocumented(t *testing.T) {
	readme := read(t, "README.md")
	for _, want := range []string{"~/.config/lazystack/config.yaml", "Ctrl+K", "~/.cache/lazystack/debug.log", "LAZYSTACK_DEBUG_LOG"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README does not mention %s", want)
		}
	}
}

// TestPRDImageBindings: image actions are routed as d = deactivate,
// ctrl+g = download (properties pane), enter = edit (info pane).
func TestPRDImageBindings(t *testing.T) {
	prd := read(t, "PRD.md")
	rows := tableRows(section(t, section(t, prd, "### Keybindings"), "#### Images"))
	if a := rows["`d`"]; !strings.Contains(strings.ToLower(a), "deactivate") {
		t.Errorf("PRD Images `d` = %q, want deactivate/reactivate", a)
	}
	if a := rows["`Ctrl+G`"]; !strings.Contains(strings.ToLower(a), "download") {
		t.Errorf("PRD Images `Ctrl+G` = %q, want download", a)
	}
	if a, ok := rows["`e`"]; ok {
		t.Errorf("PRD Images documents unbound key `e` (%q)", a)
	}
	mgmt := section(t, prd, "#### Image Management")
	if strings.Contains(mgmt, "**Download** (`d`)") || strings.Contains(mgmt, "**Edit** (`e`)") {
		t.Error("PRD Image Management still lists d = download / e = edit")
	}
}

// TestNoReservedKeysDocumented: Ctrl+A (Screen) and Ctrl+B (tmux) are never
// lazystack bindings.
func TestNoReservedKeysDocumented(t *testing.T) {
	re := regexp.MustCompile("(?i)^\\|\\s*`ctrl\\+[ab]`")
	for _, f := range []string{"README.md", "PRD.md"} {
		for _, line := range strings.Split(read(t, f), "\n") {
			if re.MatchString(strings.TrimSpace(line)) {
				t.Errorf("%s documents a reserved key: %s", f, strings.TrimSpace(line))
			}
		}
	}
}

// TestIndexMakeTargetsExist: every Makefile target INDEX.md names must exist.
func TestIndexMakeTargetsExist(t *testing.T) {
	index := read(t, "INDEX.md")
	targetRe := regexp.MustCompile(`^([a-z]+):`)
	for _, mk := range []string{"Makefile", "src/Makefile"} {
		defined := map[string]bool{}
		for _, line := range strings.Split(read(t, mk), "\n") {
			if m := targetRe.FindStringSubmatch(line); m != nil {
				defined[m[1]] = true
			}
		}
		var row string
		for _, line := range strings.Split(index, "\n") {
			if strings.HasPrefix(line, "| [`"+mk+"`]") {
				row = line
			}
		}
		if row == "" {
			t.Errorf("INDEX.md has no row for %s", mk)
			continue
		}
		cells := strings.Split(row, "|")
		for _, m := range regexp.MustCompile("`([a-z]+)`").FindAllStringSubmatch(cells[2], -1) {
			if !defined[m[1]] {
				t.Errorf("INDEX.md lists %s target %q, which is not defined", mk, m[1])
			}
		}
	}
}
