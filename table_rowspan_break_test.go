package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// A table does not break between the rows a rowspan joins, with a repeated
// header or without one: the group moves to the next page together. The
// sweep over the number of rows before the group puts the page end between
// its rows for some count, wherever the font metrics place it.
func TestRowspanGroupStaysOnOnePage(t *testing.T) {
	for _, head := range []string{`<thead><tr><th>Kopf</th><th>Preis</th></tr></thead>`, ""} {
		name := "with a header"
		if head == "" {
			name = "without a header"
		}
		t.Run(name, func(t *testing.T) {
			for n := 40; n <= 70; n++ {
				t.Run(fmt.Sprintf("%drows", n), func(t *testing.T) {
					html := `<table class="items">` + head + `<tbody>` + itemRows(n) + rowspanGroup + `</tbody></table>`
					pages := renderHTMLPages(t, tableSplitCSS, html)
					requireAllRows(t, pages, n)
					requireInkAboveBottomMargin(t, pages)
					for i, pg := range pages {
						txt := pageText(pg)
						if a, b := strings.Contains(txt, "GruppeA"), strings.Contains(txt, "GruppeB"); a != b {
							t.Errorf("page %d holds one row of the rowspan group only (A %v, B %v)", i+1, a, b)
						}
					}
				})
			}
		})
	}
}

const rowspanGroup = `<tr><td rowspan="2">Gruppe</td><td>GruppeA</td></tr><tr><td>GruppeB</td></tr>
<tr><td>Danach</td><td>0.00 EUR</td></tr>`

// The same in FlowText regions, for a table without a header, the region
// ending after each of the rows before the group in turn.
func TestFlowTextRowspanGroupStaysInOneRegion(t *testing.T) {
	for n := 1; n <= 6; n++ {
		t.Run(fmt.Sprintf("%drows", n), func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			var rows strings.Builder
			for i := 0; i < n; i++ {
				rows.WriteString(`<tr><td>Rq</td><td>x</td></tr>`)
			}
			body := `<table><tbody>` + rows.String() + `<tr><td rowspan="2">Gq</td><td>GruppeA</td></tr><tr><td>GruppeB</td></tr></tbody></table>`
			tr := flow(t, cb, body, wide("60pt"), wide("1000pt"))
			for i, f := range tr.filled {
				var txt strings.Builder
				for _, l := range boxLines(f) {
					txt.WriteString(l.text)
				}
				if a, b := strings.Contains(txt.String(), "GruppeA"), strings.Contains(txt.String(), "GruppeB"); a != b {
					t.Errorf("region %d holds one row of the rowspan group only (A %v, B %v)", i+1, a, b)
				}
			}
		})
	}
}
