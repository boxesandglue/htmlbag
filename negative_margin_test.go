package htmlbag

import (
	"fmt"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// A negative margin-left moves a block left past its container's content
// edge (CSS 2.1 §8.3). In a table cell the block was widened by the margin
// but not shifted, so it overhung the cell on the right instead.
func TestNegativeMarginLeftShiftsLeft(t *testing.T) {
	extent := func(html string) (bag.ScaledPoint, bag.ScaledPoint) {
		vl := buildHTML(t, floatBuilder(t), html)
		first, last, ok := glyphExtent(vl.List, 0, false)
		if !ok {
			t.Fatalf("no glyphs in %s", html)
		}
		return first, last
	}
	margin := bag.MustSP("20pt")
	for _, tc := range []struct{ name, wrap string }{
		{"on the page", `<div style="padding-left:40pt">%s</div>`},
		{"in a table cell", `<table style="width:200pt"><tr><td style="padding:0 0 0 40pt">%s</td></tr></table>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrap := func(p string) string { return fmt.Sprintf(tc.wrap, p) }
			plainL, _ := extent(wrap(`<p>Text</p>`))
			gotL, _ := extent(wrap(`<p style="margin-left:-20pt">Text</p>`))
			if plainL-gotL != margin {
				t.Errorf("margin-left:-20pt moved the text left by %s, want %s", plainL-gotL, margin)
			}
			_, plainR := extent(wrap(`<p style="text-align:right">Text</p>`))
			_, gotR := extent(wrap(`<p style="text-align:right;margin-left:-20pt">Text</p>`))
			if gotR != plainR {
				t.Errorf("margin-left:-20pt moved the right edge from %s to %s", plainR, gotR)
			}
		})
	}
}
