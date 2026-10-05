package htmlbag

import (
	"math"
	"path/filepath"
	"regexp"
	"strconv"
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
				// A box drawn with a border is an hlist of its parts.
				hx := left
				for c := v.List; c != nil && !found; c = c.Next() {
					if vl, ok := c.(*node.VList); ok {
						save := vl.Next()
						vl.SetNext(nil)
						walk(vl, hx)
						vl.SetNext(save)
					}
					w, _, _ := c.Sizes(node.Horizontal)
					hx += w
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

// A table sits in its container's content box: centered between the padding,
// with or without a border, also when it splits across pages.
func TestMarginAutoTableInAPaddedContainer(t *testing.T) {
	table := `<table id="x" style="width: 80pt; margin: 0 auto"><tr><td>A</td></tr></table>`
	for name, div := range map[string]string{
		"bare padding":       `<div style="padding-left: 40pt">`,
		"padding and border": `<div style="padding-left: 40pt; border-left: 0.5pt solid black">`,
	} {
		t.Run(name, func(t *testing.T) {
			x, wd := boxX(t, renderHTMLPages(t, charCSS, div+table+`</div>`), "x")
			// The content box is 40..160 without a border, 40.5..160 with one.
			want := sp("60pt")
			if strings.Contains(div, "border") {
				want = sp("60.25pt")
			}
			if x != want || wd != sp("80pt") {
				t.Errorf("table at %s, %s wide; want %s, 80pt wide", x, wd, want)
			}
		})
	}
	t.Run("split thead table", func(t *testing.T) {
		rows := strings.Repeat(`<tr><td>R</td></tr>`, 20)
		pages := renderHTMLPages(t, charCSS, `<div style="padding-left: 40pt"><table style="width: 80pt; margin: 0 auto"><thead><tr><th>H</th></tr></thead><tbody>`+rows+`</tbody></table></div>`)
		if len(pages) < 2 {
			t.Fatalf("got %d pages, want the table split", len(pages))
		}
		for i, pg := range pages {
			for _, obj := range pg.Objects {
				f := Filled{Box: obj.Vlist}
				for _, l := range lineLefts(f) {
					if x := obj.X - sp("20pt") + l; x != sp("60pt") {
						t.Errorf("page %d has a row at %s, want 60pt", i+1, x)
					}
				}
			}
		}
	})
}

// imageBox returns the left edge, from the content area's, the top, from the
// page's top, and the width of the first image in the PDF of html.
func imageBox(t *testing.T, css, html string) (x, top, wd float64) {
	t.Helper()
	pdf := renderLineModelPDF(t, css, html, func(cb *CSSBuilder) { cb.frontend.Doc.CompressLevel = 0 })
	m := regexp.MustCompile(`([\d.]+) 0 0 ([\d.]+) ([\d.]+) ([\d.]+) cm`).FindSubmatch(pdf)
	if m == nil {
		t.Fatal("no image in the PDF")
	}
	num := func(b []byte) float64 {
		f, err := strconv.ParseFloat(string(b), 64)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	return num(m[3]) - 20, 200 - num(m[4]) - num(m[2]), num(m[1])
}

// A block image is placed by its side margins, as a block with a width
// (CSS 2.1 §10.3.4), not by the text-align it inherits, and its margins and
// padding apply once (#72). The image is 40pt wide in a 160pt container.
func TestMarginAutoBlockImage(t *testing.T) {
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	css := charCSS + "\nimg { display: block; width: 40pt }"
	for _, c := range []struct {
		name, html string
		x          float64
	}{
		{"centered", `<img style="margin: 0 auto">`, 60},
		{"margin-left auto", `<img style="margin-left: auto">`, 120},
		{"margin-right auto", `<img style="margin-right: auto">`, 0},
		{"margin-left auto, margin-right 20pt", `<img style="margin-left: auto; margin-right: 20pt">`, 100},
		{"margin-left 20pt, margin-right auto", `<img style="margin-left: 20pt; margin-right: auto">`, 20},
		{"no auto margins", `<img>`, 0},
		{"margin-left 20pt", `<img style="margin-left: 20pt">`, 20},
		{"padding-left 10pt", `<img style="padding-left: 10pt">`, 10},
		{"no auto margins, text-align center", `<div style="text-align: center"><img></div>`, 0},
		{"centered, text-align right", `<div style="text-align: right"><img style="margin: 0 auto"></div>`, 60},
	} {
		t.Run(c.name, func(t *testing.T) {
			html := strings.ReplaceAll(c.html, "<img", `<img src="`+png+`"`)
			if x, _, _ := imageBox(t, css, html); math.Abs(x-c.x) > 0.01 {
				t.Errorf("the image is at %.2fpt, want %.0fpt", x, c.x)
			}
		})
	}
	t.Run("margin-top 20pt", func(t *testing.T) {
		_, top0, _ := imageBox(t, css, `<img src="`+png+`">`)
		_, top, _ := imageBox(t, css, `<img src="`+png+`" style="margin-top: 20pt">`)
		if d := top - top0; math.Abs(d-20) > 0.01 {
			t.Errorf("margin-top moves the image down by %.2fpt, want 20pt", d)
		}
	})
	t.Run("width 100%, margin-right 20pt", func(t *testing.T) {
		if _, _, wd := imageBox(t, css, `<img src="`+png+`" style="width: 100%; margin-right: 20pt">`); math.Abs(wd-140) > 0.01 {
			t.Errorf("the image is %.2fpt wide, want 140pt", wd)
		}
	})
}
