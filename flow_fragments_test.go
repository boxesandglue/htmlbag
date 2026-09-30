package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// boxIDs counts the id attributes of the boxes in a region's box.
func boxIDs(f Filled) map[string]int {
	return elementIDs([]*document.Page{{Objects: []document.Object{{Vlist: f.Box}}}})
}

func fragmentsString(frs []Fragment) string {
	var s []string
	for _, fr := range frs {
		s = append(s, fmt.Sprintf("{%q %d %s+%s continued=%v continues=%v}", fr.ID, fr.Index, fr.Top, fr.Height, fr.Continued, fr.Continues))
	}
	return strings.Join(s, " ")
}

func checkFragments(t *testing.T, tr *testRegions, want [][]Fragment) {
	t.Helper()
	if len(tr.filled) != len(want) {
		t.Fatalf("filled %d regions, want %d", len(tr.filled), len(want))
	}
	for i, f := range tr.filled {
		if got, w := fragmentsString(f.Fragments), fragmentsString(want[i]); got != w {
			t.Errorf("region %d fragments\n got %s\nwant %s", i+1, got, w)
		}
	}
}

func TestFlowTextFragmentsOfASplitParagraph(t *testing.T) {
	cases := []struct {
		name, body string
		want       [][]Fragment
	}{
		{"after another block", `<p>` + charLines("A", 1) + `</p><p id="long">` + charLines("B", 10) + `</p>`, [][]Fragment{
			{{"", 0, 0, sp("12pt"), false, false}, {"long", 1, sp("12pt"), sp("48pt"), false, true}},
			{{"long", 1, 0, sp("72pt"), true, false}},
		}},
		{"the only block", `<p id="long">` + charLines("B", 11) + `</p>`, [][]Fragment{
			{{"long", 0, 0, sp("60pt"), false, true}},
			{{"long", 0, 0, sp("72pt"), true, false}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, c.body, wide("60pt"), wide("1000pt"))
			checkFragments(t, tr, c.want)
			for i, f := range tr.filled {
				if n := boxIDs(f)["long"]; n != 1 {
					t.Errorf("region %d: %d boxes carry the id, want 1", i+1, n)
				}
			}
		})
	}
}

func TestFlowTextFragmentsOfATableSplitByRows(t *testing.T) {
	var rows strings.Builder
	for i := range 8 {
		fmt.Fprintf(&rows, `<tr><td>R%dq</td></tr>`, i)
	}
	cases := map[string]string{
		"with a header":    `<table id="tbl"><thead><tr><th>Hq</th></tr></thead><tbody>` + rows.String() + `</tbody></table>`,
		"without a header": `<table id="tbl"><tbody>` + rows.String() + `</tbody></table>`,
	}
	for name, table := range cases {
		t.Run(name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, `<p>`+charLines("A", 1)+`</p>`+table, wide("72pt"), wide("1000pt"))
			if len(tr.filled) != 2 {
				t.Fatalf("filled %d regions, want 2", len(tr.filled))
			}
			// The table's fragment spans its rows in the region, found by
			// their text.
			extent := func(f Filled) (top, height bag.ScaledPoint) {
				first := true
				for _, l := range boxLines(f) {
					if !strings.Contains(l.text, "R") && !strings.Contains(l.text, "H") {
						continue
					}
					if first {
						top, first = l.top, false
					}
					height = l.bottom - top
				}
				return top, height
			}
			top1, h1 := extent(tr.filled[0])
			top2, h2 := extent(tr.filled[1])
			if top1 != charLine || top2 != 0 {
				t.Fatalf("rows start at %s and %s, want 12pt and 0pt", top1, top2)
			}
			checkFragments(t, tr, [][]Fragment{
				{{"", 0, 0, charLine, false, false}, {"tbl", 1, top1, h1, false, true}},
				{{"tbl", 1, top2, h2, true, false}},
			})
			for i, f := range tr.filled {
				if n := boxIDs(f)["tbl"]; n != 1 {
					t.Errorf("region %d: %d boxes carry the id, want 1", i+1, n)
				}
			}
		})
	}
}

// A paragraph over three regions continues from the first through the
// second into the third.
func TestFlowTextFragmentsOverThreeRegions(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<p id="long">`+charLines("B", 10)+`</p><p id="z">Zq</p>`, wide("48pt"), wide("48pt"), wide("1000pt"))
	checkFragments(t, tr, [][]Fragment{
		{{"long", 0, 0, sp("48pt"), false, true}},
		{{"long", 0, 0, sp("48pt"), true, true}},
		{{"long", 0, 0, sp("24pt"), true, false}, {"z", 1, sp("24pt"), charLine, false, false}},
	})
}

// A side float's fragment is as tall as the float paints, though its box
// reports no height.
func TestFlowTextFragmentOfASideFloat(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<div id="f" style="float: left; width: 40pt; height: 20pt">Fq</div><p id="g">`+charLines("G", 3)+`</p>`, wide("1000pt"))
	checkFragments(t, tr, [][]Fragment{
		{{"f", 0, 0, sp("20pt"), false, false}, {"g", 1, 0, sp("36pt"), false, false}},
	})
}

// On pages too, every fragment of a split paragraph carries its id.
func TestSplitParagraphFragmentsCarryTheID(t *testing.T) {
	pages := renderHTMLPages(t, charCSS, `<html><body><p>Aq</p><p id="long">`+charLines("B", 30)+`</p></body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	if n := elementIDs(pages)["long"]; n != 3 {
		t.Errorf("%d boxes carry the id, want one on each of the 3 pages", n)
	}
}

// idBoxWidths lists the widths of the boxes in a region's box that carry id.
func idBoxWidths(f Filled, id string) []bag.ScaledPoint {
	var ws []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				if got, _ := v.GetAttribute("id"); got == id {
					ws = append(ws, v.Width)
				}
				walk(v.List)
			case *node.HList:
				if got, _ := v.GetAttribute("id"); got == id {
					ws = append(ws, v.Width)
				}
				walk(v.List)
			}
		}
	}
	walk(f.Box)
	return ws
}

// The box that carries a table's id is as wide as the table, whole or split
// by rows, so a caller that walks the boxes finds it where it is drawn.
func TestFlowTextTableIDBoxIsTheTablesWidth(t *testing.T) {
	var rows strings.Builder
	for i := range 8 {
		fmt.Fprintf(&rows, `<tr><td>R%dq</td></tr>`, i)
	}
	for name, rows := range map[string]string{"whole": `<tr><td>Rq</td></tr>`, "split by rows": rows.String()} {
		t.Run(name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, `<p>Aq</p><table id="tbl" style="width: 80pt">`+rows+`</table>`, wide("72pt"), wide("1000pt"))
			for i, f := range tr.filled {
				ws := idBoxWidths(f, "tbl")
				if len(ws) != 1 || ws[0] != sp("80pt") {
					t.Errorf("region %d: the id is on boxes %v wide, want one of 80pt", i+1, ws)
				}
			}
		})
	}
}
