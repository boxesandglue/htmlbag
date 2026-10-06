package htmlbag

import (
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// floatPNG is a floated image for the paragraphs below.
func floatPNG(t *testing.T) string {
	t.Helper()
	png, err := filepath.Abs("testdata/float.png")
	if err != nil {
		t.Fatal(err)
	}
	return `<img src="` + png + `" width="60pt" height="40pt" style="float:left">`
}

// A float before the first text of a paragraph floats: it is lifted to stand
// before the paragraph in its container, where a float at the top of the
// paragraph's first line is (CSS 2.1 §9.5.1), and the lines beside it clear it.
func TestAFloatAtAParagraphsStartFloats(t *testing.T) {
	for name, p := range map[string]string{
		"image":         `<p>` + floatPNG(t) + ` ` + floatProse + `</p>`,
		"span":          `<p> <span style="float:left;width:60pt;height:40pt">F</span>` + floatProse + `</p>`,
		"in a bare div": `<div><p>` + floatPNG(t) + floatProse + `</p></div>`,
		// A float is painted after the paragraph's background
		// (document.PaintLast), so it no longer covers it.
		"with background": `<p style="background-color: yellow">` + floatPNG(t) + ` ` + floatProse + `</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			vl := buildHTML(t, floatBuilder(t), `<div>`+p+`</div>`)
			if floatBox(vl) == nil {
				t.Fatal("the float stayed inline")
			}
			indents := lineIndents(vl)
			if len(indents) == 0 || indents[0] == 0 {
				t.Errorf("the first line beside the float is not indented: %v", indents)
			}
		})
	}
}

// A float after text, in a link, in a list item, or in a paragraph with a
// border or padding stays inline: where a float later in a paragraph starts
// depends on the lines before it; lifted out of a link it would lose the link;
// a list item's float belongs beside its marker; and lifted, a float would
// stand on the paragraph's border rather than inside its padding.
func TestAFloatThatIsNotLiftedStaysInline(t *testing.T) {
	for name, body := range map[string]string{
		"after text":     `<p>before ` + floatPNG(t) + ` after</p>`,
		"in a link":      `<p><a href="https://example.com">` + floatPNG(t) + `</a> text</p>`,
		"in a list item": `<ul><li>` + floatPNG(t) + ` text</li></ul>`,
		"with border":    `<p style="border: 1pt solid black">` + floatPNG(t) + ` text</p>`,
		"with padding":   `<p style="padding-left: 6pt">` + floatPNG(t) + ` text</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			if box := floatBox(buildHTML(t, floatBuilder(t), `<div>`+body+`</div>`)); box != nil {
				t.Errorf("the float was lifted out of the paragraph")
			}
		})
	}
}

// The lifting rewrites the container's items, so a second formatting pass of
// the same Text finds the float where the first one put it, once.
func TestALiftedFloatSurvivesASecondFormattingPass(t *testing.T) {
	cb := floatBuilder(t)
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><div><p>` + floatPNG(t) + floatProse + `</p></div></body></html>`)
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

// Two floats at the start of a paragraph are both lifted, in their order.
func TestTwoFloatsAtAParagraphsStartAreBothLifted(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div><p>`+floatPNG(t)+`<span style="float:right;width:30pt">G</span>`+floatProse+`</p></div>`)
	var sides []string
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			if v, ok := n.(*node.VList); ok {
				if origin, _ := v.Attributes["origin"].(string); origin == "float" {
					side := "left"
					if v.ShiftX > 0 {
						side = "right"
					}
					sides = append(sides, side)
					continue
				}
				walk(v.List)
			}
		}
	}
	walk(vl.List)
	if len(sides) != 2 || sides[0] != "left" || sides[1] != "right" {
		t.Errorf("floats %v, want left then right", sides)
	}
}

// A float lifted from a paragraph whose top margin is larger than the bottom
// margin before it stands at the paragraph's first line, below that margin:
// the margin is laid out before the float, and the paragraph's collapses with
// it.
func TestALiftedFloatStandsBelowTheParagraphsMargin(t *testing.T) {
	vl := buildHTML(t, floatBuilder(t), `<div><p style="margin: 0">Before.</p><p style="margin-top: 20pt">`+floatPNG(t)+` `+floatProse+`</p></div>`)
	box := floatBox(vl)
	if box == nil {
		t.Fatal("the float stayed inline")
	}
	space := func(n node.Node, next func(node.Node) node.Node) bag.ScaledPoint {
		var sum bag.ScaledPoint
		for n = next(n); n != nil; n = next(n) {
			switch v := n.(type) {
			case *node.Kern:
				sum += v.Kern
			case *node.Glue:
				sum += v.Width
			default:
				return sum
			}
		}
		return sum
	}
	if above := space(box, node.Node.Prev); above != bag.MustSP("20pt") {
		t.Errorf("the space above the float is %s, want the paragraph's 20pt margin", above)
	}
	if below := space(box, node.Node.Next); below != 0 {
		t.Errorf("%s between the float and the paragraph, want none", below)
	}
}

// floatBoxes counts the float boxes in v.
func floatBoxes(v *node.VList) int {
	n := 0
	var walk func(e node.Node)
	walk = func(e node.Node) {
		for ; e != nil; e = e.Next() {
			if c, ok := e.(*node.VList); ok {
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					n++
				}
				walk(c.List)
			}
		}
	}
	walk(v.List)
	return n
}

// A floated inline element whose content makes more than one run, by a <br>
// or a child element, is one float, lifted from a paragraph or as the only
// content of a block (#83). Two floated spans stay two floats.
func TestAFloatedSpanWithSeveralRunsIsOneFloat(t *testing.T) {
	span := `<span style="float: left; width: 50pt; border: 1pt solid red">A<br><em>B</em> b<br>C</span>`
	for name, c := range map[string]struct {
		body   string
		floats int
	}{
		"in a paragraph":    {`<div><p>` + span + floatProse + `</p></div>`, 1},
		"in a block":        {`<div><div>` + span + `</div><p>` + floatProse + `</p></div>`, 1},
		"two floated spans": {`<div><p><span style="float: left">One</span><span style="float: left">Two</span>` + floatProse + `</p></div>`, 2},
	} {
		t.Run(name, func(t *testing.T) {
			if got := floatBoxes(buildHTML(t, floatBuilder(t), c.body)); got != c.floats {
				t.Errorf("%d float boxes, want %d", got, c.floats)
			}
		})
	}
}

// The runs of such a float are its inline content: the float and the box
// (background, border, padding, width) are on the element's Text alone, so
// a run neither floats again nor paints the padding a second time.
func TestAFloatedSpansRunsCarryNoBox(t *testing.T) {
	cb := floatBuilder(t)
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><div><span style="float: left; width: 50pt; padding: 4pt; background-color: yellow">A<br>B</span></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	var group *frontend.Text
	var find func(items []any)
	find = func(items []any) {
		for _, itm := range items {
			if t, ok := itm.(*frontend.Text); ok && group == nil {
				if _, isFloat := t.Settings[settingFloat]; isFloat {
					group = t
					return
				}
				find(t.Items)
			}
		}
	}
	find(te.Items)
	if group == nil {
		t.Fatal("no floated Text")
	}
	if _, ok := group.Settings[frontend.SettingPaddingLeft]; !ok {
		t.Error("the float has lost its padding")
	}
	runs := 0
	for _, itm := range group.Items {
		run, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		runs++
		for _, k := range []frontend.SettingType{settingFloat, frontend.SettingPaddingLeft, frontend.SettingBackgroundColor, frontend.SettingWidth} {
			if _, has := run.Settings[k]; has {
				t.Errorf("run %d carries setting %v of the float's box", runs, k)
			}
		}
	}
	if runs != 3 {
		t.Errorf("the float holds %d runs, want 3 (A, the break, B)", runs)
	}
}
