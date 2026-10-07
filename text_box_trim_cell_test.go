package htmlbag

import (
	"fmt"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

const trimCellCSS = trimCSS + ` table { border-collapse: collapse } td { padding: 0; font-family: serif; font-size: 10pt; line-height: 16pt }`

// trimAmounts returns what text-box-trim takes off the start and the end of
// a trimCSS line, measured on a paragraph on the page.
func trimAmounts(t *testing.T) (start, end bag.ScaledPoint) {
	t.Helper()
	full := sp("16pt")
	size := func(style string) bag.ScaledPoint {
		return lineSize(placedLines(renderHTMLPages(t, trimCSS, `<p style="`+style+`">A</p>`))[0])
	}
	return full - size("text-box-trim: trim-start"), full - size("text-box-trim: trim-end")
}

// A table cell is a block container: text-box-trim on the cell trims the
// first and last line of its contents, through the blocks in it, unless
// padding or a border on the side of a block lies between, and a paragraph
// in a cell trims its own lines, as on the page. A table holds no line of
// the cell, and the trim does not reach the cells of a table in it.
func TestTextBoxTrimCell(t *testing.T) {
	full := sp("16pt")
	start, end := trimAmounts(t)
	for _, c := range []struct {
		name, cell string
		height     bag.ScaledPoint
		start, end bool
	}{
		{"cell", `<td style="text-box-trim: trim-both">Aq</td>`, full, true, true},
		{"cell, start", `<td style="text-box-trim: trim-start">Aq</td>`, full, true, false},
		{"cell, end", `<td style="text-box-trim: trim-end">Aq</td>`, full, false, true},
		{"cell, its own padding", `<td style="text-box-trim: trim-both; padding: 3pt">Aq</td>`, full + sp("6pt"), true, true},
		{"cell, two lines", `<td style="text-box-trim: trim-both">A<br>B</td>`, 2 * full, true, true},
		{"cell, two paragraphs", `<td style="text-box-trim: trim-both"><p>A</p><p>B</p></td>`, 2 * full, true, true},
		{"cell, a div", `<td style="text-box-trim: trim-both"><div><p>A</p></div></td>`, full, true, true},
		{"cell, padding on the paragraph", `<td style="text-box-trim: trim-both"><p style="padding-top: 2pt">A</p></td>`, full, false, true},
		{"cell, a table last", `<td style="text-box-trim: trim-both"><p>A</p><table><tr><td>B</td></tr></table></td>`, 2 * full, true, false},
		{"paragraph", `<td><p style="text-box-trim: trim-both">Aq</p></td>`, full, true, true},
		{"paragraph with a side margin", `<td><p style="text-box-trim: trim-both; margin-left: 5pt">Aq</p></td>`, full, true, true},
		{"paragraph sized to the cell", `<td><p style="text-box-trim: trim-both">Aq <svg width="10%" height="4pt" viewBox="0 0 10 4"><rect width="10" height="4"/></svg></p></td>`, full, true, true},
		{"none", `<td>Aq</td>`, full, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			want := c.height
			if c.start {
				want -= start
			}
			if c.end {
				want -= end
			}
			pages := renderHTMLPages(t, trimCellCSS, `<table><tr>`+c.cell+`</tr></table>`)
			if got := tableRowHeights(t, pages)[0]; got != want {
				t.Errorf("row is %s, want %s", got, want)
			}
		})
	}
}

// A trimmed paragraph is formatted by htmlbag rather than in frontend, and
// the table still measures it as before: the trim changes no column width.
func TestTextBoxTrimCellKeepsColumnWidths(t *testing.T) {
	widths := func(body string) []bag.ScaledPoint {
		var ws []bag.ScaledPoint
		for c := tableRows(renderHTMLPages(t, trimCellCSS, body))[0].List; c != nil; c = c.Next() {
			w, _, _ := c.Sizes(node.Horizontal)
			ws = append(ws, w)
		}
		return ws
	}
	for _, tbl := range []string{
		`<table><tr><td%s>A fairly long text in the first cell that wraps</td><td>short</td></tr></table>`,
		`<table style="width: 100%%"><tr><td%s>Aq</td><td>A much longer second cell with several words</td></tr></table>`,
		`<table><tr><td><p%s>Supercalifragilistic word</p></td><td>x</td></tr></table>`,
	} {
		plain := widths(fmt.Sprintf(tbl, ""))
		trimmed := widths(fmt.Sprintf(tbl, ` style="text-box-trim: trim-both"`))
		if len(plain) != len(trimmed) {
			t.Fatalf("%d cells trimmed, %d without", len(trimmed), len(plain))
		}
		for i := range plain {
			if trimmed[i] != plain[i] {
				t.Errorf("%s: cell %d is %s wide, want %s as without the trim", tbl, i+1, trimmed[i], plain[i])
			}
		}
	}
}
