package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// renderForOutline runs the htmlbag pipeline far enough to populate the PDF
// outline tree and returns the CSSBuilder (so callers can inspect
// cb.frontend.Doc.PDFWriter.Outlines) without finishing the document. The
// optional configure hook runs after New so a test can flip cb fields (e.g.
// GenerateOutline) before rendering.
func renderForOutline(t *testing.T, css, html string, configure func(*CSSBuilder)) *CSSBuilder {
	t.Helper()
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
	if configure != nil {
		configure(cb)
	}
	if css != "" {
		if err := cb.ParseCSSString(css); err != nil {
			t.Fatalf("ParseCSSString: %v", err)
		}
	}
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	return cb
}

// outlineSketch renders the outline tree as nested "title(level…)" lines so a
// test can assert structure and nesting in one string. Depth is the
// indentation; Open is shown as a trailing "*".
func outlineSketch(outlines []*pdf.Outline) string {
	var b strings.Builder
	var walk func(os []*pdf.Outline, depth int)
	walk = func(os []*pdf.Outline, depth int) {
		for _, o := range os {
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(o.Title)
			if o.Open {
				b.WriteString("*")
			}
			b.WriteString("\n")
			walk(o.Children, depth+1)
		}
	}
	walk(outlines, 0)
	return b.String()
}

// TestOutlineEndToEndPDF renders a full document, finishes it, and checks the
// serialized PDF actually carries an /Outlines catalog entry plus a bookmark
// title — proving the in-memory tree reaches the file via baseline-pdf.
func TestOutlineEndToEndPDF(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
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
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><h1>Alpha</h1><h2>Beta</h2></body></html>`)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatalf("frontend.Finish: %v", err)
	}
	pdf := buf.String()
	if !strings.Contains(pdf, "/Outlines") {
		t.Error("serialized PDF has no /Outlines catalog entry")
	}
	// Bookmark titles are written as PDF string literals; "Alpha" is ASCII.
	if !strings.Contains(pdf, "(Alpha)") {
		t.Error("serialized PDF has no bookmark titled Alpha")
	}
	// Non-UA destinations are /XYZ arrays so the bookmark jumps to the
	// heading's exact vertical position.
	if !strings.Contains(pdf, "/XYZ") {
		t.Error("serialized PDF has no /XYZ outline destination")
	}
}

// TestOutlineStrictNesting verifies the automatic h1–h6 outline nests strictly
// by level: an h2 becomes a child of the preceding h1, an h3 a child of the
// preceding h2, and a following h2 starts a new sibling under the h1.
func TestOutlineStrictNesting(t *testing.T) {
	html := `<!DOCTYPE html><html><body>` +
		`<h1>One</h1><h2>One A</h2><h3>One A i</h3><h2>One B</h2><h1>Two</h1>` +
		`</body></html>`
	cb := renderForOutline(t, "", html, nil)
	got := outlineSketch(cb.frontend.Doc.PDFWriter.Outlines)
	want := "One*\n  One A*\n    One A i*\n  One B*\nTwo*\n"
	if got != want {
		t.Errorf("outline mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// TestOutlineLevelJumpClamps verifies that a level gap (h1 → h3 with no h2)
// attaches the deeper heading to the nearest shallower ancestor instead of
// orphaning it or panicking.
func TestOutlineLevelJumpClamps(t *testing.T) {
	html := `<!DOCTYPE html><html><body><h1>Root</h1><h3>Deep</h3></body></html>`
	cb := renderForOutline(t, "", html, nil)
	outlines := cb.frontend.Doc.PDFWriter.Outlines
	if len(outlines) != 1 || len(outlines[0].Children) != 1 {
		t.Fatalf("level-jump outline = %q; want Root with one child", outlineSketch(outlines))
	}
	if outlines[0].Children[0].Title != "Deep" {
		t.Errorf("child title = %q; want Deep", outlines[0].Children[0].Title)
	}
}

// TestOutlineNoHeadingsNoOutline verifies a document without headings produces
// no outline at all (no error, no empty tree).
func TestOutlineNoHeadingsNoOutline(t *testing.T) {
	cb := renderForOutline(t, "", `<!DOCTYPE html><html><body><p>just text</p></body></html>`, nil)
	if n := len(cb.frontend.Doc.PDFWriter.Outlines); n != 0 {
		t.Errorf("expected no outline, got %d top-level entries", n)
	}
}

// TestOutlineOptOut verifies GenerateOutline=false suppresses the outline even
// when headings exist (the opt-out used by glu).
func TestOutlineOptOut(t *testing.T) {
	html := `<!DOCTYPE html><html><body><h1>One</h1><h2>Two</h2></body></html>`
	cb := renderForOutline(t, "", html, func(cb *CSSBuilder) { cb.GenerateOutline = false })
	if n := len(cb.frontend.Doc.PDFWriter.Outlines); n != 0 {
		t.Errorf("GenerateOutline=false should suppress outline, got %d entries", n)
	}
}

// TestBookmarkNoneExcludes verifies -bag-bookmark: none removes a heading from
// the outline while leaving it in the heading list (TOC).
func TestBookmarkNoneExcludes(t *testing.T) {
	css := `.skip { -bag-bookmark: none; }`
	html := `<!DOCTYPE html><html><body><h1>Kept</h1><h2 class="skip">Skipped</h2></body></html>`
	cb := renderForOutline(t, css, html, nil)
	got := outlineSketch(cb.frontend.Doc.PDFWriter.Outlines)
	if got != "Kept*\n" {
		t.Errorf("outline = %q; want only the kept heading", got)
	}
	// The excluded heading must still be in the TOC heading list.
	if len(cb.Headings) != 2 {
		t.Errorf("Headings = %d; want 2 (excluded heading stays in the TOC list)", len(cb.Headings))
	}
}

// TestBookmarkArbitraryElement verifies -bag-bookmark turns a non-heading
// element into an outline entry, with explicit level and collapsed state.
func TestBookmarkArbitraryElement(t *testing.T) {
	css := `h1 { -bag-bookmark: 1; }
p.mark { -bag-bookmark: 2 closed; }`
	html := `<!DOCTYPE html><html><body><h1>Chapter</h1><p class="mark">A marked paragraph</p></body></html>`
	cb := renderForOutline(t, css, html, nil)
	outlines := cb.frontend.Doc.PDFWriter.Outlines
	if len(outlines) != 1 || len(outlines[0].Children) != 1 {
		t.Fatalf("outline = %q; want Chapter with one child", outlineSketch(outlines))
	}
	child := outlines[0].Children[0]
	if child.Title != "A marked paragraph" {
		t.Errorf("child title = %q; want the paragraph text", child.Title)
	}
	if child.Open {
		t.Errorf("child should be collapsed (-bag-bookmark: ... closed)")
	}
}

// TestBookmarkLevelOverride verifies an explicit -bag-bookmark level overrides
// the implicit heading level, re-parenting the heading in the outline.
func TestBookmarkLevelOverride(t *testing.T) {
	// h1 "Root", then an h1 forced to level 2 → it nests under the first h1.
	css := `.sub { -bag-bookmark: 2; }`
	html := `<!DOCTYPE html><html><body><h1>Root</h1><h1 class="sub">Forced child</h1></body></html>`
	cb := renderForOutline(t, css, html, nil)
	got := outlineSketch(cb.frontend.Doc.PDFWriter.Outlines)
	if got != "Root*\n  Forced child*\n" {
		t.Errorf("outline = %q; want Forced child nested under Root", got)
	}
}
