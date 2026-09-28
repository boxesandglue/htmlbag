package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// glyphExtent returns the left edge of the first glyph and the right edge of
// the last one in list, whose left edge is at x. A VList stacks its children,
// an HList sets them side by side.
func glyphExtent(list node.Node, x bag.ScaledPoint, horizontal bool) (first, last bag.ScaledPoint, found bool) {
	take := func(f, l bag.ScaledPoint) {
		if !found {
			first, found = f, true
		}
		last = l
	}
	for n := list; n != nil; n = n.Next() {
		var wd bag.ScaledPoint
		switch t := n.(type) {
		case *node.Glyph:
			take(x, x+t.Width)
			wd = t.Width
		case *node.Glue:
			wd = t.Width
		case *node.Kern:
			wd = t.Kern
		case *node.HList:
			if f, l, ok := glyphExtent(t.List, x+t.ShiftX, true); ok {
				take(f, l)
			}
			wd = t.Width
		case *node.VList:
			if f, l, ok := glyphExtent(t.List, x+t.ShiftX, false); ok {
				take(f, l)
			}
			wd = t.Width
		}
		if horizontal {
			x += wd
		}
	}
	return
}

// A paragraph's side margins apply in a table cell as they do on the page.
// The cell formatted a plain paragraph on its own, which applies padding
// but not margins, so the text sat on the cell's edge. Its vertical margins
// stay as they were in a cell, unapplied.
func TestCellParagraphKeepsItsSideMargins(t *testing.T) {
	cell := func(p string) string {
		return `<table style="width:200pt"><tr><td style="padding:0">` + p + `</td></tr></table>`
	}
	extent := func(html string) (bag.ScaledPoint, bag.ScaledPoint) {
		vl := buildHTML(t, floatBuilder(t), html)
		first, last, ok := glyphExtent(vl.List, 0, false)
		if !ok {
			t.Fatalf("no glyphs in %s", html)
		}
		return first, last
	}
	height := func(html string) bag.ScaledPoint {
		vl := buildHTML(t, floatBuilder(t), html)
		return vl.Height + vl.Depth
	}
	margin := bag.MustSP("20pt")

	plainL, _ := extent(cell(`<p>Indented</p>`))
	gotL, _ := extent(cell(`<p style="margin-left:20pt">Indented</p>`))
	if gotL-plainL != margin {
		t.Errorf("margin-left moved the text by %s, want %s", gotL-plainL, margin)
	}

	_, plainR := extent(cell(`<p style="text-align:right">Indented</p>`))
	_, gotR := extent(cell(`<p style="text-align:right;margin-right:20pt">Indented</p>`))
	if plainR-gotR != margin {
		t.Errorf("margin-right moved the text by %s, want %s", plainR-gotR, margin)
	}

	plain := height(cell(`<p style="margin:10pt 0">A</p><p style="margin:10pt 0">B</p>`))
	got := height(cell(`<p style="margin:10pt 20pt">A</p><p style="margin:10pt 20pt">B</p>`))
	if got != plain {
		t.Errorf("side margins changed the cell's height from %s to %s", plain, got)
	}
}
