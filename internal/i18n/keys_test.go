package i18n

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// sourceDirs are the front-end packages whose string literals must be present
// in the English catalog.
var sourceDirs = []string{"../ui", "../app"}

// referencedKeys collects the message IDs passed directly to i18n.T / i18n.Tf.
func referencedKeys(t *testing.T) (map[string]bool, string) {
	t.Helper()
	keys := map[string]bool{}
	var all strings.Builder
	fset := token.NewFileSet()
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			all.Write(data)
			f, err := parser.ParseFile(fset, path, data, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok || ident.Name != "i18n" || (sel.Sel.Name != "T" && sel.Sel.Name != "Tf") {
					return true
				}
				if len(call.Args) == 0 {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					keys[s] = true
				}
				return true
			})
		}
	}
	return keys, all.String()
}

func catalogKeys(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("locales", "en.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys
}

func TestCatalogCoversReferencedKeys(t *testing.T) {
	refs, _ := referencedKeys(t)
	catalog := catalogKeys(t)
	for key := range refs {
		if !catalog[key] {
			t.Errorf("key %q is referenced but missing from en.json", key)
		}
	}
}

func TestCatalogHasNoOrphans(t *testing.T) {
	_, source := referencedKeys(t)
	for key := range catalogKeys(t) {
		if !strings.Contains(source, `"`+key+`"`) {
			t.Errorf("catalog key %q is not referenced in the front-end sources", key)
		}
	}
}

func TestInitFallsBackToEnglish(t *testing.T) {
	if err := Init("zz"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := T("menu.file"); got != "File" {
		t.Errorf("fallback translation = %q, want File", got)
	}
	if got := T("does.not.exist"); got != "does.not.exist" {
		t.Errorf("unknown key = %q, want the key itself", got)
	}
}
