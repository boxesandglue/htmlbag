package htmlbag

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// These tests pin down where the paginator puts things today, at page breaks,
// so that moving it onto regions can be checked against them.

// A 200pt square page with 20pt margins: the content area is 160pt high, its
// top edge at y = 180pt, and a line is 12pt.
const charCSS = `@page { size: 200pt 200pt; margin: 20pt; }
body { margin: 0; }
p, h1, table { margin: 0; font-size: 10pt; line-height: 12pt; }
td, th { padding: 0; }`

var (
	charTop  = bag.MustSP("180pt")
	charLine = bag.MustSP("12pt")
)

// placedLine is a line, or a table row, as painted.
type placedLine struct {
	page               int
	top, bottom, width bag.ScaledPoint
	text               string
}

// placedLines lists every line and table row of the pages with its glyphs
// and its vertical extent, in painting order. Lines inside a row are part of
// the row.
func placedLines(pages []*document.Page) []placedLine {
	var out []placedLine
	var walk func(n node.Node, y bag.ScaledPoint, pg int)
	walk = func(n node.Node, y bag.ScaledPoint, pg int) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walk(v.List, y, pg)
				y -= v.Height + v.Depth
			case *node.HList:
				var sb strings.Builder
				collectGlyphs(v.List, &sb)
				if sb.Len() > 0 {
					out = append(out, placedLine{pg, y, y - v.Height - v.Depth, v.Width, sb.String()})
				}
				y -= v.Height + v.Depth
			case *node.Kern:
				y -= v.Kern
			case *node.Glue:
				y -= v.Width
			}
		}
	}
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist, obj.Y, i+1)
			}
		}
	}
	return out
}

// firstLine returns the first placed line whose text contains mark.
func firstLine(t *testing.T, lines []placedLine, mark string) placedLine {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l.text, mark) {
			return l
		}
	}
	t.Fatalf("no line contains %q", mark)
	return placedLine{}
}

// charLines is a paragraph body of n lines, the first starting with mark.
func charLines(mark string, n int) string {
	s := []string{mark}
	for i := 1; i < n; i++ {
		s = append(s, "x")
	}
	return strings.Join(s, "<br>")
}

// A margin at an automatic page break is spent at the foot of the page when
// it fits there, so the block after it starts flush with the next page top.
// A margin that does not fit is truncated at the top of the next page (CSS
// Fragmentation 3 §5.2), so the block starts flush there too. After a forced
// break, and at the top of the document, the margin is kept.
func TestCharacterizeMarginAtPageTop(t *testing.T) {
	five := bag.MustSP("5pt")
	cases := []struct {
		name, body string
		page       int
		top        bag.ScaledPoint
	}{
		{"first block", `<p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`, 1, charTop - five},
		{"margin fits at the foot", `<p>` + charLines("A", 12) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`, 2, charTop},
		{"margin does not fit", `<p>` + charLines("A", 13) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`, 2, charTop},
		{"break-before", `<p>` + charLines("A", 3) + `</p><p style="margin-top: 5pt; break-before: page">` + charLines("B", 2) + `</p>`, 2, charTop - five},
		{"break-after", `<p style="break-after: page">` + charLines("A", 3) + `</p><p style="margin-top: 5pt">` + charLines("B", 2) + `</p>`, 2, charTop - five},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pages, _ := renderHTMLPagesCB(t, charCSS, `<html><body>`+c.body+`</body></html>`)
			b := firstLine(t, placedLines(pages), "B")
			if b.page != c.page || b.top != c.top {
				t.Errorf("B starts on page %d at %s, want page %d at %s", b.page, b.top, c.page, c.top)
			}
		})
	}
}

// A footnote goes to the page its call is on, and the body never reaches
// into the footnote area below it. The paragraphs are one line each, so none
// of them takes the split path.
func TestCharacterizeFootnotesAtPageBreaks(t *testing.T) {
	var body strings.Builder
	for i := range 24 {
		fmt.Fprintf(&body, `<p>P%dq<fn>N%dq</fn></p>`, i, i)
	}
	pages, _ := renderHTMLPagesCB(t, charCSS, `<html><body>`+body.String()+`</body></html>`)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want the footnotes spread over at least 3", len(pages))
	}
	lines := placedLines(pages)
	for i := range 24 {
		p := firstLine(t, lines, fmt.Sprintf("P%dq", i))
		n := firstLine(t, lines, fmt.Sprintf("N%dq", i))
		if p.page != n.page {
			t.Errorf("footnote %d is on page %d, its call on page %d", i, n.page, p.page)
		}
	}
	for pg := 1; pg <= len(pages); pg++ {
		var lowestBody, highestNote bag.ScaledPoint = charTop, -1
		first := true
		for _, l := range lines {
			if l.page != pg {
				continue
			}
			switch {
			case strings.Contains(l.text, "N"):
				highestNote = max(highestNote, l.top)
			default:
				if first && l.top != charTop {
					t.Errorf("page %d: the body starts at %s, want the content top %s", pg, l.top, charTop)
				}
				first = false
				lowestBody = min(lowestBody, l.bottom)
			}
		}
		if highestNote >= 0 && lowestBody < highestNote {
			t.Errorf("page %d: the body reaches down to %s, below the top of a footnote at %s", pg, lowestBody, highestNote)
		}
	}
}

// Top and bottom floats at page breaks: on every page the body runs between
// the top-float stack and the bottom-float stack, nothing overlaps, and every
// float is painted once.
func TestCharacterizeTopAndBottomFloatsAtPageBreaks(t *testing.T) {
	var body strings.Builder
	floats := 0
	for i := range 14 {
		switch i % 4 {
		case 1:
			fmt.Fprintf(&body, `<p>P%dq<span style="float: top">T%dq<br>x</span><br>x<br>x</p>`, i, i)
			floats++
		case 3:
			fmt.Fprintf(&body, `<p>P%dq<span style="float: bottom">U%dq</span><br>x<br>x</p>`, i, i)
			floats++
		default:
			fmt.Fprintf(&body, `<p>P%dq<br>x<br>x</p>`, i)
		}
	}
	pages, _ := renderHTMLPagesCB(t, charCSS, `<html><body>`+body.String()+`</body></html>`)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want at least 3", len(pages))
	}
	lines := placedLines(pages)
	painted := 0
	for pg := 1; pg <= len(pages); pg++ {
		var onPage []placedLine
		for _, l := range lines {
			if l.page == pg {
				onPage = append(onPage, l)
			}
		}
		sort.SliceStable(onPage, func(i, j int) bool { return onPage[i].top > onPage[j].top })
		var order strings.Builder
		for i, l := range onPage {
			kind := byte('P')
			if l.text[0] == 'T' || l.text[0] == 'U' {
				kind = l.text[0]
				painted++
			}
			if l.text == "x" && order.Len() > 0 {
				kind = order.String()[order.Len()-1]
			}
			order.WriteByte(kind)
			if i > 0 && l.top > onPage[i-1].bottom {
				t.Errorf("page %d: %q at %s overlaps %q above it", pg, l.text, l.top, onPage[i-1].text)
			}
		}
		if !isTopBodyBottom(order.String()) {
			t.Errorf("page %d: from the top down %q, want top floats, then the body, then bottom floats", pg, order.String())
		}
	}
	if painted != floats {
		t.Errorf("%d floats painted, want %d", painted, floats)
	}
}

// isTopBodyBottom reports whether s is a run of T, then P, then U.
func isTopBodyBottom(s string) bool {
	s = strings.TrimLeft(s, "T")
	s = strings.TrimLeft(s, "P")
	return strings.Trim(s, "U") == ""
}

// A table that starts below a heading and breaks: its rows follow the heading
// directly, the header row repeats at the top of every later page, and the
// paragraph after the table starts right below its last row.
func TestCharacterizeTableAfterHeadingBreaks(t *testing.T) {
	var rows strings.Builder
	for i := range 30 {
		fmt.Fprintf(&rows, `<tr><td>R%d</td></tr>`, i)
	}
	html := `<html><body><h1>Heading</h1><table><thead><tr><th>Head</th></tr></thead><tbody>` +
		rows.String() + `</tbody></table><p>After</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, charCSS, html)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want the table over at least 3", len(pages))
	}
	lines := placedLines(pages)
	h := firstLine(t, lines, "Heading")
	if h.page != 1 || h.top != charTop {
		t.Errorf("heading on page %d at %s, want page 1 at %s", h.page, h.top, charTop)
	}
	var prev placedLine
	heads := 0
	for _, l := range lines {
		switch {
		case l.text == "Head":
			heads++
			want := charTop
			if l.page == 1 {
				want = h.bottom
			}
			if l.top != want {
				t.Errorf("header row on page %d at %s, want %s", l.page, l.top, want)
			}
		case strings.HasPrefix(l.text, "R"), l.text == "After":
			if l.page == prev.page && l.top != prev.bottom {
				t.Errorf("%q on page %d at %s, want it right below %q at %s", l.text, l.page, l.top, prev.text, prev.bottom)
			}
		default:
			continue
		}
		prev = l
	}
	if heads != len(pages) {
		t.Errorf("the header row is painted %d times, want once on each of the %d pages", heads, len(pages))
	}
	if after := firstLine(t, lines, "After"); after.page != len(pages) {
		t.Errorf("the paragraph after the table is on page %d, want the table's last page %d", after.page, len(pages))
	}
}

// A table row that breaks across the switch from a narrow first page to a
// wider page: the header rows and the whole rows after the break are rebuilt
// at the new width, the rest of the split row keeps the width it was broken
// at.
func TestCharacterizeRowSplitAcrossAWidthChange(t *testing.T) {
	css := charCSS + `@page :first { margin-right: 60pt; } table { width: 100%; }`
	var rows strings.Builder
	for i := range 6 {
		fmt.Fprintf(&rows, `<tr><td>R%dq</td></tr>`, i)
	}
	rows.WriteString(`<tr style="break-inside: auto"><td>Sq<br>` + charLines("x", 7) + `</td></tr>`)
	for i := 6; i < 10; i++ {
		fmt.Fprintf(&rows, `<tr><td>R%dq</td></tr>`, i)
	}
	html := `<html><body><table><thead><tr><th>Head</th></tr></thead><tbody>` + rows.String() + `</tbody></table></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	narrow, wide := bag.MustSP("120pt"), bag.MustSP("160pt")
	var split []placedLine
	for _, l := range placedLines(pages) {
		want := narrow
		if l.page == 2 && !strings.HasPrefix(l.text, "x") {
			want = wide
		}
		if strings.HasPrefix(l.text, "Sq") || strings.HasPrefix(l.text, "x") {
			split = append(split, l)
		}
		if l.width != want {
			t.Errorf("%q on page %d is %s wide, want %s", l.text, l.page, l.width, want)
		}
	}
	if len(split) != 2 || split[0].page != 1 || split[1].page != 2 {
		t.Errorf("the row with break-inside: auto is in %d parts, want one on each page", len(split))
	}
}

// Headings record the page they are painted on and the top of their box,
// which the outline uses for its /XYZ destinations.
func TestCharacterizeHeadingPagesAndPositions(t *testing.T) {
	var body strings.Builder
	for i := range 8 {
		fmt.Fprintf(&body, `<h1>H%d</h1><p>%s</p>`, i, charLines("x", 3+i%4*3))
	}
	pages, cb := renderHTMLPagesCB(t, charCSS, `<html><body>`+body.String()+`</body></html>`)
	lines := placedLines(pages)
	if len(cb.Headings) != 8 {
		t.Fatalf("got %d headings, want 8", len(cb.Headings))
	}
	for i, h := range cb.Headings {
		l := firstLine(t, lines, fmt.Sprintf("H%d", i))
		if h.Page != l.page || h.Y != l.top {
			t.Errorf("heading %d recorded on page %d at %s, painted on page %d at %s", i, h.Page, h.Y, l.page, l.top)
		}
	}
}

// Every forced break keyword is a plain page break: the paginator inserts no
// blank page to reach a left or a right one.
func TestCharacterizeForcedBreakKeywords(t *testing.T) {
	html := `<html><body><p>A</p><p style="break-before: left">B</p><p style="break-before: right">C</p>` +
		`<p style="break-before: right">D</p><p style="break-after: left">E</p><p>F</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, charCSS, html)
	lines := placedLines(pages)
	for mark, want := range map[string]int{"A": 1, "B": 2, "C": 3, "D": 4, "E": 4, "F": 5} {
		if l := firstLine(t, lines, mark); l.page != want {
			t.Errorf("%s is on page %d, want %d", mark, l.page, want)
		}
	}
	if len(pages) != 5 {
		t.Errorf("got %d pages, want 5", len(pages))
	}
}

// Nothing of a page is left in the builder once the pages are out.
func TestCharacterizeBuilderStateAfterPagination(t *testing.T) {
	var body strings.Builder
	for i := range 10 {
		fmt.Fprintf(&body, `<p>P%dq<fn>N%dq</fn><span style="float: top">T%dq</span><br>x<br>x</p>`, i, i, i)
	}
	body.WriteString(`<div style="position: absolute; top: 5pt; left: 5pt">Abs</div>`)
	_, cb := renderHTMLPagesCB(t, charCSS, `<html><body>`+body.String()+`</body></html>`)
	if len(cb.pageBuf) != 0 || cb.pageBufHeight != 0 {
		t.Errorf("page buffer holds %d entries (%s)", len(cb.pageBuf), cb.pageBufHeight)
	}
	for class, ins := range cb.pageInserts {
		if len(ins) > 0 {
			t.Errorf("%d inserts of class %d are left", len(ins), class)
		}
	}
	for class, h := range cb.pageInsertHeight {
		if h != 0 {
			t.Errorf("insert height %s of class %d is left", h, class)
		}
	}
	if len(cb.positionedItems) != 0 {
		t.Errorf("%d positioned items are left", len(cb.positionedItems))
	}
	if cb.fragLines != nil {
		t.Errorf("the widows/orphans map is left: %v", cb.fragLines)
	}
}
