package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// A width in em is relative to the element's font size, a width in rem to
// the root font size (#101). The cell width took the table width as its em
// base, so 5em made the first column five times as wide as the table.
func TestCellWidthInEmIsRelativeToTheFontSize(t *testing.T) {
	cellCSS := colWidthCSS + `
html { font-size: 12pt }
td { font-size: 10pt }`
	for _, tc := range []struct{ em, pt string }{
		{"5em", "50pt"},
		{"5rem", "60pt"},
	} {
		em := firstRowCellWidths(t, renderHTMLPages(t, cellCSS,
			`<table><tr><td style="width:`+tc.em+`">A</td><td>B</td><td>C</td></tr></table>`))
		pt := firstRowCellWidths(t, renderHTMLPages(t, cellCSS,
			`<table><tr><td style="width:`+tc.pt+`">A</td><td>B</td><td>C</td></tr></table>`))
		if len(em) != 3 || len(pt) != 3 {
			t.Fatalf("%s: got %d and %d columns, want 3", tc.em, len(em), len(pt))
		}
		for i := range em {
			if !closeTo(em[i], pt[i]) {
				t.Errorf("%s: column %d is %s, want %s as with width:%s", tc.em, i, em[i], pt[i], tc.pt)
			}
		}
	}
}

// The table width and a block width in em had the same problem: the table
// ignored it, the block took the container width as its em base.
func TestTableAndBlockWidthInEm(t *testing.T) {
	widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS,
		`<table style="width:20em; font-size:10pt"><tr><td>one</td><td>two</td></tr></table>`))
	var sum bag.ScaledPoint
	for _, w := range widths {
		sum += w
	}
	if want := bag.MustSP("200pt"); !closeTo(sum, want) {
		t.Errorf("table columns add up to %s, want %s", sum, want)
	}

	vl := buildHTML(t, floatBuilder(t), `<div style="width:20em; font-size:10pt"><p>Text</p></div>`)
	var box *node.VList
	for n := vl.List; n != nil && box == nil; n = n.Next() {
		box, _ = n.(*node.VList)
	}
	if box == nil {
		t.Fatal("no box for the div")
	}
	if want := bag.MustSP("200pt"); !closeTo(box.Width, want) {
		t.Errorf("the div is %s wide, want %s", box.Width, want)
	}
}
