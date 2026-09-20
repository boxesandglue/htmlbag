package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// tableRowHeights returns the heights of the table rows on the pages. Rows
// are HLists tagged "table row"; their height is the row height of the
// table layout.
func tableRowHeights(t *testing.T, pages []*document.Page) []bag.ScaledPoint {
	t.Helper()
	var heights []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin == "table row" {
					heights = append(heights, v.Height)
					continue
				}
				walk(v.List)
			case *node.VList:
				walk(v.List)
			}
		}
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist)
			}
		}
	}
	if len(heights) == 0 {
		t.Fatal("no table row found on any page")
	}
	return heights
}

// CSS height on a <tr> or a <td> is the minimum height of the row (CSS 2.1
// §17.5.3). Content taller than the declared height keeps its size.
func TestTableRowHeightIsMinimum(t *testing.T) {
	css := cellWidthCSS
	html := `<table>
<tr style="height: 3cm"><td>row height</td></tr>
<tr><td style="height: 2cm">cell height</td></tr>
<tr style="height: 1pt"><td>too small</td></tr>
<tr><td>natural</td></tr>
</table>`
	heights := tableRowHeights(t, renderHTMLPages(t, css, html))
	if len(heights) != 4 {
		t.Fatalf("got %d rows, want 4", len(heights))
	}
	natural := heights[3]
	if natural <= 0 || natural >= bag.MustSP("1cm") {
		t.Fatalf("natural row height %s is not a single text line", natural)
	}
	if !closeTo(heights[0], bag.MustSP("3cm")) {
		t.Errorf("tr height: row is %s, want 3cm", heights[0])
	}
	if !closeTo(heights[1], bag.MustSP("2cm")) {
		t.Errorf("td height: row is %s, want 2cm", heights[1])
	}
	if heights[2] != natural {
		t.Errorf("tr height smaller than the content: row is %s, want the natural %s", heights[2], natural)
	}
}
