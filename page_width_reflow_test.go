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

// renderHTMLPagesCB runs html through the full pipeline like renderHTMLPages
// but also returns the CSSBuilder for assertions on collected headings.
func renderHTMLPagesCB(t *testing.T, css, html string) ([]*document.Page, *CSSBuilder) {
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
	return fe.Doc.Pages, cb
}

// maxLineWidth walks the page objects and returns the widest paragraph line
// (HList with origin "line"). Lines are hpacked to the paragraph's hsize, so
// this reports the measure the line breaker actually used on that page.
func maxLineWidth(pg *document.Page) bag.ScaledPoint {
	var maxW bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if v.Attributes != nil {
					if o, _ := v.Attributes["origin"].(string); o == "line" && v.Width > maxW {
						maxW = v.Width
					}
				}
				walk(v.List)
			case *node.VList:
				walk(v.List)
			}
		}
	}
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			walk(obj.Vlist.List)
		}
	}
	return maxW
}

// requireWidth asserts got == want within a 1pt tolerance.
func requireWidth(t *testing.T, what string, got, want bag.ScaledPoint) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > bag.MustSP("1pt") {
		t.Errorf("%s: line width %s, want %s", what, got, want)
	}
}

// A4 (210mm) with 20mm margins => 170mm content width; the :first page
// carries a tall letterhead and a 60mm right margin => 130mm content width.
// Deliberately written with margin longhands — the literal repro from
// seite-2-problem.md Teil B — which csshtml.doPage folds into the page
// geometry since 2026-07-15.
const reflowNarrowFirstCSS = `
@page { size: a4; margin: 20mm; }
@page :first { margin-top: 80mm; margin-right: 60mm; }
body { margin: 0; }`

// The mirrored case: wide :first page, narrow following pages.
const reflowWideFirstCSS = `
@page { size: a4; margin: 20mm 60mm 20mm 20mm; }
@page :first { margin: 20mm; }
body { margin: 0; }`

// TestPageWidthReflowParagraph is the minimal repro from
// seite-2-problem.md, Teil B: a single long paragraph starts on a narrow
// :first page and must re-flow to the full width of page 2 instead of
// keeping the frozen first-page measure.
func TestPageWidthReflowParagraph(t *testing.T) {
	html := `<html><body><p>` + strings.Repeat("Wort ", 2000) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, reflowNarrowFirstCSS, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	requireWidth(t, "page 1", maxLineWidth(pages[0]), bag.MustSP("130mm"))
	for i := 1; i < len(pages); i++ {
		requireWidth(t, fmt.Sprintf("page %d", i+1), maxLineWidth(pages[i]), bag.MustSP("170mm"))
	}
}

// TestPageWidthReflowParagraphNarrower covers the reverse case: page 1 is
// wide, the following pages are narrower. Without the reflow the second
// page's lines would overflow into the right margin.
func TestPageWidthReflowParagraphNarrower(t *testing.T) {
	html := `<html><body><p>` + strings.Repeat("Wort ", 2000) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, reflowWideFirstCSS, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	requireWidth(t, "page 1", maxLineWidth(pages[0]), bag.MustSP("170mm"))
	for i := 1; i < len(pages); i++ {
		requireWidth(t, fmt.Sprintf("page %d", i+1), maxLineWidth(pages[i]), bag.MustSP("130mm"))
	}
}

// TestPageWidthReflowItemsAndHeadings: the break falls between body items, so
// OutputPagesFromText restarts the group and rebuilds the remaining items at
// the new width. The heading placed after the restart must keep exactly one
// Headings entry (the rebuild must not register duplicates) with the correct
// page number (carry of _heading_idx).
func TestPageWidthReflowItemsAndHeadings(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&body, "<p>Absatz %d %s</p>\n", i, strings.Repeat("Wort ", 120))
	}
	body.WriteString("<h2>KAPITELZWEI</h2>\n<p>" + strings.Repeat("Schluss ", 120) + "</p>")
	pages, cb := renderHTMLPagesCB(t, reflowNarrowFirstCSS, `<html><body>`+body.String()+`</body></html>`)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	requireWidth(t, "page 1", maxLineWidth(pages[0]), bag.MustSP("130mm"))
	for i := 1; i < len(pages); i++ {
		requireWidth(t, fmt.Sprintf("page %d", i+1), maxLineWidth(pages[i]), bag.MustSP("170mm"))
	}
	// The heading text must appear exactly once in the document.
	found := 0
	headingPage := 0
	for i, pg := range pages {
		if c := strings.Count(pageText(pg), "KAPITELZWEI"); c > 0 {
			found += c
			headingPage = i + 1
		}
	}
	if found != 1 {
		t.Errorf("heading rendered %d times, want exactly once", found)
	}
	var entries []HeadingEntry
	for _, h := range cb.Headings {
		if h.Text == "KAPITELZWEI" {
			entries = append(entries, h)
		}
	}
	if len(entries) != 1 {
		t.Fatalf("got %d Headings entries for KAPITELZWEI, want 1 (rebuild must not duplicate)", len(entries))
	}
	if entries[0].Page != headingPage {
		t.Errorf("Headings entry page = %d, heading painted on page %d", entries[0].Page, headingPage)
	}
}

// TestPageWidthReflowSplitTable: a table with a repeating header starts on
// the narrow :first page and continues on wider pages. The remaining rows
// (and the repeated header) must be rebuilt at the new content width, and
// every row must appear exactly once (no losses, no duplicates).
func TestPageWidthReflowSplitTable(t *testing.T) {
	css := reflowNarrowFirstCSS + `
table.items { width: 100%; border-collapse: collapse; }
table.items td { padding: 2pt 4pt; }`
	html := `<html><body><table class="items">
<thead><tr><th>Kopf</th><th>Preis</th></tr></thead>
<tbody>` + itemRows(60) + `</tbody></table></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	requireAllRows(t, pages, 60)
	requireInkAboveBottomMargin(t, pages)

	// Row boxes are placed as individual page objects with the table
	// width. Restrict the measurement to objects that carry glyphs so the
	// (invisible) @page border box does not mask a missed reflow.
	maxTextObjWidth := func(pg *document.Page) bag.ScaledPoint {
		var maxW bag.ScaledPoint
		for _, obj := range pg.Objects {
			if obj.Vlist == nil {
				continue
			}
			var sb strings.Builder
			collectComponents(obj.Vlist.List, &sb)
			if sb.Len() > 0 && obj.Vlist.Width > maxW {
				maxW = obj.Vlist.Width
			}
		}
		return maxW
	}
	requireWidth(t, "page 1 table", maxTextObjWidth(pages[0]), bag.MustSP("130mm"))
	requireWidth(t, "page 2 table", maxTextObjWidth(pages[1]), bag.MustSP("170mm"))

	// The repeated thead header on page 2 must come from the rebuilt table,
	// i.e. it must be present at all (outputTableRows path, not the splice).
	if !strings.Contains(pageText(pages[1]), "Kopf") {
		t.Error("repeating table header missing on page 2")
	}
}

// TestPageWidthSameWidthControl: when :first only changes the vertical
// margins, the content width stays the same and the fast path (pure height
// slicing, no rebuild) must produce the usual layout.
func TestPageWidthSameWidthControl(t *testing.T) {
	css := `
@page { size: a4; margin: 20mm; }
@page :first { margin-top: 80mm; }
body { margin: 0; }`
	html := `<html><body><p>` + strings.Repeat("Wort ", 2000) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	for i, pg := range pages {
		requireWidth(t, fmt.Sprintf("page %d", i+1), maxLineWidth(pg), bag.MustSP("170mm"))
	}
}
