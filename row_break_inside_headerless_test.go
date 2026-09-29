package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// A row with break-inside: auto splits across pages whether or not the table
// has a repeated header; without one it used to move whole to the next page
// and run into the bottom margin. A <tfoot> without a <thead> stays at the
// end of the table, as it does for tables without a split row.
func TestBreakInsideAutoWithoutHeader(t *testing.T) {
	var lines []string
	for i := 1; i <= 120; i++ {
		lines = append(lines, fmt.Sprintf("Zeile %d Beschreibung", i))
	}
	for _, tc := range []struct{ name, head, foot string }{
		{"header=true", "<thead><tr><th>Kopf</th></tr></thead>", ""},
		{"header=false", "", ""},
		{"header=false/footer=true", "", "<tfoot><tr><td>Fuss</td></tr></tfoot>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := `<table class="items">` + tc.head + tc.foot + `<tbody>
<tr><td>Vorher</td></tr>
<tr style="break-inside: auto"><td>` + strings.Join(lines, "<br>") + `</td></tr>
<tr><td>Nachher</td></tr>
</tbody></table>`
			pages := renderHTMLPages(t, tableSplitCSS, html)
			requireAllRows(t, pages, 120)
			requireInkAboveBottomMargin(t, pages)
			if first := pageText(pages[0]); !strings.Contains(first, "Vorher") || !strings.Contains(first, "Zeile1Beschreibung") {
				t.Errorf("page 1 holds %q, want the row before and the first lines of the split row", first)
			}
			last := pageText(pages[len(pages)-1])
			if !strings.Contains(last, "Nachher") {
				t.Errorf("the last page holds %q, want the row after the split row", last)
			}
			if tc.foot != "" {
				n := 0
				for _, pg := range pages {
					n += strings.Count(pageText(pg), "Fuss")
				}
				if n != 1 || !strings.Contains(last, "Fuss") {
					t.Errorf("footer found %d times, want once on the last page", n)
				}
			}
		})
	}
}

// A table without a header that fits on a page but not in the space left on
// this one splits a break-inside: auto row where the page ends, as it would
// with a header, rather than moving the row to the next page.
func TestBreakInsideAutoWithoutHeaderSplitsAtPageEnd(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("Zeile %d Beschreibung", i))
	}
	html := `<div style="height: 200mm">Oben</div><table class="items"><tbody>
<tr style="break-inside: auto"><td>` + strings.Join(lines, "<br>") + `</td></tr>
</tbody></table>`
	pages := renderHTMLPages(t, tableSplitCSS, html)
	if len(pages) != 2 {
		t.Fatalf("%d pages, want 2", len(pages))
	}
	requireAllRows(t, pages, 30)
	requireInkAboveBottomMargin(t, pages)
	if first := pageText(pages[0]); !strings.Contains(first, "Zeile1Beschreibung") {
		t.Errorf("page 1 holds %q, want the first lines of the row", first)
	}
}

// Rows a rowspan joins stay on one page in a table without a header that
// takes the row-splitting path.
func TestRowspanGroupStaysOnOnePageWithoutHeader(t *testing.T) {
	for n := 40; n <= 70; n++ {
		t.Run(fmt.Sprintf("%drows", n), func(t *testing.T) {
			html := `<table class="items"><tbody><tr style="break-inside: auto"><td>Anfang</td><td>0.00 EUR</td></tr>` + itemRows(n) +
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
