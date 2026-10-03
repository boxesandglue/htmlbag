package htmlbag

import (
	"math"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// boxX returns the left edge, from the content area's, and the width of the
// box that carries id on the pages.
func boxX(t *testing.T, pages []*document.Page, id string) (bag.ScaledPoint, bag.ScaledPoint) {
	t.Helper()
	var x, wd bag.ScaledPoint
	found := false
	var walk func(n node.Node, left bag.ScaledPoint)
	walk = func(n node.Node, left bag.ScaledPoint) {
		for ; n != nil && !found; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				l := left + v.ShiftX
				if got, _ := v.GetAttribute("id"); got == id {
					x, wd, found = l, v.Width, true
					return
				}
				walk(v.List, l)
			case *node.HList:
				if got, _ := v.GetAttribute("id"); got == id {
					x, wd, found = left, v.Width, true
					return
				}
			}
		}
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil && !found {
				walk(obj.Vlist, obj.X-bag.MustSP("20pt"))
			}
		}
	}
	if !found {
		t.Fatalf("no box with id %q", id)
	}
	return x, wd
}

// margin: auto (#58): auto side margins share the room a block's width
// leaves, as in CSS 2.1 §10.3.3; without a width the block fills the line and
// auto is 0. Top and bottom auto are 0 (§10.6.3). The container is 160pt.
func TestMarginAuto(t *testing.T) {
	cases := []struct {
		name, html string
		x, wd      string
	}{
		{"margin-left auto, no width", `<p id="x" style="margin-left: auto">A</p>`, "0pt", "160pt"},
		{"margin-left auto, larger font", `<p id="x" style="margin-left: auto; font-size: 20pt">A</p>`, "0pt", "160pt"},
		{"paragraph with a width, centered", `<p id="x" style="width: 80pt; margin: 0 auto">A</p>`, "40pt", "80pt"},
		{"paragraph with a width, margin-left auto", `<p id="x" style="width: 80pt; margin-left: auto">A</p>`, "80pt", "80pt"},
		{"paragraph with a width, margin-right auto", `<p id="x" style="width: 80pt; margin-right: auto">A</p>`, "0pt", "80pt"},
		{"div with a width, centered", `<div style="width: 80pt; margin: 0 auto"><p id="x">A</p><p>B</p></div>`, "40pt", "80pt"},
		{"table with a width, centered", `<table id="x" style="width: 80pt; margin: 0 auto"><tr><td>A</td></tr></table>`, "40pt", "80pt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pages := renderHTMLPages(t, charCSS, c.html)
			x, wd := boxX(t, pages, "x")
			if x != sp(c.x) || wd != sp(c.wd) {
				t.Errorf("box at %s, %s wide; want %s, %s wide", x, wd, c.x, c.wd)
			}
		})
	}
	t.Run("margin-top auto", func(t *testing.T) {
		lines := placedLines(renderHTMLPages(t, charCSS, `<p>A</p><p style="margin-top: auto">B</p>`))
		a, b := firstLine(t, lines, "A"), firstLine(t, lines, "B")
		if a.bottom != b.top {
			t.Errorf("gap of %s between A and B, want none", a.bottom-b.top)
		}
	})
}

// A table without a width is as wide as its content (shrink-to-fit), and its
// auto margins place it by that width. The numbers are Chromium's for the same
// HTML and font (Crimson Pro, 10pt, a 160pt container), within 0.02pt.
func TestMarginAutoShrinkToFitTable(t *testing.T) {
	css := charCSS + `body { font-family: serif } table { border-collapse: collapse } td { padding: 0 }`
	cases := []struct {
		html  string
		x, wd float64
	}{
		{`<table id="x" style="margin: 0 auto"><tr><td>Aq</td><td>Shrink to fit</td></tr></table>`, 51.11, 57.77},
		{`<table id="x" style="margin-left: auto"><tr><td>Aq</td><td>Shrink to fit</td></tr></table>`, 102.22, 57.77},
		{`<table id="x" style="margin: 0 auto"><tr><td>Short</td></tr><tr><td>A longer second row</td></tr></table>`, 39.68, 80.64},
	}
	for _, c := range cases {
		x, wd := boxX(t, renderHTMLPages(t, css, c.html), "x")
		if math.Abs(x.ToPT()-c.x) > 0.02 || math.Abs(wd.ToPT()-c.wd) > 0.02 {
			t.Errorf("%s: at %.2f, %.2f wide; want %.2f, %.2f wide", c.html, x.ToPT(), wd.ToPT(), c.x, c.wd)
		}
	}
}

// lineLefts lists the left edge of every line and row in a region's box.
func lineLefts(f Filled) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(n node.Node, left bag.ScaledPoint)
	walk = func(n node.Node, left bag.ScaledPoint) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walk(v.List, left+v.ShiftX)
			case *node.HList:
				out = append(out, left)
			}
		}
	}
	walk(f.Box, 0)
	return out
}

// In FlowText regions too: a centered table that splits keeps its place in
// every region, the repeated header included.
func TestMarginAutoInRegions(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	body := `<table style="width: 80pt; margin: 0 auto"><thead><tr><th>H</th></tr></thead><tbody>` + strings.Repeat(`<tr><td>R</td></tr>`, 12) + `</tbody></table>`
	tr := flow(t, cb, body, wide("60pt"), wide("1000pt"))
	if len(tr.filled) != 2 {
		t.Fatalf("filled %d regions, want 2", len(tr.filled))
	}
	for i, f := range tr.filled {
		for _, l := range lineLefts(f) {
			if l != sp("40pt") {
				t.Errorf("region %d has a row at %s, want 40pt", i+1, l)
			}
		}
	}
}

// A pre-rendered box (data-vlist-id) has its own width, by which auto side
// margins place it.
func TestMarginAutoPlaceholderBox(t *testing.T) {
	for _, c := range []struct{ style, shift string }{
		{"margin: 0 auto", "60pt"},
		{"margin-left: auto", "120pt"},
		{"margin-right: auto", "0pt"},
	} {
		pages, _ := placeholderPages(t, "", `<div data-vlist-id="b" style="`+c.style+`"></div>`)
		if got := placeholderShifts(pageLists(pages)...); len(got) != 1 || got[0] != sp(c.shift) {
			t.Errorf("%s: the box is shifted by %v, want %s", c.style, got, c.shift)
		}
	}
}
