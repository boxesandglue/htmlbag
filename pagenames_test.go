package htmlbag

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
)

const namedPagesCSS = `
@page { size: a4; margin: 20mm; }
@page wide { size: a4 landscape; margin: 10mm; }
`

// requirePage asserts the size of a page in mm and that it holds want.
func requirePage(t *testing.T, pages []*document.Page, i int, wdMM, htMM int, want string) {
	t.Helper()
	if i >= len(pages) {
		t.Fatalf("page %d does not exist, %d pages", i+1, len(pages))
	}
	pg := pages[i]
	mm := func(n int) bag.ScaledPoint { return bag.MustSP(strconv.Itoa(n) + "mm") }
	if pg.Width != mm(wdMM) || pg.Height != mm(htMM) {
		t.Errorf("page %d is %s x %s, want %dmm x %dmm", i+1, pg.Width, pg.Height, wdMM, htMM)
	}
	if want != "" && !strings.Contains(pageText(pg), want) {
		t.Errorf("page %d does not hold %q: %q", i+1, want, pageText(pg))
	}
}

// A named block starts a page of its own at the size of its @page rule, the
// block after it goes back to the unnamed pages.
func TestPageNameChangesPageSize(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<p>PORTRAIT</p>
<div style="page: wide"><p>LANDSCAPE</p></div>
<p>AGAIN</p>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	requirePage(t, pages, 0, 210, 297, "PORTRAIT")
	requirePage(t, pages, 1, 297, 210, "LANDSCAPE")
	requirePage(t, pages, 2, 210, 297, "AGAIN")
}

// Two neighbors of the same name stay on one page, and pages after an
// automatic break keep the name.
func TestPageNameSharedAndAutomaticBreak(t *testing.T) {
	long := strings.Repeat("<p>filler</p>", 80)
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<div style="page: wide"><p>ONE</p></div>
<div style="page: wide"><p>TWO</p>`+long+`</div>
<p>END</p>
</body></html>`)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want at least 3", len(pages))
	}
	requirePage(t, pages, 0, 297, 210, "ONE")
	requirePage(t, pages, 0, 297, 210, "TWO")
	for i := 1; i < len(pages)-1; i++ {
		requirePage(t, pages, i, 297, 210, "filler")
	}
	requirePage(t, pages, len(pages)-1, 210, 297, "END")
}

// The page property of the first block names the first page.
func TestPageNameFirstPage(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<div style="page: wide"><p>LANDSCAPE</p></div>
<p>PORTRAIT</p>
</body></html>`)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	requirePage(t, pages, 0, 297, 210, "LANDSCAPE")
	requirePage(t, pages, 1, 210, 297, "PORTRAIT")
}

// A positioned element starts the first page while the text is built, before
// any name is known.
func TestPageNameFirstPageWithPositionedElement(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<div style="position: absolute; top: 1cm; left: 1cm">POS</div>
<div style="page: wide"><p>LANDSCAPE</p></div>
</body></html>`)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	requirePage(t, pages, 0, 297, 210, "LANDSCAPE")
}

// An element's page applies to its children with page: auto, the start and end
// values reach through the blocks around them.
func TestPageNameThroughNesting(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<p>PORTRAIT</p>
<div><div><p style="page: wide">INNER</p></div></div>
<div><div><p style="page: wide">INNER2</p></div><p>AFTER</p></div>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	requirePage(t, pages, 0, 210, 297, "PORTRAIT")
	requirePage(t, pages, 1, 297, 210, "INNER2")
	requirePage(t, pages, 2, 210, 297, "AFTER")
}

// A named block further down in a block is reached by the forced break, and the
// block goes on at the width of the new page.
func TestPageNameInsideBlock(t *testing.T) {
	text := strings.Repeat("word ", 300)
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<main><p>PORTRAIT</p>
<section style="page: wide"><p>LANDSCAPE `+text+`</p></section>
<p>AGAIN `+text+`</p></main>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	requirePage(t, pages, 0, 210, 297, "PORTRAIT")
	requirePage(t, pages, 1, 297, 210, "LANDSCAPE")
	requirePage(t, pages, 2, 210, 297, "AGAIN")
	requireWidth(t, "landscape page", maxLineWidth(pages[1]), bag.MustSP("277mm"))
	requireWidth(t, "portrait page", maxLineWidth(pages[2]), bag.MustSP("170mm"))
}

// Blocks around the change at any depth go on at the width of the new page, and
// the block around a change comes back to the old page after it.
func TestPageNameDeepInsideBlocks(t *testing.T) {
	text := strings.Repeat("word ", 300)
	row := "<tr><td>cell</td><td>cell</td></tr>"
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS+"table { width: 100% }", `<html><body>
<main><article><p>PORTRAIT</p>
<section style="page: wide"><table>`+row+`</table><p>`+text+`</p></section>
<p>AGAIN `+text+`</p></article><p>LAST</p></main>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	requirePage(t, pages, 1, 297, 210, "cell")
	requireWidth(t, "landscape page", maxLineWidth(pages[1]), bag.MustSP("277mm"))
	requirePage(t, pages, 2, 210, 297, "AGAIN")
	requirePage(t, pages, 2, 210, 297, "LAST")
	requireWidth(t, "portrait page", maxLineWidth(pages[2]), bag.MustSP("170mm"))
}

// A box around a change is cut as a split would: only the first part has the
// top side and the margin, only the last the bottom side.
func TestPageNameCutsBoxLikeASplit(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	b, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AddCSS(namedPagesCSS); err != nil {
		t.Fatal(err)
	}
	te, err := b.HTMLToText(`<html><body>
<div id="box" style="border: 1pt solid red; padding: 5pt; margin: 7pt"><p>ONE</p><p style="page: wide">TWO</p></div>
</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.resolvePageNames(te); err != nil {
		t.Fatal(err)
	}
	body := findBody(te)
	var parts []*frontend.Text
	for _, itm := range body.Items {
		if p, ok := itm.(*frontend.Text); ok && !isWhitespaceOnly(p) {
			parts = append(parts, p)
		}
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	head, tail := settingsToHTMLValues(parts[0].Settings), settingsToHTMLValues(parts[1].Settings)
	if head.BorderTopWidth == 0 || head.BorderBottomWidth != 0 || head.PaddingBottom != 0 {
		t.Errorf("first part: %+v", head)
	}
	if tail.BorderTopWidth != 0 || tail.PaddingTop != 0 || tail.BorderBottomWidth == 0 {
		t.Errorf("second part: %+v", tail)
	}
	if _, ok := parts[1].Settings[frontend.SettingElementID]; ok {
		t.Errorf("the id went with the second part")
	}
	if _, ok := parts[0].Settings[frontend.SettingMarginBottom]; ok {
		t.Errorf("the first part kept the margin at the end")
	}
}

// A table on a page of its own gets the width of that page.
func TestPageNameOnTable(t *testing.T) {
	row := "<tr><td>a</td><td>b</td></tr>"
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS+"table { width: 100% }", `<html><body>
<p>PORTRAIT</p>
<table style="page: wide">`+row+`</table>
<p>AGAIN</p>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	requirePage(t, pages, 1, 297, 210, "a")
	requirePage(t, pages, 2, 210, 297, "AGAIN")
}

// page on a table cell and on an inline element has no effect.
func TestPageNameIgnoredInTableAndInline(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<p>one <span style="page: wide">two</span> three</p>
<table><tr><td style="page: wide">a</td></tr><tr><td>b</td></tr></table>
</body></html>`)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	requirePage(t, pages, 0, 210, 297, "three")
}

// A break to a page side stays when the page name changes with it.
func TestPageNameKeepsPageSide(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS+"@page wide:blank { margin: 33mm } .c { page: wide; break-before: right }", `<html><body>
<p>ONE</p><p class="c">THREE</p>
</body></html>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	// The empty page belongs to the block it waits for, so @page wide:blank
	// can reach it.
	requirePage(t, pages, 1, 297, 210, "")
	requirePage(t, pages, 2, 297, 210, "THREE")
	margin := func(i int) bag.ScaledPoint {
		return pages[i].Userdata[PageDimensionsKey].(PageDimensions).MarginTop
	}
	if margin(1) != bag.MustSP("33mm") || margin(2) != bag.MustSP("10mm") {
		t.Errorf("margins of pages 2 and 3: %s, %s", margin(1), margin(2))
	}
}

// break-after on the block before the change takes the name with it: the
// break-before it causes finds the new page empty and adds nothing.
func TestPageNameAfterForcedBreakAfter(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<div><p>ONE</p><p style="break-after: page; margin-bottom: 2cm">TWO</p>
<div style="page: wide"><h1 style="break-before: right">THREE</h1></div></div>
</body></html>`)
	if len(pages) != 3 && len(pages) != 2 {
		t.Fatalf("got %d pages", len(pages))
	}
	requirePage(t, pages, len(pages)-1, 297, 210, "THREE")
}

// A name without a rule breaks the page and uses the generic rule.
func TestPageNameWithoutRule(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<p>ONE</p><p style="page: nothing">TWO</p>
</body></html>`)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	requirePage(t, pages, 1, 210, 297, "TWO")
}

// The rules of a named page cascade by specificity: the name over the side,
// the side of the name over the name, and :first over the side.
func TestPageRuleCascade(t *testing.T) {
	css := `
@page { size: a4; margin: 10mm; margin-top: 20mm; }
@page :right { margin-top: 30mm; }
@page :first { margin-bottom: 15mm; }
@page ch { margin-top: 40mm; }
@page ch:left, ch:blank { margin-top: 50mm; }
`
	_, cb := renderHTMLPagesCB(t, css, `<html><body>
<p>ONE</p><p style="page: ch">TWO</p><p style="page: ch; break-before: page">THREE</p>
</body></html>`)
	pages := cb.frontend.Doc.Pages
	want := []struct {
		top, bottom string
	}{{"30mm", "15mm"}, {"50mm", "10mm"}, {"40mm", "10mm"}}
	if len(pages) != len(want) {
		t.Fatalf("got %d pages, want %d", len(pages), len(want))
	}
	for i, w := range want {
		pd := pages[i].Userdata[PageDimensionsKey].(PageDimensions)
		if pd.MarginTop != bag.MustSP(w.top) || pd.MarginBottom != bag.MustSP(w.bottom) {
			t.Errorf("page %d: margin top %s, bottom %s, want %s, %s", i+1, pd.MarginTop, pd.MarginBottom, w.top, w.bottom)
		}
	}
}

// Without a margin of its own, a page takes the initial margin, not that of the
// page before it.
func TestPageGeometryDoesNotCarryOver(t *testing.T) {
	_, cb := renderHTMLPagesCB(t, `@page a { size: a5; margin: 5mm; }`, `<html><body>
<p style="page: a">ONE</p><p>TWO</p>
</body></html>`)
	pages := cb.frontend.Doc.Pages
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	pd := pages[1].Userdata[PageDimensionsKey].(PageDimensions)
	if pd.MarginTop != onecm || pd.Width != bag.MustSP("210mm") {
		t.Errorf("page 2: margin %s, width %s", pd.MarginTop, pd.Width)
	}
}

// A block beside the columns of a direct child of the body changes the page,
// and the page property inside columns of such a child has no effect.
func TestPageNameBesideColumns(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS, `<html><body>
<div style="column-count: 2"><p>ONE</p><p style="page: wide">TWO</p></div>
<p style="page: wide">THREE</p>
</body></html>`)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	requirePage(t, pages, 0, 210, 297, "TWO")
	requirePage(t, pages, 1, 297, 210, "THREE")
}

// Columns start a new page when the name changes.
func TestPageNameInColumns(t *testing.T) {
	pages, _ := renderHTMLPagesCB(t, namedPagesCSS+"body { column-count: 2 }", `<html><body>
<p>ONE</p><p style="page: wide">TWO</p>
</body></html>`)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	requirePage(t, pages, 1, 297, 210, "TWO")
}
