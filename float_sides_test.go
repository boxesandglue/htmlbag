package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

const (
	leftFloat60x40  = `<div style="float:left;width:60pt;height:40pt"></div>`
	rightFloat50x80 = `<div style="float:right;width:50pt;height:80pt"></div>`
)

// lineRightInsets returns the room each line leaves at its end: the line-end
// glue, which holds a right inset together with the space of a ragged line.
func lineRightInsets(v *node.VList) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.VList:
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					continue
				}
				walk(c.List)
			case *node.HList:
				if origin, _ := c.Attributes["origin"].(string); origin == "line" {
					var room bag.ScaledPoint
					for m := c.List; m != nil; m = m.Next() {
						if g, ok := m.(*node.Glue); ok {
							if o, _ := g.Attributes["origin"].(string); o == "lineend" {
								room += g.Width
							}
						}
					}
					out = append(out, room)
				}
				walk(c.List)
			}
		}
	}
	walk(v.List)
	return out
}

// A left and a right float written one after the other stand side by side,
// both level with the paragraph after them, and each narrows its own edge of
// the lines for its own height.
func TestALeftAndARightFloatStandSideBySide(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+`<p style="margin:0">`+floatProse+floatProse+`</p></div>`)
	if got, want := strings.Join(verticalOrder(vl), " "), "float float block"; got != want {
		t.Fatalf("got %q, want %q: the right float went below the left one", got, want)
	}
	indents, rights := lineIndents(vl), lineRightInsets(vl)
	if len(indents) < 10 {
		t.Fatalf("want a paragraph well past both floats, got %d lines", len(indents))
	}
	rightClear := bag.MustSP("50pt") + floatGutter
	if indents[0] == 0 || rights[0] < rightClear {
		t.Errorf("first line: indent %s, room at the end %s; want both floats cleared", indents[0], rights[0])
	}
	// Past the left float the lines start at the edge again while the right
	// one still narrows them, and past both they run the full measure.
	var rightOnly, full bool
	for i := range indents {
		if indents[i] == 0 && rights[i] >= rightClear {
			rightOnly = true
		}
		if indents[i] == 0 && rights[i] < rightClear {
			full = true
			if !rightOnly {
				t.Errorf("line %d runs the full measure before any line beside the right float alone", i)
			}
			break
		}
	}
	if !rightOnly || !full {
		t.Errorf("lines beside the right float alone: %v, full lines after them: %v; want both", rightOnly, full)
	}
}

// Two floats that leave no room for text between them do not stand side by
// side: the second goes below the first, as after a clear. 130pt and 60pt
// with their gutters take 208pt of the 200pt measure.
func TestFloatsThatLeaveNoRoomBetweenThemStack(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+
		`<div style="float:left;width:130pt;height:40pt"></div>`+
		`<div style="float:right;width:60pt;height:30pt"></div>`+
		`<p style="margin:0">a</p></div>`)
	if got, want := strings.Join(verticalOrder(vl), " "), "float kern:40 float block kern:18"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Skipping one band moves the cursor past the other one too: a second left
// float goes below the first, and the right float beside both is passed by
// as much, so that it ends where it does, 80pt down. Left at its full height
// after the skip, it would hold the container open 40pt below that.
func TestSkippingABandPassesTheOtherBy(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+
		`<div style="float:left;width:60pt;height:20pt"></div>`+
		`<p style="margin:0">a</p></div>`)
	if got, want := strings.Join(verticalOrder(vl), " "), "float float kern:40 float block kern:28"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := vl.Height+vl.Depth, bag.MustSP("80pt"); got != want {
		t.Errorf("container is %s tall, want the right float's %s", got, want)
	}
}

// clear: both goes below the taller of the two floats, clear: left only below
// the left one, with the right one still beside the cleared block.
func TestClearEndsTheBandsItNames(t *testing.T) {
	both := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+`<p style="margin:0;clear:both">a</p></div>`)
	if got, want := strings.Join(verticalOrder(both), " "), "float float kern:40 kern:40 block"; got != want {
		t.Errorf("clear: both: got %q, want %q", got, want)
	}
	left := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+`<p style="margin:0;clear:left">`+floatProse+`</p></div>`)
	if got, want := strings.Join(verticalOrder(left), " "), "float float kern:40 block"; got != want {
		t.Errorf("clear: left: got %q, want %q", got, want)
	}
	if indents, rights := lineIndents(left), lineRightInsets(left); len(indents) == 0 || indents[0] != 0 || rights[0] < bag.MustSP("50pt")+floatGutter {
		t.Errorf("clear: left: the first line must start at the edge and keep clear of the right float, indents %v, room at the end %v", indents, rights)
	}
}

// The margin a float lays out above itself moves the cursor down past the
// band of the float on the other side as well.
func TestAFloatsMarginPassesTheOtherBandBy(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+
		`<p style="margin:0 0 30pt 0">a</p>`+
		`<div style="float:right;width:50pt;height:10pt"></div>`+
		`<p style="margin:0">b</p></div>`)
	if got, want := strings.Join(verticalOrder(vl), " "), "float block kern:30 float block"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Two lines and the margin pass the left float: nothing overhangs.
	var content bag.ScaledPoint
	for n := vl.List; n != nil; n = n.Next() {
		content += vlistNodeHeight(n)
	}
	if vl.Height+vl.Depth > content {
		t.Errorf("container is %s tall for %s of content: the left band outlived the margin", vl.Height+vl.Depth, content)
	}
}

// A bare container passes both bands down to its paragraph, a bordered one is
// narrowed by both and moved by the left one.
func TestAContainerSitsBetweenBothFloats(t *testing.T) {
	bare := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+`<div><p style="margin:0">`+floatProse+`</p></div></div>`)
	if indents, rights := lineIndents(bare), lineRightInsets(bare); len(indents) == 0 || indents[0] == 0 || rights[0] < bag.MustSP("50pt")+floatGutter {
		t.Errorf("bare container: the first line must clear both floats, indents %v, room at the end %v", indents, rights)
	}
	bordered := buildHTML(t, floatBuilder(t), `<div>`+leftFloat60x40+rightFloat50x80+`<div style="border:1pt solid black"><p style="margin:0">a</p></div></div>`)
	var box *node.VList
	for _, n := range verticalChildren(bordered) {
		if v, ok := n.(*node.VList); ok {
			if o, _ := v.Attributes["origin"].(string); o != "float" {
				box = v
			}
		}
	}
	if box == nil {
		t.Fatal("no bordered box")
	}
	left, right := bag.MustSP("60pt")+floatGutter, bag.MustSP("50pt")+floatGutter
	if box.ShiftX != left || box.Width != bag.MustSP(floatMeasure)-left-right {
		t.Errorf("bordered box at %s, %s wide; want at %s, %s wide", box.ShiftX, box.Width, left, bag.MustSP(floatMeasure)-left-right)
	}
}

// verticalChildren is the list verticalOrder reads, as nodes.
func verticalChildren(vl *node.VList) []node.Node {
	for {
		inner, ok := vl.List.(*node.VList)
		if !ok || inner.Next() != nil {
			break
		}
		vl = inner
	}
	var out []node.Node
	for n := vl.List; n != nil; n = n.Next() {
		out = append(out, n)
	}
	return out
}

// A paragraph beside a left and a right float that a page break splits keeps
// its lines narrowed on both sides on the float's page, runs the full measure
// on the next, and loses no word in between: the rest is re-broken from the
// first page's lines, which the replay has to narrow on both sides as they
// were. The right float is the taller one, so the band that still covers
// lines past the break is not the left one.
func TestASplitParagraphBetweenTwoFloats(t *testing.T) {
	css := paginationCSS + ` .l { float: left; width: 40pt; height: 20pt; } .r { float: right; width: 50pt; height: 40pt; }`
	var words []string
	for i := 1; i <= 200; i++ {
		words = append(words, fmt.Sprintf("w%d", i))
	}
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="l"></div><div class="r"></div><p>` + strings.Join(words, " ") + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 1 {
		t.Fatalf("floats painted on page %d, want 1", got)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want the paragraph to continue on page 2", len(pages))
	}
	var got strings.Builder
	perPage := map[int][]placedLine{}
	for _, l := range textLines(pages) {
		if len(l.text) > 1 && l.text[0] == 'w' && l.text[1] >= '0' && l.text[1] <= '9' {
			got.WriteString(l.text)
			perPage[l.page] = append(perPage[l.page], l)
		}
	}
	if want := strings.Join(words, ""); got.String() != want {
		t.Errorf("the paragraph lost or repeated words at the page break:\ngot  %s\nwant %s", got.String(), want)
	}
	if len(perPage[1]) == 0 || len(perPage[2]) == 0 {
		t.Fatalf("lines per page: %d on page 1, %d on page 2; want some on each", len(perPage[1]), len(perPage[2]))
	}
	// Short words leave less than the right float's inset at a line end;
	// the paragraph's last line may leave more.
	rightClear := bag.MustSP("50pt") + floatGutter
	var p1, p2 []bag.ScaledPoint
	for _, obj := range pages[0].Objects {
		if obj.Vlist != nil {
			p1 = append(p1, lineRightInsets(obj.Vlist)...)
		}
	}
	for _, obj := range pages[1].Objects {
		if obj.Vlist != nil {
			p2 = append(p2, lineRightInsets(obj.Vlist)...)
		}
	}
	if len(p1) == 0 || p1[len(p1)-1] < rightClear {
		t.Errorf("page 1: the last line, beside the right float, leaves %v at its end, want at least %s", p1[len(p1)-1:], rightClear)
	}
	for i, room := range p2[:len(p2)-1] {
		if room >= rightClear {
			t.Errorf("page 2: line %d leaves %s at its end, narrowed beside a float that is on page 1", i, room)
		}
	}
	if n := countIndented(pageLineIndents(pages[1])); n > 0 {
		t.Errorf("page 2: %d lines are narrowed beside floats that are on page 1", n)
	}
}

// Two floats side by side go to the next page together with the paragraph
// beside them when the taller one does not fit: the shorter one must not
// stay behind alone at the bottom of the page.
func TestFloatsSideBySideMoveOnTogether(t *testing.T) {
	css := paginationCSS + ` .l { float: left; width: 40pt; height: 40pt; } .r { float: right; width: 50pt; height: 60pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="l"></div><div class="r"></div><p>` + floatProse + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 2 {
		t.Errorf("first float painted on page %d, want 2 with the other one", got)
	}
	assertFloatsKeepTheirText(t, pages)
}
