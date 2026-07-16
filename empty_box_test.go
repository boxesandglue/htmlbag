package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// hasBoxOfSize reports whether the node list (descending into nested
// lists) contains a VList or HList of exactly the given outer size.
func hasBoxOfSize(n node.Node, wd, ht bag.ScaledPoint) bool {
	for ; n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.VList:
			if v.Width == wd && v.Height+v.Depth == ht {
				return true
			}
			if hasBoxOfSize(v.List, wd, ht) {
				return true
			}
		case *node.HList:
			if v.Width == wd && v.Height+v.Depth == ht {
				return true
			}
			if hasBoxOfSize(v.List, wd, ht) {
				return true
			}
		}
	}
	return false
}

// TestEmptyBlockWithHeightPaintsBackground: a childless div with
// width/height/background-color must produce a visible box of exactly
// the declared size (empty blocks used to collapse to zero height).
func TestEmptyBlockWithHeightPaintsBackground(t *testing.T) {
	css := `@page { size: a4; margin: 20mm; }
.swatch { width: 5mm; height: 40mm; background-color: #39b004; }`
	html := `<html><body>
<p>VOR dem Swatch</p>
<div class="swatch"></div>
<p>NACH dem Swatch</p>
</body></html>`
	pages := renderHTMLPages(t, css, html)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	wantW, wantH := bag.MustSP("5mm"), bag.MustSP("40mm")
	found := false
	for _, obj := range pages[0].Objects {
		if obj.Vlist == nil {
			continue
		}
		if hasBoxOfSize(obj.Vlist, wantW, wantH) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no %s x %s box found on the page", wantW, wantH)
	}
	// The following paragraph must sit below the swatch: at least 40mm
	// lower than the preceding one.
	beforeY := paragraphY(pages[0], "VORdemSwatch")
	afterY := paragraphY(pages[0], "NACHdemSwatch")
	if beforeY == -1 || afterY == -1 {
		t.Fatal("marker paragraphs not found")
	}
	if beforeY-afterY < wantH {
		t.Errorf("paragraph after the swatch is only %s below the one before it, want at least %s", beforeY-afterY, wantH)
	}
}

// TestEmptyBlockHeightReservesFlowSpace: a bare height-only spacer (no
// background) must push the following flow content down by its height.
func TestEmptyBlockHeightReservesFlowSpace(t *testing.T) {
	css := `@page { size: a4; margin: 20mm; }
.spacer { height: 85mm; }`
	withSpacer := `<html><body><div class="spacer"></div><p>FLIESSTEXT nach dem Spacer</p></body></html>`
	control := `<html><body><p>FLIESSTEXT nach dem Spacer</p></body></html>`
	pagesA := renderHTMLPages(t, css, withSpacer)
	pagesB := renderHTMLPages(t, css, control)
	yA := paragraphY(pagesA[0], "FLIESSTEXT")
	yB := paragraphY(pagesB[0], "FLIESSTEXT")
	if yA == -1 || yB == -1 {
		t.Fatal("marker paragraph not found")
	}
	if diff := yB - yA; diff < bag.MustSP("85mm") {
		t.Errorf("spacer shifted the paragraph by %s, want at least 85mm", diff)
	}
}
