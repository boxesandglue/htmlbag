package htmlbag

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func splitRowsOf(n node.Node, out *[]*node.HList) {
	for ; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.HList:
			if t.Attributes["origin"] == "table row" {
				*out = append(*out, t)
			}
		case *node.VList:
			splitRowsOf(t.List, out)
		}
	}
}

// break-inside: auto on a row lets it break across pages; a row without it,
// or with avoid, stays whole.
func TestBreakInsideAutoLetsARowSplit(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<table><tr style="break-inside:auto"><td>a</td></tr><tr><td>b</td></tr><tr style="break-inside:avoid"><td>c</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("16cm"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []*node.HList
	splitRowsOf(vl, &rows)
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	for i, want := range []bool{true, false, false} {
		_, got := rows[i].Attributes["_split"].(frontend.RowSplitter)
		if got != want {
			t.Errorf("row %d splits: %v, want %v", i+1, got, want)
		}
	}
}

// A row with break-inside: auto in a table with a repeated header is split
// where the page ends: its first lines stay on the page it starts on, the
// rest runs on under the header on the next pages, nothing is lost or
// doubled and nothing runs into the bottom margin. A footer closes every
// page.
func TestBreakInsideAutoSplitsARowAcrossPages(t *testing.T) {
	var lines []string
	for i := 1; i <= 120; i++ {
		lines = append(lines, fmt.Sprintf("Zeile %d Beschreibung", i))
	}
	for _, foot := range []string{"", "<tfoot><tr><td>Fuss</td></tr></tfoot>"} {
		html := `<table class="items"><thead><tr><th>Kopf</th></tr></thead>` + foot + `<tbody>
<tr><td>Vorher</td></tr>
<tr style="break-inside: auto"><td>` + strings.Join(lines, "<br>") + `</td></tr>
<tr><td>Nachher</td></tr>
</tbody></table>`
		pages := renderHTMLPages(t, tableSplitCSS, html)
		if len(pages) < 3 {
			t.Fatalf("footer %v: %d pages, want at least 3", foot != "", len(pages))
		}
		requireAllRows(t, pages, 120)
		requireInkAboveBottomMargin(t, pages)
		if first := pageText(pages[0]); !strings.Contains(first, "Vorher") || !strings.Contains(first, "Zeile1Beschreibung") {
			t.Errorf("footer %v: page 1 holds %q, want the row before and the first lines of the split row", foot != "", first)
		}
		for i, pg := range pages {
			txt := pageText(pg)
			if !strings.Contains(txt, "Kopf") {
				t.Errorf("footer %v: page %d has no header", foot != "", i+1)
			}
			if foot != "" && !strings.Contains(txt, "Fuss") {
				t.Errorf("page %d has no footer", i+1)
			}
		}
		if last := pageText(pages[len(pages)-1]); !strings.Contains(last, "Nachher") {
			t.Errorf("footer %v: the last page holds %q, want the row after the split row", foot != "", last)
		}
	}
}

// A first data row with break-inside: auto that would fit on a page of its
// own still starts under the header where the table starts, split at the
// end of the page, rather than taking the header to the next page.
func TestBreakInsideAutoFirstRowFollowsTheHeader(t *testing.T) {
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("Zeile %d Beschreibung", i))
	}
	html := `<div style="height: 200mm">Oben</div><table class="items"><thead><tr><th>Kopf</th></tr></thead><tbody>
<tr style="break-inside: auto"><td>` + strings.Join(lines, "<br>") + `</td></tr>
</tbody></table>`
	pages := renderHTMLPages(t, tableSplitCSS, html)
	if len(pages) != 2 {
		t.Fatalf("%d pages, want 2", len(pages))
	}
	requireAllRows(t, pages, 30)
	requireInkAboveBottomMargin(t, pages)
	if first := pageText(pages[0]); !strings.Contains(first, "Kopf") || !strings.Contains(first, "Zeile1Beschreibung") {
		t.Errorf("page 1 holds %q, want the header and the first lines of the row", first)
	}
	if !strings.Contains(pageText(pages[1]), "Kopf") {
		t.Error("page 2 has no header")
	}
}
