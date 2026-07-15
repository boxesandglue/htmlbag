package htmlbag

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// renderHTMLPages runs html through the full pipeline (ParseCSSString →
// HTMLToText → OutputPagesFromText) and returns the finished pages.
func renderHTMLPages(t *testing.T, css, html string) []*document.Page {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	return fe.Doc.Pages
}

// collectComponents concatenates the glyph Components of a node list,
// descending into nested lists. Glue is skipped, so words join without
// separators ("Zeile 60" comes back as "Zeile60").
func collectComponents(n node.Node, sb *strings.Builder) {
	for ; n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.Glyph:
			sb.WriteString(v.Components)
		case *node.HList:
			collectComponents(v.List, sb)
		case *node.VList:
			collectComponents(v.List, sb)
		}
	}
}

func pageText(pg *document.Page) string {
	var sb strings.Builder
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			collectComponents(obj.Vlist.List, &sb)
		}
	}
	return sb.String()
}

// lowestInk returns the lowest y coordinate any object on the page reaches
// (object top minus its vlist extent).
func lowestInk(pg *document.Page) bag.ScaledPoint {
	low := bag.ScaledPoint(1 << 62)
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if bottom := obj.Y - obj.Vlist.Height - obj.Vlist.Depth; bottom < low {
			low = bottom
		}
	}
	return low
}

const tableSplitCSS = `@page { size: a4; margin: 20mm; }
table.items { width: 100%; border-collapse: collapse; }
table.items td { padding: 2pt 4pt; }`

func itemRows(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "<tr><td>Zeile %d Beschreibung der Position</td><td>%d.00 EUR</td></tr>\n", i, i)
	}
	return sb.String()
}

// requireAllRows asserts that every row 1..n appears exactly once across the
// pages and returns the (1-based) page number that carries row n.
func requireAllRows(t *testing.T, pages []*document.Page, n int) int {
	t.Helper()
	texts := make([]string, len(pages))
	for i, pg := range pages {
		texts[i] = pageText(pg)
	}
	lastRowPage := 0
	for i := 1; i <= n; i++ {
		needle := fmt.Sprintf("Zeile%dBeschreibung", i)
		found := 0
		for pi, txt := range texts {
			found += strings.Count(txt, needle)
			if i == n && strings.Contains(txt, needle) {
				lastRowPage = pi + 1
			}
		}
		if found != 1 {
			t.Errorf("Zeile %d: found %d times, want exactly once", i, found)
		}
	}
	return lastRowPage
}

// requireInkAboveBottomMargin asserts no page paints below the bottom margin
// (overflowing table rows used to run into the margin). A small tolerance
// covers glyph descenders that legitimately reach below the baseline of the
// last line.
func requireInkAboveBottomMargin(t *testing.T, pages []*document.Page) {
	t.Helper()
	limit := bag.MustSP("20mm") - bag.MustSP("2mm")
	for i, pg := range pages {
		if low := lowestInk(pg); low < limit {
			t.Errorf("page %d: content reaches y=%s, below bottom margin (limit %s)", i+1, low, limit)
		}
	}
}

// TestSiblingAfterSplitTableWithThead: a block following a table that broke
// across pages must continue on the table's last page, not on a forced fresh
// page. 60 rows split 1→2; the closing paragraph must land on page 2.
func TestSiblingAfterSplitTableWithThead(t *testing.T) {
	html := `<html><body><table class="items">
<thead><tr><th>Kopf</th><th>Preis</th></tr></thead>
<tbody>` + itemRows(60) + `</tbody></table>
<p>SCHLUSSTEXT nach der Tabelle</p></body></html>`
	pages := renderHTMLPages(t, tableSplitCSS, html)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2 (closing paragraph must not force a fresh page)", len(pages))
	}
	lastRowPage := requireAllRows(t, pages, 60)
	if lastRowPage != 2 {
		t.Errorf("row 60 on page %d, want 2", lastRowPage)
	}
	if !strings.Contains(pageText(pages[1]), "SCHLUSSTEXT") {
		t.Error("closing paragraph not on page 2 (the table's last page)")
	}
	requireInkAboveBottomMargin(t, pages)
}

// TestSplitTableWithoutTheadKeepsAllRows: a 60-row table without <thead>
// followed by a sibling must still break across pages with all rows intact
// and within the bottom margin (rows 50–60 used to vanish and rows 47–49 ran
// into the margin). All variants must match the <thead> behavior: two pages,
// sibling flowing right below the last row.
func TestSplitTableWithoutTheadKeepsAllRows(t *testing.T) {
	table := `<table class="items"><tbody>` + itemRows(60) + `</tbody></table>`
	cases := []struct {
		name string
		html string
	}{
		{"bare table", table},
		{"table then paragraph", table + `<p>SCHLUSSTEXT nach der Tabelle</p>`},
		{"table then table", table + `<table class="items"><tbody><tr><td>ZWEITETABELLE</td><td>1.00</td></tr></tbody></table>`},
		{"paragraph table paragraph", `<p>EINLEITUNG vor der Tabelle</p>` + table + `<p>SCHLUSSTEXT nach der Tabelle</p>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := renderHTMLPages(t, tableSplitCSS, `<html><body>`+tc.html+`</body></html>`)
			if len(pages) != 2 {
				t.Fatalf("got %d pages, want 2", len(pages))
			}
			requireAllRows(t, pages, 60)
			requireInkAboveBottomMargin(t, pages)
			lastPage := pageText(pages[1])
			for _, marker := range []string{"SCHLUSSTEXT", "ZWEITETABELLE"} {
				if strings.Contains(tc.html, marker) && !strings.Contains(lastPage, marker) {
					t.Errorf("sibling %q not on the table's last page", marker)
				}
			}
		})
	}
}
