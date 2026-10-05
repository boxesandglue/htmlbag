package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// widestRow is the width of the widest table row on pg.
func widestRow(pg *document.Page) bag.ScaledPoint {
	var w bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.HList:
				if o, _ := c.Attributes["origin"].(string); o == "table row" {
					w = max(w, c.Width)
				}
				walk(c.List)
			case *node.VList:
				walk(c.List)
			}
		}
	}
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			walk(obj.Vlist)
		}
	}
	return w
}

// A table without a header that runs over a page break onto a wider page is
// set again at the new width, as one with a header is (#27).
func TestPageWidthReflowTableWithoutHeader(t *testing.T) {
	css := reflowNarrowFirstCSS + `
table.items { width: 100%; border-collapse: collapse; }
table.items td { padding: 2pt 4pt; }`
	table := `<table class="items"><tbody>` + itemRows(60) + `</tbody></table>`
	for _, c := range []struct{ name, body string }{
		{"alone", table},
		{"after a heading", `<h2>Items</h2>` + table},
		{"in a div", `<div>` + table + `</div>`},
		{"after a paragraph in a div", `<div><p>Before the table.</p>` + table + `</div>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages, _ := renderHTMLPagesCB(t, css, `<html><body>`+c.body+`</body></html>`)
			if len(pages) < 2 {
				t.Fatalf("got %d pages, want at least 2", len(pages))
			}
			requireAllRows(t, pages, 60)
			// The table is 100% wide: its rows take the width of the page
			// they are on.
			requireWidth(t, "page 1 rows", widestRow(pages[0]), bag.MustSP("130mm"))
			requireWidth(t, "page 2 rows", widestRow(pages[1]), bag.MustSP("170mm"))
		})
	}
}
