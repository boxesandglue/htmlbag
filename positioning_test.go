package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// positionedObject returns X and Y of the page object carrying the given
// text, or (-1, -1) when it is not on the page.
func positionedObject(pg *document.Page, needle string) (bag.ScaledPoint, bag.ScaledPoint) {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectComponents(obj.Vlist.List, &sb)
		if strings.Contains(sb.String(), needle) {
			return obj.X, obj.Y
		}
	}
	return -1, -1
}

// TestAbsolutePositionAnchorsToPageBox: the initial containing block for
// position: absolute is the page box (the physical sheet), not the
// @page-margin-shrunk content area. With @page margin 10/10/35/25 a block
// at top: 63mm / left: 25mm must land exactly 63/25 mm from the sheet
// edges (not 73/50 mm).
func TestAbsolutePositionAnchorsToPageBox(t *testing.T) {
	css := `@page { size: a4; margin: 10mm 10mm 35mm 25mm; }
.addressee { position: absolute; top: 63mm; left: 25mm; width: 80mm; }`
	html := `<html><body>
<div class="addressee">EMPFAENGERBLOCK</div>
<p>FLIESSTEXT im normalen Fluss</p>
</body></html>`
	pages := renderHTMLPages(t, css, html)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	x, y := positionedObject(pages[0], "EMPFAENGERBLOCK")
	if x == -1 {
		t.Fatal("positioned block not found on page")
	}
	wantX := bag.MustSP("25mm")
	wantY := bag.MustSP("297mm") - bag.MustSP("63mm")
	if x != wantX {
		t.Errorf("positioned block at x=%s, want %s (25mm from the left sheet edge)", x, wantX)
	}
	if y != wantY {
		t.Errorf("positioned block at y=%s, want %s (63mm from the top sheet edge)", y, wantY)
	}
	// The in-flow paragraph keeps resolving against the content area:
	// it starts at the @page margins, not at the sheet edge.
	px, py := positionedObject(pages[0], "FLIESSTEXT")
	if px != bag.MustSP("25mm") {
		t.Errorf("flow paragraph at x=%s, want %s (margin-left)", px, bag.MustSP("25mm"))
	}
	if py > bag.MustSP("297mm")-bag.MustSP("10mm") {
		t.Errorf("flow paragraph at y=%s, must be below the 10mm top margin", py)
	}
}
