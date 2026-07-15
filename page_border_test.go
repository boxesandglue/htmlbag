package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// hasBorderRule walks a node list (descending into nested HList/VList) and
// reports whether it contains the rule HTMLBorder emits for the border edges.
func hasBorderRule(n node.Node) bool {
	for ; n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.Rule:
			if o, _ := v.Attributes["origin"].(string); o == "html border + clipping" {
				return true
			}
		case *node.HList:
			if hasBorderRule(v.List) {
				return true
			}
		case *node.VList:
			if hasBorderRule(v.List) {
				return true
			}
		}
	}
	return false
}

// hasPageBorderNode reports whether pg carries a border box produced by
// HTMLBorder (the border rule is nested inside the vpack/hpack the border box
// is wrapped in), i.e. an @page border was output onto the page.
func hasPageBorderNode(pg *document.Page) bool {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if hasBorderRule(obj.Vlist.List) {
			return true
		}
	}
	return false
}

// TestPageBorderPerPage: an `@page` border must repeat on every page, and
// `@page` padding must act as a content indent. Both InitPage (page 1) and
// NewPage (page 2+) run through renderPageBorderBox, so the border box and
// the padded content area (PageAreaLeft/PageAreaTop) come out identical on
// both.
func TestPageBorderPerPage(t *testing.T) {
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
	// margin:0 anchors the border box at the sheet edge; padding indents the
	// content to x=25mm (5mm border + 20mm padding), y=10mm (padding-top).
	css := `@page { size: a4; margin: 0; border-left: 5mm solid #39b004;
	         padding: 10mm 10mm 35mm 20mm; }`
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}

	wantLeft := bag.MustSP("25mm") // 0 margin + 5mm border + 20mm padding
	wantTop := bag.MustSP("10mm")  // 0 margin + 0 border-top + 10mm padding-top

	// approxEqual tolerates sub-SP rounding from mm→SP conversions.
	approxEqual := func(a, b bag.ScaledPoint) bool {
		d := a - b
		if d < 0 {
			d = -d
		}
		return d < bag.MustSP("0.1mm")
	}

	// Page 1 (InitPage).
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	page1 := cb.frontend.Doc.CurrentPage
	if !hasPageBorderNode(page1) {
		t.Error("page 1: no @page border box output")
	}
	if pd := cb.currentPageDimensions; !approxEqual(pd.PageAreaLeft, wantLeft) {
		t.Errorf("page 1 PageAreaLeft = %s, want %s (padding as indent)", pd.PageAreaLeft, wantLeft)
	}
	if pd := cb.currentPageDimensions; !approxEqual(pd.PageAreaTop, wantTop) {
		t.Errorf("page 1 PageAreaTop = %s, want %s (padding-top as indent)", pd.PageAreaTop, wantTop)
	}

	// Page 2 (NewPage): border must repeat, content area must match.
	if err := cb.NewPage(); err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	page2 := cb.frontend.Doc.CurrentPage
	if page2 == page1 {
		t.Fatal("NewPage did not advance CurrentPage")
	}
	if !hasPageBorderNode(page2) {
		t.Error("page 2: no @page border box output (regression: border only on page 1)")
	}
	if pd := cb.currentPageDimensions; !approxEqual(pd.PageAreaLeft, wantLeft) {
		t.Errorf("page 2 PageAreaLeft = %s, want %s (must match page 1)", pd.PageAreaLeft, wantLeft)
	}
	if pd := cb.currentPageDimensions; !approxEqual(pd.PageAreaTop, wantTop) {
		t.Errorf("page 2 PageAreaTop = %s, want %s (must match page 1)", pd.PageAreaTop, wantTop)
	}
}

// TestPageWithoutBorderUnaffected guards the no-regression claim: without an
// @page border/padding, PageAreaLeft/Top collapse to the plain margins, so the
// body still lands at MarginLeft/MarginTop as before.
func TestPageWithoutBorderUnaffected(t *testing.T) {
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
	if err := cb.ParseCSSString(`@page { size: a4; margin: 2cm; }`); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	pd := cb.currentPageDimensions
	if pd.PageAreaLeft != pd.MarginLeft {
		t.Errorf("PageAreaLeft = %s, want MarginLeft %s (no @page border/padding)", pd.PageAreaLeft, pd.MarginLeft)
	}
	if pd.PageAreaTop != pd.MarginTop {
		t.Errorf("PageAreaTop = %s, want MarginTop %s (no @page border/padding)", pd.PageAreaTop, pd.MarginTop)
	}
}
