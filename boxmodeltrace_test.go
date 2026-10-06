package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestBoxModelOverlayRule checks the generated PDF code of the overlay
// rule: translucent mode activates the shared ExtGState and paints rings
// with the even-odd rule; degenerate rings (equal rectangles) are skipped.
func TestBoxModelOverlayRule(t *testing.T) {
	ten := bag.MustSP("10pt")
	content := traceRect{llx: 0, lly: -ten, urx: 10 * ten, ury: 0}
	padding := content.grow(ten, ten, ten, ten)
	border := padding // no border
	margin := border.grow(ten, ten, ten, ten)

	r := boxModelOverlayRule(margin, border, padding, content, true)
	if !r.Hide {
		t.Error("overlay rule must be hidden (Pre-only)")
	}
	if !strings.Contains(r.Pre, "/GSca0_4 gs") {
		t.Errorf("transparent overlay must activate the alpha ExtGState, got %q", r.Pre)
	}
	if !strings.Contains(r.Pre, "f*") {
		t.Errorf("rings must fill with the even-odd rule, got %q", r.Pre)
	}
	if gss, ok := r.Attributes["extgstates"].([]pdf.ExtGState); !ok || len(gss) != 1 {
		t.Errorf("overlay rule must request exactly one ExtGState via attributes, got %v", r.Attributes["extgstates"])
	}
	// border == padding: the border ring is empty, so the yellow border
	// color must not appear (only margin, padding and content layers).
	if got := strings.Count(r.Pre, "rg"); got != 3 {
		t.Errorf("expected 3 fill colors (margin, padding, content), got %d in %q", got, r.Pre)
	}

	// Opaque fallback: no ExtGState, outlines only.
	r = boxModelOverlayRule(margin, border, padding, content, false)
	if strings.Contains(r.Pre, " gs") {
		t.Errorf("opaque fallback must not use an ExtGState, got %q", r.Pre)
	}
	if !strings.Contains(r.Pre, "S") {
		t.Errorf("opaque fallback must stroke outlines, got %q", r.Pre)
	}
	if _, ok := r.Attributes["extgstates"]; ok {
		t.Error("opaque fallback must not request an ExtGState")
	}
}

// TestBoxModelTraceEndToEnd renders a small document with TraceBoxModel
// enabled and checks that the serialized PDF registers the alpha ExtGState
// in the page resources. Covers both the HTMLBorder hook (bordered p) and
// the borderless leaf hook (plain p with margins).
func TestBoxModelTraceEndToEnd(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	cb.TraceBoxModel = true
	if err = cb.AddCSS(`.boxed { border: 1pt solid black; padding: 3pt; }`); err != nil {
		t.Fatalf("AddCSS: %v", err)
	}
	if err = cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><p>plain paragraph</p><p class="boxed">boxed paragraph</p></body></html>`)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err = cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	if err = fe.Finish(); err != nil {
		t.Fatalf("frontend.Finish: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "/ExtGState") {
		t.Error("serialized PDF has no /ExtGState resource entry")
	}
	if !strings.Contains(out, "/GSca0_4") {
		t.Error("serialized PDF does not register the trace alpha graphics state")
	}
	if !strings.Contains(out, "/ca 0.4") {
		t.Error("serialized PDF misses the /ca 0.4 parameter dictionary")
	}
}

// traceOverlays renders body with css and returns how many box model
// overlays the PDF paints: each sets the overlay's alpha graphics state once.
func traceOverlays(t *testing.T, global bool, css, body string) int {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	fe.Doc.CompressLevel = 0
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	cb.TraceBoxModel = global
	if err = cb.AddCSS(css); err != nil {
		t.Fatal(err)
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err = cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err = fe.Finish(); err != nil {
		t.Fatal(err)
	}
	return strings.Count(buf.String(), "/GSca0_4 gs")
}

// -bag-trace: boxmodel paints the overlay on the matched elements only,
// without the global switch: not on their children (it is not inherited),
// not where a more specific rule says none, and once where the global
// switch is on as well. A span with it is inline, gets no box, and must not
// carry the mark into the paragraph's formatting.
func TestBagTraceBoxModel(t *testing.T) {
	const css = `.x { -bag-trace: boxmodel } .off { -bag-trace: none }`
	for _, tc := range []struct {
		name, body string
		global     bool
		want       int
	}{
		{"matched paragraph", `<p class="x">a</p><p>b</p><p>c</p>`, false, 1},
		{"no property", `<p>a</p><p>b</p>`, false, 0},
		{"none in a more specific rule", `<p class="x off">a</p>`, false, 0},
		{"not inherited", `<div class="x" style="border: 1pt solid red"><p>a</p><p>b</p></div>`, false, 1},
		{"list item", `<ul><li class="x">a</li><li>b</li></ul>`, false, 1},
		{"heading", `<h2 class="x">a</h2><p>b</p>`, false, 1},
		{"inline span", `<p>a <span class="x">b</span> c</p>`, false, 0},
		{"span in a traced paragraph", `<p class="x">a <span class="x">b</span></p>`, false, 1},
		{"with the global switch", `<p class="x">a</p>`, true, -1},
		// A paragraph split across pages loses its overlay, with the global
		// switch as well; what counts here is that the split, which formats
		// the rest of the paragraph again, gets no trace mark.
		{"split across pages", `<p class="x">` + strings.Repeat("word ", 1500) + `</p>`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want < 0 {
				// Once, as without the property.
				want = traceOverlays(t, true, "", `<p>a</p>`)
			}
			if got := traceOverlays(t, tc.global, css, tc.body); got != want {
				t.Errorf("%d overlays, want %d", got, want)
			}
		})
	}
}

// The rest of a traced paragraph that goes on to a wider page is set again
// at that page's width, as without the trace: the trace mark is taken off
// the paragraph for the new setting too, where the frontend would refuse it
// and the rest would keep the narrower width.
func TestBagTraceKeepsPageWidthReflow(t *testing.T) {
	html := `<html><body><p class="x">` + strings.Repeat("Wort ", 2000) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, reflowNarrowFirstCSS+` .x { -bag-trace: boxmodel }`, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	requireWidth(t, "page 1", maxLineWidth(pages[0]), bag.MustSP("130mm"))
	requireWidth(t, "page 2", maxLineWidth(pages[1]), bag.MustSP("170mm"))
}
