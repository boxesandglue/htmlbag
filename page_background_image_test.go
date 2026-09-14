package htmlbag

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestStyleValueURI covers reading a url() value off the token stream: the
// wrapper and both quote styles are the scanner's business, and anything that
// is not a url() reports so rather than handing back its own text.
func TestStyleValueURI(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		isURL bool
	}{
		{`url(brief.pdf)`, "brief.pdf", true},
		{`url("brief.pdf")`, "brief.pdf", true},
		{`url('brief.pdf')`, "brief.pdf", true},
		{`  url(  "a b.pdf"  )  `, "a b.pdf", true},
		{`none`, "", false},
		{``, "", false},
		{`brief.pdf`, "", false},
	}
	for _, c := range cases {
		got, isURL := textValue(c.in).uri()
		if got != c.want || isURL != c.isURL {
			t.Errorf("uri(%q) = %q, %v; want %q, %v", c.in, got, isURL, c.want, c.isURL)
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
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	cb.css.FileFinder = func(s string) (string, error) {
		return filepath.Join(dir, s), nil
	}
	css := `@page        { size: 200pt 200pt; margin: 10pt; background-image: url(bg2.png); }
	        @page :first { background-image: url(bg1.png); }`
	if err := cb.AddCSS(css); err != nil {
		t.Fatalf("AddCSS: %v", err)
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

// TestPageBackgroundImageStylesheetRelative: a relative url() in an @page
// rule of an external stylesheet must resolve against the stylesheet's
// directory, not the document directory (csshtml issue #3). ReadCSSFile
// pushes the stylesheet's directory around parsing; doPage resolves the
// path inside that window, so the stored page attribute is already absolute.
func TestPageBackgroundImageStylesheetRelative(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "template")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cssfile := filepath.Join(sub, "custom.css")
	css := `@page { size: a4; background-image: url(briefbogen.pdf); }`
	if err := os.WriteFile(cssfile, []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.ReadCSSFile(cssfile); err != nil {
		t.Fatalf("ReadCSSFile: %v", err)
	}
	pt := cb.getPageType()
	if pt == nil {
		t.Fatal("getPageType returned nil")
	}
	got, isURL := resolveDeclarations(pt.Attributes)["background-image"].uri()
	want := filepath.Join(sub, "briefbogen.pdf")
	if !isURL || got != want {
		t.Errorf("background-image = %q (url=%v), want %q (stylesheet relative)", got, isURL, want)
	}
}

// TestPageBackgroundImageCustomProperty proves the -bag-background-page
// custom property survives CSS parsing and attribute resolution (unknown
// @page properties flow through css.doPage's default case into
// resolveDeclarations' default case as a raw value). This is what lets a
// two-page letterhead PDF drive page 1 vs. page 2+ from a single file.
func TestPageBackgroundImageCustomProperty(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	css := `@page { size: a4; background-image: url(brief.pdf); -bag-background-page: 2; }`
	if err := cb.AddCSS(css); err != nil {
		t.Fatalf("AddCSS: %v", err)
	}
	pt := cb.getPageType()
	if pt == nil {
		t.Fatal("getPageType returned nil")
	}
	res := resolveDeclarations(pt.Attributes)
	// AddCSS puts the working directory on the dir stack, so the relative
	// url() comes back resolved. The subject here is that the value survives
	// at all, not what it resolves against, so only the target is pinned.
	if got, isURL := res["background-image"].uri(); !isURL || !strings.HasSuffix(got, "brief.pdf") {
		t.Errorf("background-image = %q (url=%v), want a url() pointing at brief.pdf", got, isURL)
	}
	if got := res.Get("-bag-background-page"); got != "2" {
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
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.AddCSS(css); err != nil {
		t.Fatalf("AddCSS: %v", err)
	}
	pt := cb.getPageType() // fresh doc: 0 pages placed, so :first applies
	if pt == nil {
		t.Fatal("getPageType returned nil")
	}
	res := resolveDeclarations(pt.Attributes)
	return res.Get("-bag-background-page")
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
