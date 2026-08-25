package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// colWidthCSS widens the table so that percentages on <col> have a
// meaningful reference; the geometry matches cellWidthCSS.
const colWidthCSS = cellWidthCSS + `
table { width: 100% }`

// A percentage on <col> resolves against the table width. Regression
// test for a bug where the percentage ended up as a rigid zero-width
// glue that collapsed the column entirely.
func TestColPercentWidth(t *testing.T) {
	html := `<table><colgroup>
		<col style="width:20%"><col style="width:30%"><col style="width:50%">
		</colgroup><tr><td>one</td><td>two</td><td>three</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, colWidthCSS, html))
	if len(widths) != 3 {
		t.Fatalf("got %d columns, want 3", len(widths))
	}
	cw := contentWidth(t)
	for i, frac := range []float64{0.2, 0.3, 0.5} {
		want := bag.ScaledPoint(float64(cw) * frac)
		if !closeTo(widths[i], want) {
			t.Errorf("column %d is %s, want %s (%.0f%% of %s)", i, widths[i], want, frac*100, cw)
		}
	}
}

// Percentages adding up to more than 100% scale down proportionally,
// like specified cell widths do.
func TestColPercentOverconstrained(t *testing.T) {
	html := `<table><colgroup>
		<col style="width:60%"><col style="width:80%">
		</colgroup><tr><td>one</td><td>two</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, colWidthCSS, html))
	if len(widths) != 2 {
		t.Fatalf("got %d columns, want 2", len(widths))
	}
	cw := contentWidth(t)
	// 60:80 normalized to the table width
	for i, frac := range []float64{60.0 / 140.0, 80.0 / 140.0} {
		want := bag.ScaledPoint(float64(cw) * frac)
		if !closeTo(widths[i], want) {
			t.Errorf("column %d is %s, want %s", i, widths[i], want)
		}
	}
}

// The plain HTML width attribute on <col> works like CSS width; a bare
// number means pixels.
func TestColWidthAttribute(t *testing.T) {
	html := `<table><colgroup>
		<col width="120pt"><col width="60"><col>
		</colgroup><tr><td>one</td><td>two</td><td>three</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, colWidthCSS, html))
	if len(widths) != 3 {
		t.Fatalf("got %d columns, want 3", len(widths))
	}
	if want := bag.MustSP("120pt"); !closeTo(widths[0], want) {
		t.Errorf("first column is %s, want %s", widths[0], want)
	}
	// 60 (px) = 45pt
	if want := bag.MustSP("45pt"); !closeTo(widths[1], want) {
		t.Errorf("second column is %s, want %s", widths[1], want)
	}
	if want := contentWidth(t) - bag.MustSP("165pt"); !closeTo(widths[2], want) {
		t.Errorf("auto column is %s, want %s", widths[2], want)
	}
}

// An unparsable width on <col> falls back to auto instead of collapsing
// the column to zero width.
func TestColUnparsableWidthFallsBackToAuto(t *testing.T) {
	html := `<table><colgroup>
		<col style="width:oops"><col><col>
		</colgroup><tr><td>one</td><td>two</td><td>three</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, colWidthCSS, html))
	if len(widths) != 3 {
		t.Fatalf("got %d columns, want 3", len(widths))
	}
	want := contentWidth(t) / 3
	for i, wd := range widths {
		if !closeTo(wd, want) {
			t.Errorf("column %d is %s, want %s", i, wd, want)
		}
	}
}

// A fixed length on the table element sets the table width, like a
// percentage does.
func TestTableFixedWidth(t *testing.T) {
	html := `<table style="width:300pt"><tr><td>one</td><td>two</td></tr></table>`
	widths := firstRowCellWidths(t, renderHTMLPages(t, cellWidthCSS, html))
	var sum bag.ScaledPoint
	for _, w := range widths {
		sum += w
	}
	if want := bag.MustSP("300pt"); !closeTo(sum, want) {
		t.Errorf("columns add up to %s, want the fixed table width %s", sum, want)
	}
}
