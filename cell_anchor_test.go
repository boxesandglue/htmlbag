package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// anchorPages maps each collected anchor id to the page it was placed on.
func anchorPages(cb *CSSBuilder) map[string]int {
	pages := map[string]int{}
	for _, a := range cb.Anchors {
		pages[a.ID] = a.Page
	}
	return pages
}

// TestCellInlineID checks that an element with an id inside a table cell
// renders, and that the id becomes an anchor with a page, as it does in a
// paragraph.
func TestCellInlineID(t *testing.T) {
	const svg = `<svg id="x" width="20pt" height="10pt" viewBox="0 0 20 10"><rect width="20" height="10"/></svg>`
	cases := map[string]string{
		"span":             `<table><tr><td>Initialled: <span id="x">here</span></td></tr></table>`,
		"span in p":        `<table><tr><td><p>Initialled: <span id="x">here</span></p></td></tr></table>`,
		"span in div":      `<table><tr><td><div>Initialled: <span id="x">here</span></div></td></tr></table>`,
		"span in th":       `<table><thead><tr><th>Initialled: <span id="x">here</span></th></tr></thead><tbody><tr><td>one</td></tr></tbody></table>`,
		"svg":              `<table><tr><td>` + svg + `</td></tr></table>`,
		"svg with text":    `<table><tr><td>Signed: ` + svg + `</td></tr></table>`,
		"span beside text": `<p>before</p><table><tr><td>Initialled: <span id="x">here</span></td></tr></table><p>after</p>`,
		"bordered table":   `<table style="border: 1pt solid black"><tr><td>Initialled: <span id="x">here</span></td></tr></table>`,
		"table background": `<table style="background-color: #eee"><tr><td>Initialled: <span id="x">here</span></td></tr></table>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cb := renderForAnchors(t, `<!DOCTYPE html><html><body>`+body+`</body></html>`)
			if got := anchorPages(cb)["x"]; got != 1 {
				t.Errorf("anchor x is on page %d, want 1 (anchors %#v)", got, cb.Anchors)
			}
		})
	}
}

// TestCellInlineIDSplitTable checks that an inline id in a row of a table
// taller than a page gets the page its row is painted on, with and without
// repeated header rows.
func TestCellInlineIDSplitTable(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&rows, `<tr><td>Row <span id="r%d">%d</span></td></tr>`, i, i)
	}
	const a6 = `@page { size: a6 }`
	thead := `<table style="width: 100%"><thead><tr><th>Head</th></tr></thead><tbody>` + rows.String() + `</tbody></table>`
	bordered := `<table style="border: 1pt solid black">` + rows.String() + `</table>`
	cases := []struct{ name, css, body string }{
		{"no header", a6, `<table>` + rows.String() + `</table>`},
		{"after p", a6, `<p>before</p><table>` + rows.String() + `</table>`},
		{"bordered", a6, bordered},
		{"bordered, after p", a6, `<p>before</p>` + bordered},
		{"background", a6, `<table style="background-color: #eee">` + rows.String() + `</table>`},
		{"thead", a6, thead},
		// The rows after the break are rebuilt at the wider measure.
		{"thead, reflow", reflowNarrowFirstCSS, thead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pgs, cb := renderHTMLPagesCB(t, tc.css, `<html><body>`+tc.body+`</body></html>`)
			text := make([]string, len(pgs))
			for i, pg := range pgs {
				var sb strings.Builder
				for _, obj := range pg.Objects {
					collectComponents(obj.Vlist, &sb)
				}
				// A trailing marker, so "Row1" cannot match "Row10".
				text[i] = strings.ReplaceAll(sb.String(), "Row", "|Row") + "|"
			}
			pages := anchorPages(cb)
			for i := 1; i <= 80; i++ {
				id := fmt.Sprintf("r%d", i)
				p := pages[id]
				if p < 1 || p > len(text) || !strings.Contains(text[p-1], fmt.Sprintf("|Row%d|", i)) {
					t.Errorf("%s is on page %d, which does not show its row", id, p)
				}
			}
		})
	}
}

// TestCellInlineIDNestedTableInFloat checks that a table built while its
// enclosing table is still collecting rows (here, inside a float in a cell)
// does not disturb the outer table's anchors.
func TestCellInlineIDNestedTableInFloat(t *testing.T) {
	html := `<html><body><table>
<tr><td>First <span id="a">row</span></td></tr>
<tr><td><div style="float: left; width: 30%"><table><tr><td>In <span id="n">float</span></td></tr></table></div>Second <span id="b">row</span></td></tr>
</table></body></html>`
	_, cb := renderHTMLPagesCB(t, borderModelCSS, html)
	pages := anchorPages(cb)
	for _, id := range []string{"a", "b"} {
		if pages[id] != 1 {
			t.Errorf("anchor %s is on page %d, want 1 (anchors %v)", id, pages[id], pages)
		}
	}
}

// TestCellInlineIDTwoTables checks that each table starts its row list afresh.
func TestCellInlineIDTwoTables(t *testing.T) {
	html := `<html><body><table><tr><td>First <span id="a">table</span></td></tr></table>
<table style="page-break-before: always"><tr><td>Second <span id="b">table</span></td></tr></table></body></html>`
	_, cb := renderHTMLPagesCB(t, borderModelCSS, html)
	pages := anchorPages(cb)
	if pages["a"] != 1 || pages["b"] != 2 {
		t.Errorf("anchors on pages %v, want a on 1 and b on 2", pages)
	}
}
