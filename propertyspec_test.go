package htmlbag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// caseLabels returns the string literals used as case labels of the first
// switch statement in the named function of file. The first switch is the one
// on the property name in every function this test looks at, the nested ones
// dispatch on the value.
func caseLabels(t *testing.T, fset *token.FileSet, file, fn string) []string {
	t.Helper()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn {
			continue
		}
		found := false
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if found {
				return false
			}
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			found = true
			for _, stmt := range sw.Body.List {
				cc := stmt.(*ast.CaseClause)
				for _, expr := range cc.List {
					if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						s, err := strconv.Unquote(lit.Value)
						if err != nil {
							t.Fatal(err)
						}
						labels = append(labels, s)
					}
				}
			}
			return false
		})
		if !found {
			t.Fatalf("%s: no switch statement in %s", file, fn)
		}
		return labels
	}
	t.Fatalf("%s: function %s not found", file, fn)
	return nil
}

// stringLiterals collects every string literal of the non-test Go files of
// the package.
func stringLiterals(t *testing.T, fset *token.FileSet) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					out[s] = true
				}
			}
			return true
		})
	}
	return out
}

// TestPropertySpecCoverage keeps Properties and the code in step: every
// property a switch dispatches on has an entry, and every entry names a
// property the code mentions somewhere.
func TestPropertySpecCoverage(t *testing.T) {
	if _, err := os.Stat("inheritablestyles.go"); err != nil {
		t.Skip("package sources not available")
	}
	elementGroups := []PropertyGroup{GroupText, GroupBox, GroupBorder, GroupLayout, GroupGenerated, GroupCustom}
	sources := []struct {
		file, fn string
		groups   []PropertyGroup
	}{
		{"inheritablestyles.go", "StylesToStyles", elementGroups},
		{"computedstyles.go", "resolveDeclarations", elementGroups},
		{"html.go", "CSSPropertiesToValues", elementGroups},
		{"css.go", "doPage", []PropertyGroup{GroupPage}},
		{"css.go", "doFontFace", []PropertyGroup{GroupFontFace}},
		{"colors.go", "doColor", []PropertyGroup{GroupColor}},
	}

	byGroup := map[PropertyGroup]map[string]bool{}
	for _, p := range Properties {
		if byGroup[p.Group] == nil {
			byGroup[p.Group] = map[string]bool{}
		}
		for _, n := range p.Names() {
			if byGroup[p.Group][n] {
				t.Errorf("property %q listed twice in group %q", n, p.Group)
			}
			byGroup[p.Group][n] = true
		}
	}

	fset := token.NewFileSet()
	for _, src := range sources {
		for _, label := range caseLabels(t, fset, src.file, src.fn) {
			known := false
			for _, g := range src.groups {
				if byGroup[g][label] {
					known = true
				}
			}
			if !known {
				t.Errorf("%s: %s reads %q, but Properties has no entry for it", src.file, src.fn, label)
			}
		}
	}

	literals := stringLiterals(t, fset)
	for _, p := range Properties {
		for _, n := range p.Names() {
			if !literals[n] {
				t.Errorf("Properties lists %q (group %q), but no source file mentions it", n, p.Group)
			}
		}
	}
}

func TestPropertySpecConsistency(t *testing.T) {
	groups := map[PropertyGroup]bool{}
	for _, g := range PropertyGroups {
		groups[g] = true
	}
	for _, p := range Properties {
		if !groups[p.Group] {
			t.Errorf("%s: group %q is not in PropertyGroups", p.Name, p.Group)
		}
		if p.Values == "" {
			t.Errorf("%s: empty Values", p.Name)
		}
		if !strings.HasSuffix(p.Example, ";") {
			t.Errorf("%s: example %q must end with a semicolon", p.Name, p.Example)
		}
		prop, _, ok := strings.Cut(p.Example, ":")
		if !ok {
			t.Errorf("%s: example %q is not a declaration", p.Name, p.Example)
			continue
		}
		matches := false
		for _, n := range p.Names() {
			if n == prop {
				matches = true
			}
		}
		if !matches {
			t.Errorf("%s: example %q does not use the property or one of its aliases", p.Name, p.Example)
		}
	}
}
