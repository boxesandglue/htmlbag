package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// pageFiller returns n plain paragraphs of three lines each; an A4 page with
// 20mm margins holds twenty of them.
func pageFiller(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("<p>" + floatProse + "</p>")
	}
	return sb.String()
}

// logicalFloatBoxes returns the inside/outside float boxes painted on a page.
func logicalFloatBoxes(pg *document.Page) []*node.VList {
	var out []*node.VList
	var walk func(vl *node.VList)
	walk = func(vl *node.VList) {
		if _, ok := vl.Attributes[attrFloatLogical]; ok {
			out = append(out, vl)
		}
		for n := vl.List; n != nil; n = n.Next() {
			if child, ok := n.(*node.VList); ok {
				walk(child)
			}
		}
	}
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			walk(obj.Vlist)
		}
	}
	return out
}

// pageLineIndents collects the leftskip of every line painted on a page.
func pageLineIndents(pg *document.Page) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			out = append(out, lineIndents(obj.Vlist)...)
		}
	}
	return out
}

// Page one is a right page and the parity alternates from there: the second
// page takes the :left rule. Counting the document's page list alone made
// every even page a right one from page two on.
func TestLeftAndRightPageRulesAlternate(t *testing.T) {
	css := `@page { size: a4; margin: 20mm; } @page :left { margin-left: 60mm; } p { margin: 0; }`
	pages, _ := renderHTMLPagesCB(t, css, `<!DOCTYPE html><html><body>`+pageFiller(60)+`</body></html>`)
	if len(pages) < 3 {
		t.Fatalf("got %d pages, want at least 3", len(pages))
	}
	want := []bag.ScaledPoint{bag.MustSP("20mm"), bag.MustSP("60mm"), bag.MustSP("20mm")}
	for i, w := range want {
		if len(pages[i].Objects) == 0 {
			t.Fatalf("page %d has no content", i+1)
		}
		if got := pages[i].Objects[0].X; got != w {
			t.Errorf("page %d: content starts at %s, want %s", i+1, got, w)
		}
	}
}

// On the first page, which is a right page, outside is the right edge and
// inside the left one; the margins are read as written.
func TestLogicalFloatSidesOnARightPage(t *testing.T) {
	wd := bag.MustSP(floatMeasure)
	cb := floatBuilder(t)
	outside := buildHTML(t, cb, `<div><div style="float:outside;width:60pt;height:40pt;margin-right:-20pt"></div><p>`+floatProse+`</p></div>`)
	boxes := logicalFloatBoxes(&document.Page{Objects: []document.Object{{Vlist: outside}}})
	if len(boxes) != 1 {
		t.Fatalf("got %d logical float boxes, want 1", len(boxes))
	}
	if want := wd - bag.MustSP("60pt") + bag.MustSP("20pt"); boxes[0].ShiftX != want {
		t.Errorf("outside on a right page: shift %s, want %s (right edge, margin-right as written)", boxes[0].ShiftX, want)
	}
	inside := buildHTML(t, floatBuilder(t), `<div><div style="float:inside;width:60pt;height:40pt;margin-left:5pt"></div><p>`+floatProse+`</p></div>`)
	boxes = logicalFloatBoxes(&document.Page{Objects: []document.Object{{Vlist: inside}}})
	if len(boxes) != 1 {
		t.Fatalf("got %d logical float boxes, want 1", len(boxes))
	}
	if want := bag.MustSP("5pt"); boxes[0].ShiftX != want {
		t.Errorf("inside on a right page: shift %s, want %s (left edge, margin-left as written)", boxes[0].ShiftX, want)
	}
	if in := lineIndents(inside); len(in) == 0 || in[0] == 0 {
		t.Errorf("inside on a right page must narrow the text from the left, first line indent %v", in)
	}
}

// A margin note built for a right page and painted on a left one moves to the
// left edge with its margins mirrored: what was declared for the outer edge on
// the right page is the outer edge on the left page too.
func TestLogicalFloatFollowsThePageItIsPaintedOn(t *testing.T) {
	wd := bag.MustSP(floatMeasure)
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:outside;width:40pt;height:20pt;margin-left:10pt;margin-right:-50pt"></div><p>`+floatProse+`</p></div>`)
	boxes := logicalFloatBoxes(&document.Page{Objects: []document.Object{{Vlist: vl}}})
	if len(boxes) != 1 {
		t.Fatalf("got %d logical float boxes, want 1", len(boxes))
	}
	box := boxes[0]
	if want := wd - bag.MustSP("40pt") + bag.MustSP("50pt"); box.ShiftX != want {
		t.Fatalf("built for a right page: shift %s, want %s", box.ShiftX, want)
	}
	fixLogicalFloats(vl, false)
	if want := bag.MustSP("-50pt"); box.ShiftX != want {
		t.Errorf("painted on a left page: shift %s, want %s (the declared margin-right becomes margin-left)", box.ShiftX, want)
	}
	// Painting the same box on a right page again restores the shift.
	fixLogicalFloats(vl, true)
	if want := wd - bag.MustSP("40pt") + bag.MustSP("50pt"); box.ShiftX != want {
		t.Errorf("back on a right page: shift %s, want %s", box.ShiftX, want)
	}
}

// Through the whole pipeline: the same margin note hangs into the right margin
// on page one and into the left margin on page two, without the author saying
// which page it is on.
func TestMarginNoteHangsOutsideOnBothPages(t *testing.T) {
	css := `@page { size: a4; margin: 20mm 50mm 20mm 20mm; } @page :left { margin: 20mm 20mm 20mm 50mm; }
		p { margin: 0; }
		.note { float: outside; width: 25mm; margin-left: 5mm; margin-right: -30mm; }`
	note := `<div class="note">note</div><p>` + floatProse + `</p>`
	pages, _ := renderHTMLPagesCB(t, css, `<!DOCTYPE html><html><body>`+note+pageFiller(22)+note+`</body></html>`)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	contentWidth := bag.MustSP("210mm") - bag.MustSP("70mm")
	first := logicalFloatBoxes(pages[0])
	if len(first) != 1 {
		t.Fatalf("page 1: got %d notes, want 1", len(first))
	}
	if want := contentWidth - bag.MustSP("25mm") + bag.MustSP("30mm"); first[0].ShiftX != want {
		t.Errorf("page 1 (right): note shift %s, want %s", first[0].ShiftX, want)
	}
	second := logicalFloatBoxes(pages[1])
	if len(second) != 1 {
		t.Fatalf("page 2: got %d notes, want 1", len(second))
	}
	if want := bag.MustSP("-30mm"); second[0].ShiftX != want {
		t.Errorf("page 2 (left): note shift %s, want %s", second[0].ShiftX, want)
	}
	// The note must not narrow the paragraph: with margin-right cancelling
	// width and gutter, the inset is zero on both pages.
	for i, pg := range pages[:2] {
		for j, in := range pageLineIndents(pg) {
			if in != 0 {
				t.Errorf("page %d line %d indented by %s: a margin note must leave the text alone", i+1, j, in)
				break
			}
		}
	}
}

// A float that narrows the text and lands on a page of the other parity is
// rebuilt there, so the lines give way on the side the float actually is on:
// the outside of page two is the left edge, and the lines start indented.
func TestOutsideFloatNarrowsTheTextOnItsOwnSide(t *testing.T) {
	css := `@page { size: a4; margin: 20mm; } p { margin: 0; }
		.fig { float: outside; width: 60pt; height: 40pt; }`
	fig := `<div class="fig"></div><p>` + floatProse + `</p>`
	pages, _ := renderHTMLPagesCB(t, css, `<!DOCTYPE html><html><body>`+fig+pageFiller(22)+fig+`</body></html>`)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(pages))
	}
	if in := pageLineIndents(pages[0]); len(in) == 0 || in[0] != 0 {
		t.Errorf("page 1 (right): outside is the right edge, the first line must start at 0, got %v", in[:min(len(in), 3)])
	}
	boxes := logicalFloatBoxes(pages[1])
	if len(boxes) != 1 {
		t.Fatalf("page 2: got %d floats, want 1", len(boxes))
	}
	if boxes[0].ShiftX != 0 {
		t.Errorf("page 2 (left): float shift %s, want 0", boxes[0].ShiftX)
	}
	// The lines beside the float on page two: some after the filler's last
	// full-width lines, find the first indented one.
	var indented bool
	for _, in := range pageLineIndents(pages[1]) {
		if in > 0 {
			indented = true
			break
		}
	}
	if !indented {
		t.Errorf("page 2 (left): no line is indented from the left beside the outside float")
	}
}

// clear names the side as inside/outside too, resolved against the same page
// as the float.
func TestClearOutsideEndsAnOutsideBand(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:outside;width:60pt;height:120pt"></div><p>one</p><p style="clear:outside">two</p></div>`)
	widths := lineContentWidths(vl)
	if len(widths) < 2 {
		t.Fatalf("got %d lines, want 2", len(widths))
	}
	// "two" is set below the float: the band no longer applies, so the line
	// content is the word alone, not shortened by an inset. Compare with a
	// version without clear, where the second line is still beside the float.
	beside := buildHTML(t, floatBuilder(t), `<div><div style="float:outside;width:60pt;height:120pt"></div><p>one</p><p>two</p></div>`)
	if vl.Height <= beside.Height {
		t.Errorf("clear:outside must push the second paragraph below the float: height %s with clear, %s without", vl.Height, beside.Height)
	}
}

// verticalOrder lists the body-level children of a built tree as compact
// tokens: "kern:<pt>" for margin kerns (the first block's own top margin
// included), "float" for a float box and "block"
// for anything else with height, so a test can state where a float sits
// relative to the margins around it.
func verticalOrder(vl *node.VList) []string {
	// Descend the single-child wrappers (html, body) to the first list with
	// more than one child.
	for {
		inner, ok := vl.List.(*node.VList)
		if !ok || inner.Next() != nil {
			break
		}
		vl = inner
	}
	var out []string
	for n := vl.List; n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.Kern:
			out = append(out, "kern:"+v.Kern.String())
		case *node.VList:
			if o, _ := v.Attributes["origin"].(string); o == "float" {
				out = append(out, "float")
			} else {
				out = append(out, "block")
			}
		}
	}
	return out
}

// A float between two blocks sits below the previous block's bottom margin,
// and the next block's top margin collapses with it. With equal margins the
// float starts level with the block after it; a larger top margin on the
// next block adds only the difference after the float.
func TestFloatSitsBelowThePreviousMargin(t *testing.T) {
	cb := floatBuilder(t)
	equal := buildHTML(t, cb, `<p style="margin:6pt 0">a</p><div style="float:outside;width:40pt;margin-right:-49pt">n</div><p style="margin:6pt 0">b</p>`)
	if got, want := strings.Join(verticalOrder(equal), " "), "kern:6 block kern:6 float block"; got != want {
		t.Errorf("equal margins: got %q, want %q", got, want)
	}
	larger := buildHTML(t, floatBuilder(t), `<p style="margin:6pt 0">a</p><div style="float:outside;width:40pt;margin-right:-49pt">n</div><h2 style="margin:14pt 0 0 0">b</h2>`)
	if got, want := strings.Join(verticalOrder(larger), " "), "kern:6 block kern:6 float kern:8 block"; got != want {
		t.Errorf("larger top margin after the float: got %q, want %q", got, want)
	}
	smaller := buildHTML(t, floatBuilder(t), `<h1 style="margin:0 0 12pt 0">a</h1><div style="float:outside;width:40pt;margin-right:-49pt">n</div><p style="margin:6pt 0">b</p>`)
	if got, want := strings.Join(verticalOrder(smaller), " "), "block kern:12 float block"; got != want {
		t.Errorf("smaller top margin after the float: got %q, want %q", got, want)
	}
}

// A margin note takes nothing from the text, so a figure written right after
// it starts level with it and with the paragraph both precede; the lines
// beside the figure still give way to it. Two notes in a row would overlap,
// so they keep stacking.
func TestMarginNoteAndFigureShareTheirPosition(t *testing.T) {
	cb := floatBuilder(t)
	note := `<div style="float:outside;width:40pt;margin-left:5pt;margin-right:-45pt;height:30pt">n</div>`
	// The figure floats inside, the left edge on this first page, so the
	// narrowing shows up as a line indent.
	fig := `<div style="float:inside;width:60pt;height:40pt"></div>`
	vl := buildHTML(t, cb, `<p style="margin:6pt 0">a</p>`+note+fig+`<p style="margin:6pt 0">`+floatProse+`</p>`)
	if got, want := strings.Join(verticalOrder(vl), " "), "kern:6 block kern:6 float float block"; got != want {
		t.Errorf("note then figure: got %q, want %q", got, want)
	}
	if in := lineIndents(vl); len(in) < 2 || in[1] == 0 {
		t.Errorf("the lines beside the figure must still be narrowed, indents %v", in)
	}
	stacked := buildHTML(t, floatBuilder(t), `<p style="margin:6pt 0">a</p>`+note+note+`<p style="margin:6pt 0">b</p>`)
	// The trailing kern is the second note's overhang below the one-line
	// paragraph: the container still has to hold the box.
	if got, want := strings.Join(verticalOrder(stacked), " "), "kern:6 block kern:6 float kern:30 float block kern:18"; got != want {
		t.Errorf("two notes: got %q, want %q", got, want)
	}
	// The container still holds a note taller than the text beside it.
	tall := buildHTML(t, floatBuilder(t), `<div>`+`<div style="float:outside;width:40pt;margin-left:5pt;margin-right:-45pt;height:200pt">n</div>`+`<p style="margin:0">one line</p></div>`)
	if tall.Height+tall.Depth < bag.MustSP("200pt") {
		t.Errorf("container is %s tall, must hold the 200pt note", tall.Height+tall.Depth)
	}
}
