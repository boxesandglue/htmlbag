package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// glyphXs lists the x of the first glyph of every line and table row in n,
// from n's left edge.
func glyphXs(n node.Node) []bag.ScaledPoint {
	var xs []bag.ScaledPoint
	var inH, inV func(n node.Node, x bag.ScaledPoint) (bag.ScaledPoint, bool)
	inH = func(n node.Node, x bag.ScaledPoint) (bag.ScaledPoint, bool) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.Glyph:
				return x, true
			case *node.HList:
				if gx, ok := inH(v.List, x+v.ShiftX); ok {
					return gx, true
				}
				x += v.Width
			case *node.VList:
				if gx, ok := inV(v.List, x+v.ShiftX); ok {
					return gx, true
				}
				x += v.Width
			case *node.Kern:
				x += v.Kern
			case *node.Glue:
				x += v.Width
			}
		}
		return 0, false
	}
	inV = func(n node.Node, x bag.ScaledPoint) (bag.ScaledPoint, bool) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if gx, ok := inH(v.List, x+v.ShiftX); ok {
					return gx, true
				}
			case *node.VList:
				if gx, ok := inV(v.List, x+v.ShiftX); ok {
					return gx, true
				}
			}
		}
		return 0, false
	}
	var walk func(n node.Node, x bag.ScaledPoint)
	walk = func(n node.Node, x bag.ScaledPoint) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walk(v.List, x+v.ShiftX)
			case *node.HList:
				if gx, ok := inH(v.List, x+v.ShiftX); ok {
					xs = append(xs, gx)
				}
			}
		}
	}
	if vl, ok := n.(*node.VList); ok {
		walk(vl.List, vl.ShiftX)
	}
	return xs
}

// marginTable is a one-column table of n rows "r1" … "rn" with margin-left
// ml, and the extra style and leading markup.
func marginTable(ml, style, head string, n int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<table style="margin-left: %s; width: 100pt; %s">%s`, ml, style, head)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, `<tr><td>r%d</td></tr>`, i)
	}
	sb.WriteString(`</table>`)
	return sb.String()
}

// pageGlyphXs is glyphXs of every object on every page, from the page's
// left edge.
func pageGlyphXs(t *testing.T, html string) [][]bag.ScaledPoint {
	t.Helper()
	pages := renderHTMLPages(t, charCSS, html)
	out := make([][]bag.ScaledPoint, len(pages))
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist == nil {
				continue
			}
			for _, x := range glyphXs(obj.Vlist) {
				out[i] = append(out[i], obj.X+x)
			}
		}
	}
	return out
}

// A table's margin-left moves it as it moves any block (CSS 2.1 §8.3), a
// negative one to the left of the content edge: on a page and in a region,
// placed whole or row by row across a break.
func TestTableMarginLeft(t *testing.T) {
	for _, ml := range []string{"30pt", "-10pt"} {
		shift := sp(ml)
		for _, c := range []struct {
			name, style, head string
			rows              int
		}{
			{"whole", "", "", 2},
			{"split by rows", "", "", 30},
			{"bordered", "border: 1pt solid black", "", 2},
			{"with a header split by rows", "", "<thead><tr><td>hd</td></tr></thead>", 30},
		} {
			t.Run(ml+" "+c.name+" on a page", func(t *testing.T) {
				want := pageGlyphXs(t, `<p>Aq</p>`+marginTable("0pt", c.style, c.head, c.rows))
				got := pageGlyphXs(t, `<p>Aq</p>`+marginTable(ml, c.style, c.head, c.rows))
				comparePlacedXs(t, got, want, shift)
			})
			t.Run(ml+" "+c.name+" in a region", func(t *testing.T) {
				regionXs := func(ml string) [][]bag.ScaledPoint {
					cb, _ := newFlowBuilder(t, "")
					tr := flow(t, cb, `<p>Aq</p>`+marginTable(ml, c.style, c.head, c.rows), wide("120pt"))
					out := make([][]bag.ScaledPoint, len(tr.filled))
					for i, f := range tr.filled {
						out[i] = glyphXs(f.Box)
					}
					return out
				}
				comparePlacedXs(t, regionXs(ml), regionXs("0pt"), shift)
			})
		}
	}
}

// comparePlacedXs checks that every row but the first, a paragraph on the
// first page, starts shift further right in got than in want.
func comparePlacedXs(t *testing.T, got, want [][]bag.ScaledPoint, shift bag.ScaledPoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d pages, want %d", len(got), len(want))
	}
	rows := 0
	for p := range got {
		if len(got[p]) != len(want[p]) {
			t.Fatalf("page %d: %d rows, want %d", p+1, len(got[p]), len(want[p]))
		}
		for i := range got[p] {
			if p == 0 && i == 0 {
				if got[p][i] != want[p][i] {
					t.Errorf("the paragraph moved from %s to %s", want[p][i], got[p][i])
				}
				continue
			}
			rows++
			if d := got[p][i] - want[p][i]; d != shift {
				t.Errorf("page %d row %d moved by %s, want %s", p+1, i, d, shift)
			}
		}
	}
	if rows == 0 {
		t.Fatal("no table rows found")
	}
}

// The border of a bordered table moves with it: the shift is on the border
// box (100pt and two 1pt borders), not on the table inside it.
func TestTableMarginLeftMovesTheBorder(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<p>Aq</p>`+marginTable("30pt", "border: 1pt solid black", "", 2), wide("120pt"))
	var shifted []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			if v, ok := n.(*node.VList); ok {
				if v.ShiftX != 0 {
					shifted = append(shifted, v.Width)
				}
				walk(v.List)
			}
		}
	}
	walk(tr.filled[0].Box.List)
	if len(shifted) != 1 || shifted[0] != sp("102pt") {
		t.Errorf("widths of the shifted boxes %v, want one of 102pt, the border box", shifted)
	}
}

// A table without a width takes the room its margins leave, so a shifted one
// still ends at the content edge, or before it by its margin-right.
func TestTableMarginsNarrowAnAutoWidthTable(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<table style="margin-left: 30pt; margin-right: 10pt"><tr><td>`+strings.Repeat("alpha beta ", 20)+`</td></tr></table>`, wide("1000pt"))
	var right bag.ScaledPoint
	var walk func(n node.Node, x bag.ScaledPoint)
	walk = func(n node.Node, x bag.ScaledPoint) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				right = max(right, x+v.ShiftX+v.Width)
				walk(v.List, x+v.ShiftX)
			case *node.HList:
				right = max(right, x+v.ShiftX+v.Width)
			}
		}
	}
	walk(tr.filled[0].Box.List, 0)
	if want := sp("150pt"); right != want {
		t.Errorf("the table ends at %s, want %s (160pt less the 10pt margin-right)", right, want)
	}
}

// A table rebuilt for a page of another width keeps its margin-left.
func TestTableMarginLeftAfterAWidthChange(t *testing.T) {
	css := charCSS + `@page :first { margin-left: 40pt }`
	xs := func(ml string) [][]bag.ScaledPoint {
		pages := renderHTMLPages(t, css, `<p>Aq</p>`+marginTable(ml, "", "<thead><tr><td>hd</td></tr></thead>", 30))
		out := make([][]bag.ScaledPoint, len(pages))
		for i, pg := range pages {
			for _, obj := range pg.Objects {
				if obj.Vlist != nil {
					for _, x := range glyphXs(obj.Vlist) {
						out[i] = append(out[i], obj.X+x)
					}
				}
			}
		}
		return out
	}
	got := xs("30pt")
	if len(got) < 2 {
		t.Fatalf("%d pages, want the table split over two or more", len(got))
	}
	comparePlacedXs(t, got, xs("0pt"), sp("30pt"))
}

// The rows of a table spliced into the flow keep the shift of the blocks
// around it, as its paragraphs do.
func TestSplicedTableRowsKeepAnAncestorsShift(t *testing.T) {
	doc := func(ml string) string {
		return `<div style="margin-left: ` + ml + `"><div><p>Aq</p>` + marginTable("0pt", "", "", 30) + `</div></div>`
	}
	got, want := pageGlyphXs(t, doc("30pt")), pageGlyphXs(t, doc("0pt"))
	if len(got) != len(want) || len(got) < 2 {
		t.Fatalf("%d pages, want %d and two or more", len(got), len(want))
	}
	for p := range got {
		if len(got[p]) != len(want[p]) {
			t.Fatalf("page %d: %d lines, want %d", p+1, len(got[p]), len(want[p]))
		}
		for i := range got[p] {
			if d := got[p][i] - want[p][i]; d != sp("30pt") {
				t.Errorf("page %d line %d moved by %s, want 30pt", p+1, i+1, d)
			}
		}
	}
}
