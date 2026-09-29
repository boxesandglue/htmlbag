package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// A table with a repeated header does not break between the rows a rowspan
// joins: the group moves to the next page together. The sweep over the
// number of rows before the group puts the page end between its rows for
// some count, wherever the font metrics place it.
func TestRowspanGroupStaysOnOnePage(t *testing.T) {
	for n := 40; n <= 70; n++ {
		t.Run(fmt.Sprintf("%drows", n), func(t *testing.T) {
			html := `<table class="items"><thead><tr><th>Kopf</th><th>Preis</th></tr></thead><tbody>` + itemRows(n) +
				`<tr><td rowspan="2">Gruppe</td><td>GruppeA</td></tr><tr><td>GruppeB</td></tr>
<tr><td>Danach</td><td>0.00 EUR</td></tr></tbody></table>`
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
}
