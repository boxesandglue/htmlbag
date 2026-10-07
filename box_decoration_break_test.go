package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// textLines is placedLines with the lines inside a box with a border or
// background (an HList around a VList) counted one by one.
func textLines(pages []*document.Page) []placedLine {
	var out []placedLine
	var walk func(n node.Node, y bag.ScaledPoint, pg int)
	walk = func(n node.Node, y bag.ScaledPoint, pg int) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walk(v.List, y, pg)
				y -= v.Height + v.Depth
			case *node.HList:
				boxed := false
				for c := v.List; c != nil; c = c.Next() {
					if vl, ok := c.(*node.VList); ok {
						// Its baseline sits on the HList's.
						walk(vl.List, y-v.Height+vl.Height, pg)
						boxed = true
					}
				}
				if !boxed {
					var sb strings.Builder
					collectGlyphs(v.List, &sb)
					if sb.Len() > 0 {
						out = append(out, placedLine{pg, y, y - v.Height - v.Depth, v.Width, sb.String()})
					}
				}
				y -= v.Height + v.Depth
			case *node.Kern:
				y -= v.Kern
			case *node.Glue:
				y -= v.Width
			}
		}
	}
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist, obj.Y, i+1)
			}
		}
	}
	return out
}

// fragGaps is the space between a fragment's edges and its first and last
// line: the padding and border it has on each side.
type fragGaps struct {
	lines       int
	top, bottom bag.ScaledPoint
}

func (g fragGaps) String() string {
	return fmt.Sprintf("%d lines, %s above, %s below", g.lines, g.top, g.bottom)
}

// pageFragGaps is fragGaps for each page holding nothing but a fragment.
func pageFragGaps(t *testing.T, css, body string) []fragGaps {
	t.Helper()
	pages := renderHTMLPages(t, css, body)
	lines := textLines(pages)
	var gaps []fragGaps
	for i, pg := range pages {
		var on []placedLine
		for _, l := range lines {
			if l.page == i+1 {
				on = append(on, l)
			}
		}
		if len(on) == 0 {
			continue
		}
		bottom := charTop - flowDepth(pg, charTop)
		gaps = append(gaps, fragGaps{len(on), charTop - on[0].top, on[len(on)-1].bottom - bottom})
	}
	return gaps
}

// regionFragGaps is fragGaps for each region FlowText fills.
func regionFragGaps(t *testing.T, body string) []fragGaps {
	t.Helper()
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, body, wide("160pt"))
	var gaps []fragGaps
	for _, f := range tr.filled {
		lines := textLines([]*document.Page{{Objects: []document.Object{{Vlist: f.Box}}}})
		for i := range lines {
			lines[i].top, lines[i].bottom = -lines[i].top, -lines[i].bottom
		}
		if len(lines) == 0 {
			continue
		}
		h := f.Box.Height + f.Box.Depth
		gaps = append(gaps, fragGaps{len(lines), lines[0].top, h - lines[len(lines)-1].bottom})
	}
	return gaps
}

// A block split across pages under box-decoration-break: clone keeps its
// padding and border on both sides of every fragment, and leaves room for
// them on each page; under slice (the initial value) the first fragment has
// only its top side and the last only its bottom. 20 lines of 12pt in a
// 160pt content area, 10pt of padding and border on each side.
func TestBoxDecorationBreakClone(t *testing.T) {
	ten := sp("10pt")
	clone := []fragGaps{{11, ten, ten}, {9, ten, ten}}
	slice := []fragGaps{{12, ten, 0}, {8, 0, ten}}
	const border = "border: 2pt solid black; padding: 8pt"
	cases := []struct {
		name, style string
		want        []fragGaps
	}{
		{"clone", border + "; box-decoration-break: clone", clone},
		{"slice", border + "; box-decoration-break: slice", slice},
		{"initial", border, slice},
		{"background", "background-color: #eee; padding: 10pt; box-decoration-break: clone", clone},
		{"padding only", "padding: 10pt; box-decoration-break: clone", clone},
		{"padding only, slice", "padding: 10pt", slice},
	}
	shapes := []struct{ name, open, close string }{
		{"paragraph", `<p style="%s">`, `</p>`},
		{"container", `<div style="%s"><p>`, `</p></div>`},
	}
	for _, shape := range shapes {
		for _, c := range cases {
			body := fmt.Sprintf(shape.open, c.style) + charLines("A", 20) + shape.close
			if shape.name == "container" {
				body = fmt.Sprintf(`<div style="%s"><p>`, c.style) + charLines("A", 15) + `</p><p>` + charLines("B", 5) + `</p></div>`
			}
			t.Run(shape.name+"/"+c.name+"/pages", func(t *testing.T) {
				checkFragGaps(t, pageFragGaps(t, charCSS, body), c.want)
			})
			t.Run(shape.name+"/"+c.name+"/FlowText", func(t *testing.T) {
				checkFragGaps(t, regionFragGaps(t, body), c.want)
			})
		}
	}
}

func checkFragGaps(t *testing.T, got, want []fragGaps) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fragments %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fragment %d: %v, want %v", i+1, got[i], want[i])
		}
	}
}

// A block with clone inside a container that splits is cut through
// (planSplit and cutBlock): the part before the cut keeps its bottom side,
// the rest its top side.
func TestBoxDecorationBreakCloneNested(t *testing.T) {
	ten := sp("10pt")
	body := `<p>Intro</p><div><p style="border: 2pt solid black; padding: 8pt; box-decoration-break: clone">` + charLines("A", 20) + `</p></div>`
	lines := textLines(renderHTMLPages(t, charCSS, body))
	var lastOn1, firstOn2 placedLine
	for _, l := range lines {
		if l.page == 1 {
			lastOn1 = l
		} else if l.page == 2 && firstOn2.page == 0 {
			firstOn2 = l
		}
	}
	// The content area ends at 20pt; the fragment's bottom side goes above it.
	if room := lastOn1.bottom - sp("20pt"); room < ten {
		t.Errorf("the last line on page 1 is %s above the page end, want at least the 10pt bottom side", room)
	}
	if got := charTop - firstOn2.top; got != ten {
		t.Errorf("the first line on page 2 is %s below the page top, want the 10pt top side", got)
	}
}

// A table is not cloned: its rows split in outputTableRows.
func TestBoxDecorationBreakCloneNoTable(t *testing.T) {
	var rows string
	for i := range 20 {
		rows += fmt.Sprintf(`<tr><td>R%d</td></tr>`, i)
	}
	plain := placedLines(renderHTMLPages(t, charCSS, `<table style="border: 2pt solid black">`+rows+`</table>`))
	clone := placedLines(renderHTMLPages(t, charCSS, `<table style="border: 2pt solid black; box-decoration-break: clone">`+rows+`</table>`))
	if fmt.Sprint(plain) != fmt.Sprint(clone) {
		t.Errorf("the table changes with box-decoration-break: clone")
	}
}
