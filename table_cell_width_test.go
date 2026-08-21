package htmlbag

import (
	"math"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

const cellWidthCSS = `@page { size: a4; margin: 2cm }
body { font-family: serif; font-size: 11pt }
td, th { padding: 0 }`

// firstRowCellWidths returns the calculated widths of the cells in the first
// table row. Rows are HLists tagged "table row"; their cell children are
// VLists tagged "td".
func firstRowCellWidths(t *testing.T, pages []*document.Page) []bag.ScaledPoint {
	t.Helper()
	var widths []bag.ScaledPoint
	var walk func(n node.Node) bool
	walk = func(n node.Node) bool {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if origin, _ := v.Attributes["origin"].(string); origin == "table row" {
					for c := v.List; c != nil; c = c.Next() {
						cell, ok := c.(*node.VList)
						if !ok {
							continue
						}
						if o, _ := cell.Attributes["origin"].(string); o == "td" {
							widths = append(widths, cell.Width)
						}
					}
					return len(widths) > 0
				}
				if walk(v.List) {
					return true
				}
			case *node.VList:
				if walk(v.List) {
					return true
				}
			}
		}
		return false
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil && walk(obj.Vlist) {
				return widths
			}
		}
	}
	t.Fatal("no table row found on any page")
	return nil
}

// contentWidth is the A4 width minus the 2cm margins declared in cellWidthCSS.
func contentWidth(t *testing.T) bag.ScaledPoint {
	t.Helper()
	return bag.MustSP("210mm") - 2*bag.MustSP("2cm")
}

func closeTo(got, want bag.ScaledPoint) bool {
	// One point of slack absorbs rounding in the percentage arithmetic.
	return math.Abs(float64(got-want)) < float64(bag.MustSP("1pt"))
}

// A CSS width on a <td> must set its column's share of the table, instead of
// being ignored so the columns shrink to their content.
func TestTableCellPercentWidth(t *testing.T) {
	cases := []struct {
		name string
		html string
		want []float64 // fraction of the content width per column
	}{
		{
			name: "fiftyFifty",
			html: `<table><tr><td style="width:50%">L</td><td style="width:50%">R</td></tr></table>`,
			want: []float64{0.5, 0.5},
		},
		{
			name: "quarterThreeQuarters",
			html: `<table><tr><td style="width:25%">L</td><td style="width:75%">R</td></tr></table>`,
			want: []float64{0.25, 0.75},
		},
		{
			name: "threeColumns",
			html: `<table><tr><td style="width:20%">A</td><td style="width:30%">B</td><td style="width:50%">C</td></tr></table>`,
			want: []float64{0.2, 0.3, 0.5},
		},
		{
			name: "widthOnHeaderRow",
			html: `<table><tr><th style="width:50%">H1</th><th style="width:50%">H2</th></tr><tr><td>x</td><td>y</td></tr></table>`,
			want: []float64{0.5, 0.5},
		},
	}
	cw := contentWidth(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS, tc.html))
			if len(widths) != len(tc.want) {
				t.Fatalf("got %d columns, want %d", len(widths), len(tc.want))
			}
			for i, frac := range tc.want {
				want := bag.ScaledPoint(float64(cw) * frac)
				if !closeTo(widths[i], want) {
					t.Errorf("column %d is %s, want %s (%.0f%% of %s)",
						i, widths[i], want, frac*100, cw)
				}
			}
		})
	}
}

// An absolute width is honoured just like a percentage.
func TestTableCellAbsoluteWidth(t *testing.T) {
	html := `<table><tr><td style="width:4cm">L</td><td>Rechts</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS, html))
	if len(widths) != 2 {
		t.Fatalf("got %d columns, want 2", len(widths))
	}
	if want := bag.MustSP("4cm"); !closeTo(widths[0], want) {
		t.Errorf("first column is %s, want %s", widths[0], want)
	}
}

// The declared widths must not push the table past its maximum width, even
// when they add up to more than 100%.
func TestTableCellWidthOverHundredPercentStaysInside(t *testing.T) {
	html := `<table><tr><td style="width:80%">L</td><td style="width:80%">R</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS, html))
	var sum bag.ScaledPoint
	for _, w := range widths {
		sum += w
	}
	if cw := contentWidth(t); sum > cw+bag.MustSP("1pt") {
		t.Errorf("columns add up to %s, exceeding the content width %s", sum, cw)
	}
}

// Without any declared width the columns keep shrinking to their content, so
// the existing auto layout is untouched.
func TestTableWithoutCellWidthStaysAuto(t *testing.T) {
	html := `<table><tr><td>L</td><td>R</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS, html))
	if len(widths) != 2 {
		t.Fatalf("got %d columns, want 2", len(widths))
	}
	half := contentWidth(t) / 2
	if widths[0] >= half {
		t.Errorf("auto column grew to %s; expected it to shrink well below %s", widths[0], half)
	}
}
