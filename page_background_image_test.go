package htmlbag

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestStripCSSURL covers the url() unwrapper: the wrapper, both quote
// styles, surrounding whitespace, and bare/degenerate inputs.
func TestStripCSSURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{`url(brief.pdf)`, "brief.pdf"},
		{`url("brief.pdf")`, "brief.pdf"},
		{`url('brief.pdf')`, "brief.pdf"},
		{`  url(  "a b.pdf"  )  `, "a b.pdf"},
		{`none`, "none"},
		{``, ""},
		{`brief.pdf`, "brief.pdf"},
	}
	for _, c := range cases {
		if got := stripCSSURL(c.in); got != c.want {
			t.Errorf("stripCSSURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// bgImageNode returns the *node.Image behind the "page background image"
// object placed on pg, or nil if none was placed.
func bgImageNode(pg *document.Page) *node.Image {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if o, _ := obj.Vlist.Attributes["origin"].(string); o != "page background image" {
			continue
		}
		for n := obj.Vlist.List; n != nil; n = n.Next() {
			if img, ok := n.(*node.Image); ok {
				return img
			}
		}
	}
	return nil
}

// TestPageBackgroundImagePerPage proves that `@page { background-image }`
// resolves per page: `@page :first` paints page 1, the generic `@page`
// paints page 2. This is the reines-CSS/Markdown route (no PageInitCallback,
// no Lua) that the letterhead use case needs. See seite-2-problem.md, Teil A.
func TestPageBackgroundImagePerPage(t *testing.T) {
	dir := t.TempDir()
	writeTinyPNG(t, dir, "bg1.png")
	writeTinyPNG(t, dir, "bg2.png")

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	cb.css.FileFinder = func(s string) (string, error) {
		return filepath.Join(dir, s), nil
	}
	css := `@page        { size: 200pt 200pt; margin: 10pt; background-image: url(bg2.png); }
	        @page :first { background-image: url(bg1.png); }`
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}

	// Page 1 is drawn by InitPage.
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	page1 := cb.frontend.Doc.CurrentPage
	img1 := bgImageNode(page1)
	if img1 == nil {
		t.Fatal("page 1: no page background image placed")
	}
	if !strings.HasSuffix(img1.ImageFile.Filename, "bg1.png") {
		t.Errorf("page 1 background = %q, want …bg1.png (@page :first)", img1.ImageFile.Filename)
	}
	if img1.ImageFile.PageNumber != 1 {
		t.Errorf("page 1 source page = %d, want 1 (default)", img1.ImageFile.PageNumber)
	}

	// A fresh page picks the generic @page rule (page index > 0).
	if err := cb.NewPage(); err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	page2 := cb.frontend.Doc.CurrentPage
	if page2 == page1 {
		t.Fatal("NewPage did not advance CurrentPage")
	}
	img2 := bgImageNode(page2)
	if img2 == nil {
		t.Fatal("page 2: no page background image placed")
	}
	if !strings.HasSuffix(img2.ImageFile.Filename, "bg2.png") {
		t.Errorf("page 2 background = %q, want …bg2.png (generic @page)", img2.ImageFile.Filename)
	}
}

// TestPageBackgroundImageCustomProperty proves the -bag-background-page
// custom property survives CSS parsing and attribute resolution (unknown
// @page properties flow through css.doPage's default case into
// ResolveAttributes' default case as a raw value). This is what lets a
// two-page letterhead PDF drive page 1 vs. page 2+ from a single file.
func TestPageBackgroundImageCustomProperty(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	css := `@page { size: a4; background-image: url(brief.pdf); -bag-background-page: 2; }`
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	pt := cb.getPageType()
	if pt == nil {
		t.Fatal("getPageType returned nil")
	}
	res, _ := csshtml.ResolveAttributes(pt.Attributes)
	if got := res["background-image"]; got != "url(brief.pdf)" {
		t.Errorf("background-image = %q, want url(brief.pdf)", got)
	}
	if got := res["-bag-background-page"]; got != "2" {
		t.Errorf("-bag-background-page = %q, want 2", got)
	}
}

// pageOnePageProp resolves the -bag-background-page that applies to page 1
// (the @page :first selector) for the given stylesheet.
func pageOnePageProp(t *testing.T, css string) string {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	pt := cb.getPageType() // fresh doc: 0 pages placed, so :first applies
	if pt == nil {
		t.Fatal("getPageType returned nil")
	}
	res, _ := csshtml.ResolveAttributes(pt.Attributes)
	return res["-bag-background-page"]
}

// TestPageBackgroundPageCascade pins the paged-media cascade behaviour that
// bit the letterhead example: @page :first inherits -bag-background-page from
// the generic @page unless it redeclares it. A rule that omits the property
// leaks the generic value onto page 1; setting it explicitly wins.
func TestPageBackgroundPageCascade(t *testing.T) {
	// :first omits -bag-background-page → inherits "2" from @page (the gotcha).
	leaked := `@page        { size: a4; background-image: url(b.pdf); -bag-background-page: 2; }
	           @page :first { background-image: url(b.pdf); }`
	if got := pageOnePageProp(t, leaked); got != "2" {
		t.Errorf("inherited -bag-background-page = %q, want 2 (cascade leak)", got)
	}

	// :first sets it explicitly → its own value wins (the fix).
	fixed := `@page        { size: a4; background-image: url(b.pdf); -bag-background-page: 2; }
	          @page :first { background-image: url(b.pdf); -bag-background-page: 1; }`
	if got := pageOnePageProp(t, fixed); got != "1" {
		t.Errorf("explicit -bag-background-page = %q, want 1 (pseudo wins)", got)
	}
}
