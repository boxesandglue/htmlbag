package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// pageBackgroundColor returns the fill operator of the page background color
// rule on pg, or "" if the page has none.
func pageBackgroundColor(pg *document.Page) string {
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if o, _ := obj.Vlist.Attributes["origin"].(string); o != "page background color" {
			continue
		}
		if r, ok := obj.Vlist.List.(*node.Rule); ok {
			return r.Pre
		}
	}
	return ""
}

// A background-color on @page applies to every page, and :first overrides
// it on the first page (#102). It was painted on page 1 only.
func TestPageBackgroundColorOnEveryPage(t *testing.T) {
	const html = `<p>One.</p><p style="page-break-before: always">Two.</p>
<p style="page-break-before: always">Three.</p>`
	for _, tc := range []struct {
		name, css string
		same      bool // page 1 has the color of the later pages
	}{
		{"@page", `@page { size: a6; margin: 10mm; background-color: #ffe0e0 }`, true},
		{"@page :first", `@page { size: a6; margin: 10mm; background-color: #ffe0e0 }
@page :first { background-color: #e0e0ff }`, false},
	} {
		pages := renderHTMLPages(t, tc.css, html)
		if len(pages) != 3 {
			t.Fatalf("%s: got %d pages, want 3", tc.name, len(pages))
		}
		var colors []string
		for i, pg := range pages {
			c := pageBackgroundColor(pg)
			if c == "" {
				t.Errorf("%s: page %d has no background color", tc.name, i+1)
			}
			colors = append(colors, c)
		}
		if colors[1] != colors[2] {
			t.Errorf("%s: pages 2 and 3 differ: %q, %q", tc.name, colors[1], colors[2])
		}
		if (colors[0] == colors[1]) != tc.same {
			t.Errorf("%s: page 1 %q, page 2 %q, same = %t, want %t", tc.name, colors[0], colors[1], !tc.same, tc.same)
		}
	}
}
