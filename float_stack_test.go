package htmlbag

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// Floats on one side next to each other (CSS 2.1 §9.5.1). The expected
// values are Chrome's, for floats with a declared margin towards the text,
// which bag's gutter does not replace: the second float starts at the outer
// edge of the first one's margin box, and the lines beside both give way to
// the outer one.

// stackPara is a paragraph set at 12pt line distance, so that its rows start
// 12pt apart as in the Chrome probe.
func stackPara(words int) string {
	return `<p style="margin:0;font-size:10pt;line-height:12pt">` + strings.TrimSpace(strings.Repeat("aa ", words)) + `</p>`
}

// floatShifts returns where every float box sits, in document order,
// measured from the left edge of v.
func floatShifts(v *node.VList) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(n node.Node, x bag.ScaledPoint)
	walk = func(n node.Node, x bag.ScaledPoint) {
		for e := n; e != nil; e = e.Next() {
			if c, ok := e.(*node.VList); ok {
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					out = append(out, x+c.ShiftX)
					continue
				}
				walk(c.List, x+c.ShiftX)
			}
		}
	}
	walk(v.List, 0)
	return out
}

func pts(values ...float64) []bag.ScaledPoint {
	out := make([]bag.ScaledPoint, len(values))
	for i, v := range values {
		out[i] = bag.MustSP(fmt.Sprintf("%gpt", v))
	}
	return out
}

// assertPrefix checks the first values of got.
func assertPrefix(t *testing.T, what string, got, want []bag.ScaledPoint) {
	t.Helper()
	if len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
		t.Errorf("%s = %v, want it to start with %v", what, got, want)
	}
}

const (
	left60x40 = `<div style="float:left;width:60pt;height:40pt;margin-right:9pt"></div>`
	left60x20 = `<div style="float:left;width:60pt;height:20pt;margin-right:9pt"></div>`
)

// The shorter float outside: the lines beside both clear the outer one, the
// lines below its end the inner one, and then the text runs full width.
func TestASecondFloatStandsBesideTheFirst(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+left60x40+left60x20+stackPara(60)+`</div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 69))
	assertPrefix(t, "line indents", lineIndents(vl), pts(138, 138, 69, 69, 0, 0))
	if got := strings.Join(verticalOrder(vl), " "); got != "float float block" {
		t.Errorf("got %q, want both floats level with the paragraph", got)
	}
}

// The taller float outside: it stays where it is when the inner one ends,
// and so do the lines beside it.
func TestTheOuterFloatHoldsTheLinesUntilItEnds(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+left60x20+left60x40+stackPara(60)+`</div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 69))
	assertPrefix(t, "line indents", lineIndents(vl), pts(138, 138, 138, 138, 0, 0))
}

// The same on the right: the second float stands left of the first one's
// margin box, and the line ends give way by both. Justified, so that the
// line-end glue is the inset and nothing else.
func TestRightFloatsStandNextToEachOther(t *testing.T) {
	right := func(h int) string {
		return fmt.Sprintf(`<div style="float:right;width:50pt;height:%dpt;margin-left:9pt"></div>`, h)
	}
	para := strings.Replace(stackPara(120), `margin:0;`, `margin:0;text-align:justify;`, 1)
	vl := buildHTML(t, floatBuilder(t), `<div>`+right(40)+right(20)+para+`</div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(150, 91))
	assertPrefix(t, "room at the line ends", lineRightInsets(vl), pts(118, 118, 59, 59, 0, 0))
}

// A float opening after the one at the edge has ended still stands beside
// the one that has not: the space the first one leaves is not filled (CSS
// 2.1 §9.5.1, rule 2). The lines beside the new float clear it, those below
// it the float it stands beside.
func TestALaterFloatStandsBesideTheOneStillThere(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+left60x40+
		`<div style="float:left;width:40pt;height:80pt;margin-right:9pt"></div>`+
		`<div style="height:48pt"></div>`+
		`<div style="float:left;width:30pt;height:20pt;margin-right:9pt"></div>`+
		stackPara(60)+`</div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 69, 118))
	assertPrefix(t, "line indents", lineIndents(vl), pts(157, 157, 118, 0))
}

// A float in a container beside a float stands next to it too: the band
// passed down carries where the outer float's margin box ends.
func TestAFloatInANestedContainerStandsBesideTheOuterOne(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+left60x40+`<div>`+
		`<div style="float:left;width:30pt;height:20pt;margin-right:9pt"></div>`+
		stackPara(60)+`</div></div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 69))
	assertPrefix(t, "line indents", lineIndents(vl), pts(108, 108, 69, 69, 0))
}

// Without a margin the gutter keeps the text off the float, but not the
// next float: two floats without margins touch, as in a browser.
func TestTheGutterIsKeptFromTheTextOnly(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+
		`<div style="float:left;width:60pt;height:40pt"></div>`+
		`<div style="float:left;width:60pt;height:20pt"></div>`+
		stackPara(60)+`</div>`)
	g := floatGutter
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 60))
	assertPrefix(t, "line indents", lineIndents(vl), []bag.ScaledPoint{120*bag.Factor + g, 120*bag.Factor + g, 60*bag.Factor + g})
}

// A paragraph beside two floats on one side that a page break splits loses
// no word: the rest is re-broken from the first page's lines, which the
// replay has to narrow step by step as they were. The inner float is the
// taller one, so the lines on page 1 come in two widths.
func TestASplitParagraphBesideStackedFloats(t *testing.T) {
	css := paginationCSS + ` .a { float: left; width: 40pt; height: 40pt; margin-right: 9pt } .b { float: left; width: 40pt; height: 20pt; margin-right: 9pt }`
	var words []string
	for i := 1; i <= 200; i++ {
		words = append(words, fmt.Sprintf("w%d", i))
	}
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="a"></div><div class="b"></div><p>` + strings.Join(words, " ") + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 1 {
		t.Fatalf("floats painted on page %d, want 1", got)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want the paragraph to continue on page 2", len(pages))
	}
	var got strings.Builder
	for _, l := range textLines(pages) {
		if len(l.text) > 1 && l.text[0] == 'w' && l.text[1] >= '0' && l.text[1] <= '9' {
			got.WriteString(l.text)
		}
	}
	if want := strings.Join(words, ""); got.String() != want {
		t.Errorf("the paragraph lost or repeated words at the page break:\ngot  %s\nwant %s", got.String(), want)
	}
	var widths []bag.ScaledPoint
	for _, in := range pageLineIndents(pages[0]) {
		if in > 0 && !slices.Contains(widths, in) {
			widths = append(widths, in)
		}
	}
	if want := pts(98, 49); !slices.Equal(widths, want) {
		t.Errorf("page 1: the lines beside the floats are indented by %v, want %v", widths, want)
	}
	if n := countIndented(pageLineIndents(pages[1])); n > 0 {
		t.Errorf("page 2: %d lines are narrowed beside floats that are on page 1", n)
	}
}

// Two outside floats next to each other, built for a right page and painted
// on a left one, keep their order from the outer edge: the second one stays
// past the first one's margin box, which is now on the left.
func TestStackedOutsideFloatsFollowThePage(t *testing.T) {
	outside := `<div style="float:outside;width:40pt;height:20pt;margin-left:9pt"></div>`
	vl := buildHTML(t, floatBuilder(t), `<div>`+outside+outside+stackPara(60)+`</div>`)
	if got, want := floatShifts(vl), pts(160, 111); !slices.Equal(got, want) {
		t.Fatalf("built for a right page: floats at %v, want %v", got, want)
	}
	fixLogicalFloats(vl, false)
	if got, want := floatShifts(vl), pts(0, 49); !slices.Equal(got, want) {
		t.Errorf("painted on a left page: floats at %v, want %v", got, want)
	}
}

// A negative bottom margin ends a float's margin box above its box. A float
// opening below that end does not stand beside it but at the edge, over the
// rest of the first float's box, as in a browser; the lines clear the new
// float alone.
func TestAFloatPastAShortenedMarginBoxStartsAtTheEdge(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div>`+
		`<div style="float:left;width:60pt;height:40pt;margin-right:9pt;margin-bottom:-30pt"></div>`+
		stackPara(1)+
		`<div style="float:left;width:30pt;height:20pt;margin-right:9pt"></div>`+
		stackPara(20)+`</div>`)
	assertPrefix(t, "float positions", floatShifts(vl), pts(0, 0))
	assertPrefix(t, "line indents", lineIndents(vl), pts(69, 39, 39))
}

// Each row takes the widest inset among the bands that reach into it,
// whatever order the bands come in: a float can stand beside another and
// still take less from the text, pulled over it by a negative margin.
func TestIndentStepsTakeTheWidestBand(t *testing.T) {
	leading := bag.MustSP("12pt")
	wide := sideBand{inset: bag.MustSP("98pt"), height: bag.MustSP("20pt")}
	narrow := sideBand{inset: bag.MustSP("49pt"), height: bag.MustSP("40pt")}
	want := []node.IndentStep{{Inset: wide.inset, Rows: 2}, {Inset: narrow.inset, Rows: 2}}
	for _, bands := range [][]sideBand{{wide, narrow}, {narrow, wide}} {
		if got := indentSteps(bands, leading); !slices.Equal(got, want) {
			t.Errorf("bands %v: steps %v, want %v", bands, got, want)
		}
	}
}
