package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// leadingProbe renders one paragraph and returns its line HLists plus the
// number of lineskip glues (between lines or trailing) in the paragraph.
func leadingProbe(t *testing.T, css string) (lines []*node.HList, lineskips int) {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if css != "" {
		if err = cb.ParseCSSString(css); err != nil {
			t.Fatal(err)
		}
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><p>several words that wrap onto more than one line for sure</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("100pt"))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch v := e.(type) {
			case *node.VList:
				walk(v.List)
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin == "line" {
					lines = append(lines, v)
				} else {
					walk(v.List)
				}
			case *node.Glue:
				if origin, _ := v.Attributes["origin"].(string); origin == "lineskip" || origin == "last lineskip" {
					lineskips++
				}
			}
		}
	}
	walk(vl.List)
	return lines, lineskips
}

// TestLeadingModelCSSProperty checks the -bag-leading-model routing. Since
// the UA default stylesheet declares half-leading on body (CSS conformance,
// bag issue #28), the default renders full-line-height line boxes without
// lineskip glue; an explicit "trailing" restores the TeX-flavored model.
func TestLeadingModelCSSProperty(t *testing.T) {
	lineHeight := bag.MustSP("12pt") // UA default: 10pt font, line-height 1.2

	// Default (UA stylesheet): half-leading.
	lines, lineskips := leadingProbe(t, "")
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line paragraph, got %d lines", len(lines))
	}
	for i, hl := range lines {
		if got := hl.Height + hl.Depth; got != lineHeight {
			t.Errorf("half-leading line %d spans %s, want %s", i, got, lineHeight)
		}
		if hl.Depth <= 0 {
			t.Errorf("half-leading line %d has no depth share (%s)", i, hl.Depth)
		}
	}
	if lineskips != 0 {
		t.Errorf("half-leading paragraph still carries %d lineskip glues", lineskips)
	}

	// Explicit trailing: lines keep their natural metrics and the leading
	// sits in lineskip glues.
	lines, lineskips = leadingProbe(t, `p { -bag-leading-model: trailing; }`)
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line paragraph, got %d lines", len(lines))
	}
	for i, hl := range lines {
		if got := hl.Height + hl.Depth; got >= lineHeight {
			t.Errorf("trailing-leading line %d spans %s, expected less than %s", i, got, lineHeight)
		}
	}
	if lineskips == 0 {
		t.Error("trailing-leading paragraph should carry lineskip glues")
	}

	// Re-enabling half on the element overrides an inherited trailing.
	lines, lineskips = leadingProbe(t, `body { -bag-leading-model: trailing; } p { -bag-leading-model: half; }`)
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line paragraph, got %d lines", len(lines))
	}
	if lineskips != 0 {
		t.Error("explicit half must remove the lineskip glues again")
	}
}
