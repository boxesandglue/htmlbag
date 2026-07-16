package htmlbag

import (
	"reflect"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// glyphObjectPositions returns the X/Y coordinates of every object on the
// page that carries at least one glyph, in output order. Decoration-only
// objects (e.g. the sheet-filling @page border/padding box) are skipped so
// the two @page variants below stay comparable.
func glyphObjectPositions(pg *document.Page) [][2]bag.ScaledPoint {
	var pos [][2]bag.ScaledPoint
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectComponents(obj.Vlist.List, &sb)
		if sb.Len() > 0 {
			pos = append(pos, [2]bag.ScaledPoint{obj.X, obj.Y})
		}
	}
	return pos
}

// lowestGlyphInk returns the lowest y coordinate any glyph-carrying object
// on the page reaches (object top minus its vlist extent).
func lowestGlyphInk(pg *document.Page) bag.ScaledPoint {
	low := bag.ScaledPoint(1 << 62)
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectComponents(obj.Vlist.List, &sb)
		if sb.Len() == 0 {
			continue
		}
		if bottom := obj.Y - obj.Vlist.Height - obj.Vlist.Depth; bottom < low {
			low = bottom
		}
	}
	return low
}

// TestSplitTableRespectsPagePadding: when the content area comes from
// @page { margin: 0; padding: ... } (the pattern a full-height page border
// bar requires), a table that splits across pages must honor it exactly like
// equivalent margins. The split path used to ignore the padding entirely:
// rows and the repeated header started at the sheet edge (x=0/y=0) and ran
// through the bottom padding.
func TestSplitTableRespectsPagePadding(t *testing.T) {
	html := `<html><body><table class="items">
<thead><tr><th>Kopf</th><th>Preis</th></tr></thead>
<tbody>` + itemRows(70) + `</tbody></table>
<p>SCHLUSSTEXT nach der Tabelle</p></body></html>`
	tableCSS := `
table.items { width: 100%; border-collapse: collapse; }
table.items td { padding: 2pt 4pt; }`
	marginPages := renderHTMLPages(t, `@page { size: a4; margin: 10mm 10mm 35mm 20mm; }`+tableCSS, html)
	paddingPages := renderHTMLPages(t, `@page { size: a4; margin: 0; padding: 10mm 10mm 35mm 20mm; }`+tableCSS, html)

	if len(paddingPages) != len(marginPages) {
		t.Fatalf("padding variant has %d pages, margin variant %d — want identical", len(paddingPages), len(marginPages))
	}
	requireAllRows(t, paddingPages, 70)
	if !strings.Contains(pageText(paddingPages[len(paddingPages)-1]), "SCHLUSSTEXT") {
		t.Error("closing paragraph not on the table's last page")
	}

	// Content area edges: 20mm left, 10mm top, 35mm bottom free.
	left := bag.MustSP("20mm")
	top := bag.MustSP("297mm") - bag.MustSP("10mm")
	bottom := bag.MustSP("35mm") - bag.MustSP("2mm") // descender tolerance
	for i, pg := range paddingPages {
		for _, p := range glyphObjectPositions(pg) {
			if p[0] < left {
				t.Errorf("page %d: object at x=%s, left of the padding-left edge (%s)", i+1, p[0], left)
			}
			if p[1] > top {
				t.Errorf("page %d: object at y=%s, above the padding-top edge (%s)", i+1, p[1], top)
			}
		}
		if low := lowestGlyphInk(pg); low < bottom {
			t.Errorf("page %d: content reaches y=%s, into the bottom padding (limit %s)", i+1, low, bottom)
		}
	}

	// Acceptance criterion: both @page variants render congruent pages —
	// every text-carrying object sits at identical coordinates.
	for i := range marginPages {
		want := glyphObjectPositions(marginPages[i])
		got := glyphObjectPositions(paddingPages[i])
		if !reflect.DeepEqual(want, got) {
			t.Errorf("page %d: object positions differ between margin and padding variant:\nmargin:  %v\npadding: %v", i+1, want, got)
		}
	}
}
