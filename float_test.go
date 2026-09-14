package htmlbag

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

const floatMeasure = "200pt"

func floatBuilder(t *testing.T) *CSSBuilder {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	return cb
}

func buildHTML(t *testing.T, cb *CSSBuilder, body string) *node.VList {
	t.Helper()
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP(floatMeasure))
	if err != nil {
		t.Fatal(err)
	}
	return vl
}

// lineIndents returns, for every line in the tree, how much of the measure its
// content leaves unused at the start — the leftskip, which is where the
// linebreaker's left inset lands.
func lineIndents(v *node.VList) []bag.ScaledPoint {
	var out []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.VList:
				// A float's own content is set in its own box, not in the flow
				// the band shortens.
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					continue
				}
				walk(c.List)
			case *node.HList:
				if origin, _ := c.Attributes["origin"].(string); origin == "line" {
					var indent bag.ScaledPoint
					for m := c.List; m != nil; m = m.Next() {
						if g, ok := m.(*node.Glue); ok {
							if o, _ := g.Attributes["origin"].(string); o == "leftskip" {
								indent += g.Width
							}
						}
					}
					out = append(out, indent)
				}
				walk(c.List)
			}
		}
	}
	walk(v.List)
	return out
}

const floatProse = `Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur excepteur sint occaecat cupidatat non proident.`

// A float:left takes text off the flow and the lines beside it clear it.
func TestFloatLeftIndentsTheLinesBesideIt(t *testing.T) {
	cb := floatBuilder(t)
	floated := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:40pt"></div><p>`+floatProse+`</p></div>`)
	plain := buildHTML(t, floatBuilder(t), `<div><p>`+floatProse+`</p></div>`)

	for i, in := range lineIndents(plain) {
		if in != 0 {
			t.Fatalf("without a float, line %d is already indented by %s", i, in)
		}
	}
	indents := lineIndents(floated)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	if indents[0] == 0 {
		t.Errorf("the first line beside a 60pt float is not indented at all")
	}
	// The band is 40pt of a paragraph taller than that, so the lines past it
	// must run the full measure again.
	var wrapped, full int
	for _, in := range indents {
		if in > 0 {
			wrapped++
		} else {
			full++
		}
	}
	if wrapped == 0 || full == 0 {
		t.Errorf("lines indented=%d full-width=%d, want some of each: the float is shorter than the text beside it", wrapped, full)
	}
}

// The band crosses sibling blocks: the float shortens everything that follows
// until its height is used up, not just the block it appears in.
func TestFloatBandCrossesSiblingBlocks(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:120pt"></div><p>one</p><p>two</p><p>three</p></div>`)
	indents := lineIndents(vl)
	if len(indents) < 3 {
		t.Fatalf("want a line per paragraph, got %d", len(indents))
	}
	for i, in := range indents[:3] {
		if in == 0 {
			t.Errorf("paragraph %d is not clear of a float still %s tall", i, "120pt")
		}
	}
}

// clear ends the band early, whatever is left of the float.
func TestClearEndsTheBand(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:120pt"></div><p>one</p><p style="clear:left">two</p></div>`)
	indents := lineIndents(vl)
	if len(indents) < 2 {
		t.Fatalf("want two lines, got %d", len(indents))
	}
	if indents[0] == 0 {
		t.Errorf("the first paragraph should sit beside the float")
	}
	if indents[1] != 0 {
		t.Errorf("a paragraph that clears the float is indented by %s, want the full measure", indents[1])
	}
}

// A float taller than the content beside it extends its container rather than
// hanging out of the bottom.
func TestContainerExtendsToHoldATallFloat(t *testing.T) {
	cb := floatBuilder(t)
	tall := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:200pt"></div><p>one short line</p></div>`)
	short := buildHTML(t, floatBuilder(t), `<div><p>one short line</p></div>`)
	got := tall.Height + tall.Depth
	float, prose := bag.MustSP("200pt"), short.Height+short.Depth
	if got < float {
		t.Errorf("container is %s tall, want at least the float's %s", got, float)
	}
	// And no taller: a float that merely stacked above the paragraph would make
	// the container the sum of the two, which is the behaviour being replaced.
	if got >= float+prose {
		t.Errorf("container is %s tall, the float (%s) and the paragraph (%s) stacked rather than overlapping", got, float, prose)
	}
}

// lineContentWidths returns the width of each line's content: the packed box
// less the skips around it. Line-end glue alone says nothing — every ragged line
// carries some — so an inset has to be read off the content it leaves room for.
func lineContentWidths(v *node.VList) []bag.ScaledPoint {
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
					var skips bag.ScaledPoint
					for m := c.List; m != nil; m = m.Next() {
						if g, ok := m.(*node.Glue); ok {
							switch o, _ := g.Attributes["origin"].(string); o {
							case "leftskip", "lineend":
								skips += g.Width
							}
						}
					}
					out = append(out, c.Width-skips)
				}
				walk(c.List)
			}
		}
	}
	walk(v.List)
	return out
}

// A float:right narrows the lines from the end without moving where they start.
func TestFloatRightNarrowsFromTheEnd(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:right;width:60pt;height:40pt"></div><p>`+floatProse+`</p></div>`)

	for i, in := range lineIndents(vl) {
		if in != 0 {
			t.Errorf("line %d was shifted by %s: a right float must not move where a line starts", i, in)
		}
	}
	widths := lineContentWidths(vl)
	if len(widths) == 0 {
		t.Fatal("no lines")
	}
	limit := bag.MustSP(floatMeasure) - bag.MustSP("60pt") - floatGutter
	if widths[0] > limit {
		t.Errorf("the first line holds %s of content, more than the %s left beside the float", widths[0], limit)
	}
	// Past the float's height the measure comes back, so some line must use more
	// than the band allowed — otherwise the inset was applied to every row.
	var widest bag.ScaledPoint
	for _, w := range widths {
		if w > widest {
			widest = w
		}
	}
	if widest <= limit {
		t.Errorf("no line exceeds %s: the inset was applied past the float's height", limit)
	}
}

// A floated <img> arrives wrapped in the anonymous inline run its siblings
// share. Without unwrapping it, the commonest float of all — a picture beside
// its caption — is never recognised as one.
func TestFloatedImageIsRecognisedThroughItsInlineRun(t *testing.T) {
	cb := floatBuilder(t)
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	vl := buildHTML(t, cb, `<div><img src="`+png+`" width="60pt" height="40pt" style="float:left"><p>`+floatProse+`</p></div>`)
	indents := lineIndents(vl)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	if indents[0] == 0 {
		t.Errorf("the first line beside a floated image is not indented")
	}
}

// cssLength writes a measured length the way CSS wants it: ScaledPoint prints
// the number alone, which is not a length at all to the parser.
func cssLength(sp bag.ScaledPoint) string {
	return sp.String() + "pt"
}

// buildHTMLErr is buildHTML without the Fatal: the sentinel tests are about
// whether the document builds at all.
func buildHTMLErr(t *testing.T, cb *CSSBuilder, body string) error {
	t.Helper()
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>` + body + `</body></html>`)
	if err != nil {
		return err
	}
	_, err = cb.CreateVlist(te, bag.MustSP(floatMeasure))
	return err
}

// float and clear are stamped on whatever declares them, including elements a
// float is never lifted out of. The sentinels are htmlbag-private SettingTypes
// and FormatParagraph rejects what it does not know, so an inline float used to
// abort the whole document rather than being ignored.
func TestFloatOnInlineContentIsIgnoredRatherThanFatal(t *testing.T) {
	for _, body := range []string{
		`<p>before <span style="float:left">floated</span> after</p>`,
		`<p>before <span style="clear:both">cleared</span> after</p>`,
		`<p style="float:left">a floated paragraph of inline content</p>`,
	} {
		if err := buildHTMLErr(t, floatBuilder(t), body); err != nil {
			t.Errorf("%s: %v", body, err)
		}
	}
}

// A table cell formats its content through its own path, which never passed the
// container branch that strips the sentinels.
func TestFloatInATableCellIsIgnoredRatherThanFatal(t *testing.T) {
	body := `<table><tr><td><p style="float:left">cell</p></td><td>label</td></tr></table>`
	if err := buildHTMLErr(t, floatBuilder(t), body); err != nil {
		t.Errorf("float inside a table cell: %v", err)
	}
}

// A float's own Text is formatted by buildFloat, which is not the path that
// strips -bag-bookmark for a container's children.
func TestBookmarkOnAFloatIsNotFatal(t *testing.T) {
	cb := floatBuilder(t)
	if err := cb.ParseCSSString(`.mark { -bag-bookmark: 2; }`); err != nil {
		t.Fatal(err)
	}
	body := `<div><p class="mark" style="float:left;width:60pt">marked</p><p>` + floatProse + `</p></div>`
	if err := buildHTMLErr(t, cb, body); err != nil {
		t.Errorf("bookmark on a floated element: %v", err)
	}
}

// The same Text is formatted more than once — a table cell measures min/max/
// final, a page-width reflow rebuilds the page. Consuming the settings that
// describe the float left the second pass with an ex-float in flow.
func TestFloatSurvivesASecondFormattingPass(t *testing.T) {
	cb := floatBuilder(t)
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>` +
		`<div><div style="float:left;width:60pt;height:40pt"></div><p>` + floatProse + `</p></div>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cb.CreateVlist(te, bag.MustSP(floatMeasure))
	if err != nil {
		t.Fatal(err)
	}
	second, err := cb.CreateVlist(te, bag.MustSP(floatMeasure))
	if err != nil {
		t.Fatal(err)
	}
	if ha, hb := first.Height+first.Depth, second.Height+second.Depth; ha != hb {
		t.Errorf("the container is %s tall on the first pass and %s on the second: the float was taken out of the flow once only", ha, hb)
	}
	a, b := lineIndents(first), lineIndents(second)
	if len(a) == 0 || a[0] == 0 {
		t.Fatalf("the first pass did not indent beside the float: %v", a)
	}
	if len(b) != len(a) {
		t.Fatalf("second pass produced %d lines, first produced %d", len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("line %d: first pass indented %s, second %s", i, a[i], b[i])
		}
	}
}

// A float opening while a band is live must not simply replace it: the first
// float would then overhang everything after it by whatever was left.
func TestASecondFloatStartsBelowTheFirst(t *testing.T) {
	cb := floatBuilder(t)
	two := buildHTML(t, cb, `<div>`+
		`<div style="float:left;width:60pt;height:40pt"></div>`+
		`<div style="float:left;width:60pt;height:40pt"></div>`+
		`<p>one short line</p></div>`)
	got := two.Height + two.Depth
	want := bag.MustSP("80pt")
	if got < want {
		t.Errorf("container is %s tall, want at least %s: the second float overwrote the first one's band", got, want)
	}
}

// The band is consumed by what a child actually advances the cursor by. A
// child's own depth becomes the container's depth rather than its height, so
// measuring the height alone counts the previous child's depth and misses this
// one's — a drift that shows up as a phantom indent on the child after the band
// should have ended.
func TestTheBandEndsWhereTheContentItCoversEnds(t *testing.T) {
	// Measure one paragraph's advance, then float exactly that much.
	one := buildHTML(t, floatBuilder(t), `<div><p>one</p></div>`)
	// A point inside the paragraph's own advance: the band must end with it. The
	// drift being tested for is a whole line's depth, several points of it.
	advance := one.Height + one.Depth - bag.MustSP("1pt")

	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:`+cssLength(advance)+`"></div><p>one</p><p>two</p></div>`)
	indents := lineIndents(vl)
	if len(indents) < 2 {
		t.Fatalf("want a line per paragraph, got %d", len(indents))
	}
	if indents[0] == 0 {
		t.Errorf("the paragraph the float covers is not indented")
	}
	if indents[1] != 0 {
		t.Errorf("the paragraph after the band is indented by %s: the band outlived the content it covers", indents[1])
	}
}

// A bare container inside a band carries it as a live band rather than stamping
// its full height on every child: only the children the float still covers are
// narrowed, and a `clear` among them ends it.
func TestABareContainerNarrowsOnlyWhatTheFloatCovers(t *testing.T) {
	one := buildHTML(t, floatBuilder(t), `<div><p>one</p></div>`)
	advance := one.Height + one.Depth - bag.MustSP("1pt")

	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:`+cssLength(advance)+`"></div>`+
		`<div><p>one</p><p>two</p></div></div>`)
	indents := lineIndents(vl)
	if len(indents) < 2 {
		t.Fatalf("want a line per paragraph, got %d", len(indents))
	}
	if indents[0] == 0 {
		t.Errorf("the first paragraph of the nested container is not clear of the float")
	}
	if indents[1] != 0 {
		t.Errorf("the second is indented by %s: the band was stamped whole on every child", indents[1])
	}
}

// `clear` inside a bare container was consumed without effect: the container
// stamped the band on its children up front and had nothing left to end.
func TestClearWorksInsideABareContainer(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:120pt"></div>`+
		`<div><p>one</p><p style="clear:left">two</p></div></div>`)
	indents := lineIndents(vl)
	if len(indents) < 2 {
		t.Fatalf("want two lines, got %d", len(indents))
	}
	if indents[0] == 0 {
		t.Errorf("the first paragraph should sit beside the float")
	}
	if indents[1] != 0 {
		t.Errorf("the paragraph that clears the float is indented by %s", indents[1])
	}
}

// boxAfterTheFloat returns the first box laid out beside a float — the sibling
// the float's own box is inserted in front of.
func boxAfterTheFloat(v *node.VList) *node.VList {
	var found *node.VList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			c, ok := e.(*node.VList)
			if !ok {
				continue
			}
			if origin, _ := c.Attributes["origin"].(string); origin == "float" {
				for m := c.Next(); m != nil && found == nil; m = m.Next() {
					if sib, ok := m.(*node.VList); ok {
						found = sib
					}
				}
				continue
			}
			walk(c.List)
		}
	}
	walk(v.List)
	return found
}

// A container with a border or background is narrowed whole rather than having
// only its lines shortened, so it has to move clear of the float as well. The
// shift belongs on the box HTMLBorder returns: applied to the box inside it, the
// frame stays behind under the float while its content moves out.
func TestABorderedContainerInABandMovesItsFrameToo(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:120pt"></div>`+
		`<div style="border:1pt solid black"><p>one</p></div></div>`)

	box := boxAfterTheFloat(vl)
	if box == nil {
		t.Fatal("no box beside the float")
	}
	inset := bag.MustSP("60pt") + floatGutter
	if box.ShiftX != inset {
		t.Errorf("the framed box is shifted by %s, want the float's %s", box.ShiftX, inset)
	}
	if want := bag.MustSP(floatMeasure) - inset; box.Width != want {
		t.Errorf("the framed box is %s wide, want %s", box.Width, want)
	}
}

// The band's inset is written to the same setting as text-indent and the
// initial-letter corner, and it is derived per pass. Leaving it behind means
// the paragraph carries an indent it never declared into whatever formats it
// next.
func TestTheBandDoesNotKeepTheIndentChannel(t *testing.T) {
	cb := floatBuilder(t)
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>` +
		`<div><div style="float:left;width:60pt;height:40pt"></div><p>` + floatProse + `</p></div>` +
		`</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cb.CreateVlist(te, bag.MustSP(floatMeasure)); err != nil {
		t.Fatal(err)
	}

	// ApplySettings writes a zero indent on every element, so the invariant is
	// the band's own value, not the presence of the setting.
	inset := bag.MustSP("60pt") + floatGutter
	var walk func(tx *frontend.Text)
	walk = func(tx *frontend.Text) {
		for _, key := range []frontend.SettingType{frontend.SettingIndentLeft, frontend.SettingIndentRight} {
			if v, _ := tx.Settings[key].(bag.ScaledPoint); v == inset {
				t.Errorf("%v left behind as the float's inset (%s): nothing here declares an indent", key, v)
			}
		}
		for _, key := range []frontend.SettingType{frontend.SettingIndentLeftRows, frontend.SettingIndentRightRows} {
			if rows, _ := tx.Settings[key].(int); rows > 0 {
				t.Errorf("%v left behind as %d: the band's row count outlived the pass that derived it", key, rows)
			}
		}
		for _, itm := range tx.Items {
			if inner, ok := itm.(*frontend.Text); ok {
				walk(inner)
			}
		}
	}
	walk(te)
}

// A percentage width resolves against the containing block. The float is taken
// out of the flow before the pass that usually resolves deferred sizes, so it
// used to be packed at the image's intrinsic size — for a picture wider than the
// measure, a float wider than the page, with every line beside it squeezed to
// nothing.
func TestAPercentWidthFloatResolvesAgainstItsContainer(t *testing.T) {
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	vl := buildHTML(t, floatBuilder(t), `<div><img src="`+png+`" style="float:left;width:50%"><p>`+floatProse+`</p></div>`)
	indents := lineIndents(vl)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	measure := bag.MustSP(floatMeasure)
	if want := measure/2 + floatGutter; indents[0] != want {
		t.Errorf("the first line is indented %s, want %s — half the measure plus the gutter", indents[0], want)
	}
	// And the text beside it is still text: an intrinsically-sized float left
	// lines with no room at all.
	for _, w := range lineContentWidths(vl)[:1] {
		if w <= 0 {
			t.Errorf("the first line holds %s of content: nothing fits beside the float", w)
		}
	}
}

// A float holds text clear of its margins, not just of its box. The gutter is
// what a float with no margin of its own gets instead.
func TestAFloatsSideMarginHoldsTheTextClear(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:40pt;margin-right:20pt"></div><p>`+floatProse+`</p></div>`)
	indents := lineIndents(vl)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	if want := bag.MustSP("80pt"); indents[0] != want {
		t.Errorf("the first line is indented %s, want %s — the float plus its margin", indents[0], want)
	}
}

// The same on the other side, where the margin faces the text from the right.
func TestARightFloatsSideMarginHoldsTheTextClear(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:right;width:60pt;height:40pt;margin-left:20pt"></div><p>`+floatProse+`</p></div>`)
	widths := lineContentWidths(vl)
	if len(widths) == 0 {
		t.Fatal("no lines")
	}
	if limit := bag.MustSP(floatMeasure) - bag.MustSP("80pt"); widths[0] > limit {
		t.Errorf("the first line holds %s of content, more than the %s left beside the float and its margin", widths[0], limit)
	}
}

// A margin below the float is part of what the text has to clear, so the band
// is that much taller.
func TestAFloatsBottomMarginExtendsTheBand(t *testing.T) {
	narrowed := func(body string) int {
		n := 0
		for _, in := range lineIndents(buildHTML(t, floatBuilder(t), body)) {
			if in > 0 {
				n++
			}
		}
		return n
	}
	plain := narrowed(`<div><div style="float:left;width:60pt;height:40pt"></div><p>` + floatProse + `</p></div>`)
	withMargin := narrowed(`<div><div style="float:left;width:60pt;height:40pt;margin-bottom:30pt"></div><p>` + floatProse + `</p></div>`)
	if withMargin <= plain {
		t.Errorf("%d lines are narrowed with a 30pt bottom margin and %d without: the margin is not part of the band", withMargin, plain)
	}
}

// A margin above the float pushes it down, and the container that holds it has
// to grow by as much.
func TestAFloatsTopMarginPushesItDown(t *testing.T) {
	plain := buildHTML(t, floatBuilder(t), `<div><div style="float:left;width:60pt;height:200pt"></div><p>one short line</p></div>`)
	pushed := buildHTML(t, floatBuilder(t), `<div><div style="float:left;width:60pt;height:200pt;margin-top:20pt"></div><p>one short line</p></div>`)
	got, want := pushed.Height+pushed.Depth, (plain.Height+plain.Depth)+bag.MustSP("20pt")
	if got != want {
		t.Errorf("the container is %s tall, want %s — the float's own height plus the margin above it", got, want)
	}
}

// floatBox returns the float's own box: the one openBand takes out of the flow.
func floatBox(v *node.VList) *node.VList {
	var found *node.VList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil && found == nil; e = e.Next() {
			c, ok := e.(*node.VList)
			if !ok {
				continue
			}
			if origin, _ := c.Attributes["origin"].(string); origin == "float" {
				found = c
				return
			}
			walk(c.List)
		}
	}
	walk(v.List)
	return found
}

// The margin on the far side — the one facing the container edge rather than
// the text — is space the float holds too: it moves the float inwards and the
// content gives up that much more. The shift is the half an indent cannot show,
// since a float sitting at the edge indents the text by exactly as much as one
// held 20pt off it.
func TestAFarSideMarginMovesTheFloatAndTheText(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:left;width:60pt;height:40pt;margin-left:20pt"></div><p>`+floatProse+`</p></div>`)

	indents := lineIndents(vl)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	// The float, the default gutter (nothing was declared on the text side) and
	// the margin holding it off the edge.
	if want := bag.MustSP("60pt") + floatGutter + bag.MustSP("20pt"); indents[0] != want {
		t.Errorf("the first line is indented %s, want %s", indents[0], want)
	}
	box := floatBox(vl)
	if box == nil {
		t.Fatal("no float box")
	}
	if want := bag.MustSP("20pt"); box.ShiftX != want {
		t.Errorf("the float sits at %s, want %s from the container edge", box.ShiftX, want)
	}
}

// The mirror: a right float's far side is the right edge.
func TestARightFloatsFarSideMarginMovesItInFromTheEdge(t *testing.T) {
	cb := floatBuilder(t)
	vl := buildHTML(t, cb, `<div><div style="float:right;width:60pt;height:40pt;margin-right:20pt"></div><p>`+floatProse+`</p></div>`)

	widths := lineContentWidths(vl)
	if len(widths) == 0 {
		t.Fatal("no lines")
	}
	measure, width, margin := bag.MustSP(floatMeasure), bag.MustSP("60pt"), bag.MustSP("20pt")
	if limit := measure - width - floatGutter - margin; widths[0] > limit {
		t.Errorf("the first line holds %s of content, more than the %s beside the float", widths[0], limit)
	}
	box := floatBox(vl)
	if box == nil {
		t.Fatal("no float box")
	}
	if want := measure - width - margin; box.ShiftX != want {
		t.Errorf("the float sits at %s, want %s — in from the right edge by its margin", box.ShiftX, want)
	}
}

// The three below are pgundlach's, from the review of #12, kept as he wrote
// them: the failures are his, and so is the point that a fix has to make all
// three green rather than the first one.

// A float's margins reach the band through whatever object openBand is handed
// them from. For a <div> that is the float itself. For a replaced element it is
// the anonymous inline run around it, whose margin settings are stamped zeros,
// so the picture holds no more space than its own box.
func TestAFloatedImagesSideMarginHoldsTheTextClear(t *testing.T) {
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	vl := buildHTML(t, floatBuilder(t), `<div><img src="`+png+
		`" style="float:left;width:60pt;margin-right:30pt"><p>`+floatProse+`</p></div>`)
	indents := lineIndents(vl)
	if len(indents) == 0 {
		t.Fatal("no lines")
	}
	if want := bag.MustSP("60pt") + bag.MustSP("30pt"); indents[0] != want {
		t.Errorf("the first line is indented %s, want %s: the image plus the margin it asked for", indents[0], want)
	}
}

// The control, and the whole of the difference: the same declaration on a <div>
// float does hold the text clear. The two paths differ only in which object the
// margins are read from, so a fix that closes the gap leaves this one alone.
func TestADivAndAnImageFloatAgreeOnTheirMargins(t *testing.T) {
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	style := `float:left;width:60pt;height:40pt;margin-right:30pt`
	div := lineIndents(buildHTML(t, floatBuilder(t),
		`<div><div style="`+style+`"></div><p>`+floatProse+`</p></div>`))
	img := lineIndents(buildHTML(t, floatBuilder(t),
		`<div><img src="`+png+`" style="`+style+`"><p>`+floatProse+`</p></div>`))
	if len(div) == 0 || len(img) == 0 {
		t.Fatal("no lines")
	}
	if div[0] != img[0] {
		t.Errorf("the same float declaration indents the first line by %s as a div and %s as an image", div[0], img[0])
	}
}

// The float's own VList carries what later passes scan it for: "inserts" for a
// footnote raised out of the float, the _splittable family for a float that has
// to break across a page. Those scans read one node's attributes and do not
// recurse, so whatever openBand wraps the box in has to carry them on.
func TestAFloatsTopMarginKeepsTheBoxsAttributes(t *testing.T) {
	body := func(style string) string {
		return `<div><div style="float:left;width:60pt;height:40pt;` + style +
			`">pic<fn>raised out of the float</fn></div><p>` + floatProse + `</p></div>`
	}
	plain := floatBox(buildHTML(t, floatBuilder(t), body("")))
	pushed := floatBox(buildHTML(t, floatBuilder(t), body("margin-top:20pt")))
	if plain == nil || pushed == nil {
		t.Fatal("no float box")
	}
	// Without this the rest proves nothing: the footnote has to be on the float
	// in the first place for its loss to mean anything.
	if got := len(insertsOnNode(plain)); got != 1 {
		t.Fatalf("the float carries %d inserts with no top margin, want 1", got)
	}
	if got := len(insertsOnNode(pushed)); got != 1 {
		t.Errorf("the float carries %d inserts once it declares a top margin, want 1: the footnote is lost", got)
	}
	for key := range plain.Attributes {
		if _, ok := pushed.Attributes[key]; !ok {
			t.Errorf("%q is on the float box but not on the wrapper its top margin puts around it", key)
		}
	}
}
