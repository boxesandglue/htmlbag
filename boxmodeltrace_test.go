package htmlbag

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
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
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	cb.TraceBoxModel = true
	if err = cb.ParseCSSString(`.boxed { border: 1pt solid black; padding: 3pt; }`); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
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
