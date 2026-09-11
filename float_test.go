package htmlbag

import (
	"bytes"
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
