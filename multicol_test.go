package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// The column properties are read with their shorthands, and none of them
// passes to a child.
func TestMulticolProperties(t *testing.T) {
	ih := resolveOnto(t, baseStyles(), resolveCSSText(`columns: 12em 3; column-gap: 2em; column-rule: 0.5pt solid red; column-fill: auto`))
	m := ih.multicol
	if m.count != 3 {
		t.Errorf("column-count %d, want 3 from columns: 12em 3", m.count)
	}
	if !m.gapSet || m.gap != bag.MustSP("20pt") {
		t.Errorf("column-gap %v (set %v), want 20pt", m.gap, m.gapSet)
	}
	if m.ruleWidth != bag.MustSP("0.5pt") || !m.ruleSolid || rgb(t, m.ruleColor) != rgb(t, resolveOnto(t, baseStyles(), resolveCSSText(`color: red`)).color) {
		t.Errorf("column-rule %v solid %v %s, want 0.5pt solid red", m.ruleWidth, m.ruleSolid, rgb(t, m.ruleColor))
	}
	if !m.fillAuto {
		t.Error("column-fill: auto not read")
	}

	if span := resolveOnto(t, baseStyles(), resolveCSSText(`column-span: all`)).multicol; !span.spanAll {
		t.Error("column-span: all not read")
	}
	if normal := resolveOnto(t, baseStyles(), resolveCSSText(`column-count: 2; column-gap: normal`)).multicol; normal.gapSet || normal.count != 2 {
		t.Errorf("column-gap: normal set %v, count %d; want unset, 2", normal.gapSet, normal.count)
	}
	if child := ih.Clone().multicol; child != (multicol{}) {
		t.Errorf("a child inherits the column properties: %+v", child)
	}
}

// placedText is the text of one object on a page and where its top left
// corner is.
type placedText struct {
	x, y bag.ScaledPoint
	text string
}

// pageTexts lists the objects of pg that hold text, in painting order.
func pageTexts(pg *document.Page) []placedText {
	var out []placedText
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectGlyphs(obj.Vlist.List, &sb)
		if sb.Len() > 0 {
			out = append(out, placedText{obj.X, obj.Y, sb.String()})
		}
	}
	return out
}

// xOf is the left edge of the first object on pg whose text contains s.
func xOf(t *testing.T, pg *document.Page, s string) bag.ScaledPoint {
	t.Helper()
	for _, pt := range pageTexts(pg) {
		if strings.Contains(pt.text, s) {
			return pt.x
		}
	}
	t.Fatalf("%q is not on the page", s)
	return 0
}

// An A6 page with 10mm margins has 85mm of content width: two columns with a
// 5mm gap are 40mm wide, the second starts at 55mm.
const columnsCSS = `@page { size: a6; margin: 10mm; } body { margin: 0; column-count: 2; column-gap: 5mm; }`

func requireX(t *testing.T, what string, got, want bag.ScaledPoint) {
	t.Helper()
	if d := got - want; d < -bag.MustSP("0.1pt") || d > bag.MustSP("0.1pt") {
		t.Errorf("%s at x %s, want %s", what, got, want)
	}
}

// The body's content fills the first column, then the second, then the next
// page; every line is as wide as a column.
func TestColumnsFillSideBySide(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&sb, "<p>Paragraph %d.</p>", i)
	}
	pages, _ := renderHTMLPagesCB(t, columnsCSS, "<html><body>"+sb.String()+"</body></html>")
	if len(pages) < 2 {
		t.Fatalf("%d pages, want at least 2", len(pages))
	}
	xs := map[bag.ScaledPoint]bool{}
	for _, pt := range pageTexts(pages[0]) {
		xs[pt.x] = true
	}
	if len(xs) != 2 || !xs[bag.MustSP("10mm")] {
		t.Errorf("page 1 holds text at %v, want 10mm and 55mm", xs)
	}
	requireX(t, "paragraph 1", xOf(t, pages[0], "Paragraph1."), bag.MustSP("10mm"))
	requireX(t, "the first paragraph of page 2", xOf(t, pages[1], "Paragraph"), bag.MustSP("10mm"))
	if w := maxLineWidth(pages[0]); w > bag.MustSP("40mm")+bag.MustSP("0.1pt") {
		t.Errorf("a line is %s wide, a column 40mm", w)
	}
	last := pageTexts(pages[0])
	requireX(t, "the last text of page 1", last[len(last)-1].x, bag.MustSP("55mm"))
}

// break-before: column moves on to the next column of the page, and does
// nothing without columns.
func TestColumnBreak(t *testing.T) {
	html := `<html><body><p>One</p><p style="break-before: column">Two</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, columnsCSS, html)
	if len(pages) != 1 {
		t.Fatalf("%d pages, want 1", len(pages))
	}
	requireX(t, "One", xOf(t, pages[0], "One"), bag.MustSP("10mm"))
	requireX(t, "Two", xOf(t, pages[0], "Two"), bag.MustSP("55mm"))

	pages, _ = renderHTMLPagesCB(t, `@page { size: a6; margin: 10mm; } body { margin: 0 }`, html)
	if len(pages) != 1 {
		t.Fatalf("without columns: %d pages, want 1", len(pages))
	}
	requireX(t, "Two without columns", xOf(t, pages[0], "Two"), bag.MustSP("10mm"))

	pages, _ = renderHTMLPagesCB(t, columnsCSS, `<html><body><p>One</p><p style="break-before: page">Two</p></body></html>`)
	if len(pages) != 2 {
		t.Fatalf("break-before: page in columns: %d pages, want 2", len(pages))
	}
}

// A footnote is set at the foot of the column that holds its call, as wide
// as the column.
func TestColumnFootnote(t *testing.T) {
	html := `<html><body><p>One</p><p style="break-before: column">Two<span class="footnote">The note.</span></p></body></html>`
	pages, _ := renderHTMLPagesCB(t, columnsCSS, html)
	if len(pages) != 1 {
		t.Fatalf("%d pages, want 1", len(pages))
	}
	requireX(t, "the footnote", xOf(t, pages[0], "Thenote."), bag.MustSP("55mm"))
}

// column-gap: normal is 1em of the body's font.
func TestColumnGapNormal(t *testing.T) {
	html := `<html><body><p>One</p><p style="break-before: column">Two</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, `@page { size: a6; margin: 10mm; } body { margin: 0; column-count: 2; font-size: 10pt }`, html)
	w := (bag.MustSP("85mm") - bag.MustSP("10pt")) / 2
	requireX(t, "Two", xOf(t, pages[0], "Two"), bag.MustSP("10mm")+w+bag.MustSP("10pt"))
}

// columnBottoms returns the lowest bottom edge of the text in each column of
// pg, keyed by the column's left edge.
func columnBottoms(pg *document.Page) map[bag.ScaledPoint]bag.ScaledPoint {
	out := map[bag.ScaledPoint]bag.ScaledPoint{}
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		var sb strings.Builder
		collectGlyphs(obj.Vlist.List, &sb)
		if sb.Len() == 0 {
			continue
		}
		bottom := obj.Y - obj.Vlist.Height - obj.Vlist.Depth
		if b, ok := out[obj.X]; !ok || bottom < b {
			out[obj.X] = bottom
		}
	}
	return out
}

// allText is the text of all pages in painting order.
func allText(pages []*document.Page) string {
	var sb strings.Builder
	for _, pg := range pages {
		for _, pt := range pageTexts(pg) {
			sb.WriteString(pt.text)
		}
	}
	return sb.String()
}

// The last row is balanced: content that fits into one column is spread over
// both, and every paragraph is set once, in order, also when the balancer's
// trial runs split them.
func TestColumnsBalance(t *testing.T) {
	var sb strings.Builder
	var want strings.Builder
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&sb, "<p>Paragraph %d %s</p>", i, strings.Repeat("word ", 30))
		fmt.Fprintf(&want, "Paragraph%d%s", i, strings.Repeat("word", 30))
	}
	pages, _ := renderHTMLPagesCB(t, columnsCSS, "<html><body>"+sb.String()+"</body></html>")
	if len(pages) != 1 {
		t.Fatalf("%d pages, want 1", len(pages))
	}
	bottoms := columnBottoms(pages[0])
	left, right := bottoms[bag.MustSP("10mm")], bottoms[bag.MustSP("55mm")]
	if left == 0 || right == 0 {
		t.Fatalf("text in the columns: %v, want both", bottoms)
	}
	if d := left - right; d < -bag.MustSP("15pt") || d > bag.MustSP("15pt") {
		t.Errorf("the columns end at %s and %s, more than a line apart", left, right)
	}
	if got := allText(pages); got != want.String() {
		t.Errorf("text after balancing:\n%s\nwant\n%s", got, want.String())
	}
}

// column-fill: auto fills the first column first, also on the last page.
func TestColumnsFillAuto(t *testing.T) {
	html := "<html><body><p>One</p><p>Two</p></body></html>"
	pages, _ := renderHTMLPagesCB(t, columnsCSS+" body { column-fill: auto }", html)
	requireX(t, "Two", xOf(t, pages[0], "Two"), bag.MustSP("10mm"))
}

// An element with column-span: all is as wide as the page and stands below
// the columns before it; the columns after it start below it.
func TestColumnSpanner(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, "<p>Before%d %s</p>", i, strings.Repeat("word ", 20))
	}
	sb.WriteString(`<div style="column-span: all">Spanner</div>`)
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, "<p>After%d %s</p>", i, strings.Repeat("word ", 20))
	}
	pages, _ := renderHTMLPagesCB(t, columnsCSS, "<html><body>"+sb.String()+"</body></html>")
	if len(pages) != 1 {
		t.Fatalf("%d pages, want 1", len(pages))
	}
	var spanner placedText
	for _, pt := range pageTexts(pages[0]) {
		if strings.HasPrefix(pt.text, "Spanner") {
			spanner = pt
		}
	}
	requireX(t, "the spanner", spanner.x, bag.MustSP("10mm"))
	if w := maxLineWidth(pages[0]); w < bag.MustSP("80mm") {
		t.Errorf("the widest line is %s, the spanner's should be 85mm", w)
	}
	for _, pt := range pageTexts(pages[0]) {
		switch {
		case strings.HasPrefix(pt.text, "Before") && pt.y <= spanner.y:
			t.Errorf("%q is not above the spanner", pt.text[:7])
		case strings.HasPrefix(pt.text, "After") && pt.y >= spanner.y:
			t.Errorf("%q is not below the spanner", pt.text[:6])
		}
	}
	requireX(t, "Before3", xOf(t, pages[0], "Before3"), bag.MustSP("55mm"))
	requireX(t, "After3", xOf(t, pages[0], "After3"), bag.MustSP("55mm"))
}

// A direct child of body with column-count sets its children in columns; its
// siblings span them.
func TestColumnsOnAChildOfBody(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<header><p>Title</p></header><main>")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, "<p>Text%d %s</p>", i, strings.Repeat("word ", 20))
	}
	sb.WriteString("</main>")
	css := `@page { size: a6; margin: 10mm; } body { margin: 0 } main { column-count: 2; column-gap: 5mm }`
	pages, _ := renderHTMLPagesCB(t, css, "<html><body>"+sb.String()+"</body></html>")
	requireX(t, "the title", xOf(t, pages[0], "Title"), bag.MustSP("10mm"))
	requireX(t, "Text1", xOf(t, pages[0], "Text1"), bag.MustSP("10mm"))
	requireX(t, "Text4", xOf(t, pages[0], "Text4"), bag.MustSP("55mm"))

	// With padding the child keeps its children in one column.
	pages, _ = renderHTMLPagesCB(t, css+" main { padding: 2pt }", "<html><body>"+sb.String()+"</body></html>")
	for _, pt := range pageTexts(pages[0]) {
		if pt.x > bag.MustSP("20mm") {
			t.Errorf("%q in a second column of a padded element", pt.text[:5])
		}
	}
}
