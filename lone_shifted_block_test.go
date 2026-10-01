package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// lineStarts lists the x of the first glyph of each line in vl, measured from
// vl's left edge, with the text of the line.
func lineStarts(vl *node.VList) (xs []bag.ScaledPoint, texts []string) {
	var walkV func(n node.Node, x bag.ScaledPoint)
	// firstGlyph returns the x of the first glyph in n, and whether there is one.
	var firstGlyph func(n node.Node, x bag.ScaledPoint) (bag.ScaledPoint, bool)
	firstGlyph = func(n node.Node, x bag.ScaledPoint) (bag.ScaledPoint, bool) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.Glyph:
				return x, true
			case *node.HList:
				if gx, ok := firstGlyph(v.List, x+v.ShiftX); ok {
					return gx, true
				}
				x += v.Width
			case *node.Kern:
				x += v.Kern
			case *node.Glue:
				x += v.Width
			}
		}
		return x, false
	}
	walkV = func(n node.Node, x bag.ScaledPoint) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walkV(v.List, x+v.ShiftX)
			case *node.HList:
				if gx, ok := firstGlyph(v.List, x+v.ShiftX); ok {
					var sb strings.Builder
					collectGlyphs(v.List, &sb)
					xs, texts = append(xs, gx), append(texts, sb.String())
				}
			}
		}
	}
	walkV(vl.List, vl.ShiftX)
	return xs, texts
}

// A block shifted by margin-left keeps the shift when it is the only block
// of the flow, as it does among other blocks (CSS 2.1 §8.3): the lines of a
// hanging paragraph start at margin-left, its first line at margin-left plus
// the negative text-indent (CSS 2.1 §16.1), inside the region.
func TestFlowTextLoneShiftedBlock(t *testing.T) {
	const css = `.hang { margin-left: 60pt; text-indent: -50pt }`
	para := `<p class="hang">(a) ` + strings.Repeat("alpha beta gamma ", 6) + `</p>`
	first, rest := sp("10pt"), sp("60pt")
	for _, c := range []struct{ name, body string }{
		{"only block", para},
		{"after a block", `<p>Before</p>` + para},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, css)
			tr := flow(t, cb, c.body, wide("1000pt"))
			if len(tr.filled) != 1 {
				t.Fatalf("filled %d regions, want 1", len(tr.filled))
			}
			xs, texts := lineStarts(tr.filled[0].Box)
			i := 0
			for i < len(texts) && !strings.HasPrefix(texts[i], "(a)") {
				i++
			}
			if len(texts)-i < 2 {
				t.Fatalf("lines %q, want the hanging paragraph's two or more", texts)
			}
			if xs[i] != first {
				t.Errorf("first line starts at %s, want %s", xs[i], first)
			}
			for j := i + 1; j < len(xs); j++ {
				if xs[j] != rest {
					t.Errorf("line %d starts at %s, want %s", j-i+1, xs[j], rest)
				}
			}
		})
	}
}

// The body's only child on a page takes the same unwrap.
func TestLoneShiftedBlockOnAPage(t *testing.T) {
	css := charCSS + `.hang { margin-left: 60pt; text-indent: -50pt }`
	pages := renderHTMLPages(t, css, `<p class="hang">(a) `+strings.Repeat("alpha beta gamma ", 6)+`</p>`)
	var xs []bag.ScaledPoint
	for _, obj := range pages[0].Objects {
		if obj.Vlist != nil {
			x, _ := lineStarts(obj.Vlist)
			for _, v := range x {
				xs = append(xs, obj.X+v)
			}
		}
	}
	if len(xs) < 2 {
		t.Fatalf("%d lines, want two or more", len(xs))
	}
	// The page's left margin is 20pt.
	if xs[0] != sp("30pt") {
		t.Errorf("first line starts at %s, want 30pt", xs[0])
	}
	for j, x := range xs[1:] {
		if x != sp("80pt") {
			t.Errorf("line %d starts at %s, want 80pt", j+2, x)
		}
	}
}

// numberedLines is n short lines, one per <br>.
func numberedLines(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			sb.WriteString("<br>")
		}
		sb.WriteString("line")
	}
	return sb.String()
}

// pageLineStarts lists the x of every line on each page.
func pageLineStarts(pages []*document.Page) [][]bag.ScaledPoint {
	out := make([][]bag.ScaledPoint, len(pages))
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				xs, _ := lineStarts(obj.Vlist)
				for _, x := range xs {
					out[i] = append(out[i], obj.X+x)
				}
			}
		}
	}
	return out
}

// A tall shifted box with one child is not _splittable; its shift moves onto
// the child, which still splits across pages.
func TestLoneShiftedBlockSplits(t *testing.T) {
	pages := renderHTMLPages(t, charCSS, `<div style="margin-left:30pt"><p>`+numberedLines(80)+`</p></div>`)
	if len(pages) != 7 {
		t.Fatalf("%d pages, want 7", len(pages))
	}
	n := 0
	for i, xs := range pageLineStarts(pages) {
		for _, x := range xs {
			n++
			if x != sp("50pt") {
				t.Errorf("page %d: line starts at %s, want 50pt", i+1, x)
			}
		}
	}
	if n != 80 {
		t.Errorf("%d lines, want 80", n)
	}
}

// A shifted container with several children splits between and inside them,
// and each heading keeps its own page.
func TestLoneShiftedContainerSplits(t *testing.T) {
	html := `<div style="margin-left:30pt"><h1>A</h1><p>` + numberedLines(40) +
		`</p><h1>B</h1><p>` + numberedLines(40) + `</p><h1>C</h1><p>x</p></div>`
	pages, cb := renderHTMLPagesCB(t, charCSS, html)
	if len(pages) != 7 {
		t.Fatalf("%d pages, want 7", len(pages))
	}
	var got []int
	for _, h := range cb.Headings {
		got = append(got, h.Page)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 4 || got[2] != 7 {
		t.Errorf("headings on pages %v, want [1 4 7]", got)
	}
	for i, xs := range pageLineStarts(pages) {
		for _, x := range xs {
			if x != sp("50pt") {
				t.Errorf("page %d: line starts at %s, want 50pt", i+1, x)
			}
		}
	}
}
