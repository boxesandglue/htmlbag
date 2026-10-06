package htmlbag

import (
	"path/filepath"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
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
// background or border of its own stays inline: where a float later in a
// paragraph starts depends on the lines before it; lifted out of a link it
// would lose the link; a list item's float belongs beside its marker; and a
// paragraph's own background would be painted over a float before it.
func TestAFloatThatIsNotLiftedStaysInline(t *testing.T) {
	for name, body := range map[string]string{
		"after text":      `<p>before ` + floatPNG(t) + ` after</p>`,
		"in a link":       `<p><a href="https://example.com">` + floatPNG(t) + `</a> text</p>`,
		"in a list item":  `<ul><li>` + floatPNG(t) + ` text</li></ul>`,
		"with background": `<p style="background-color: yellow">` + floatPNG(t) + ` text</p>`,
		"with border":     `<p style="border: 1pt solid black">` + floatPNG(t) + ` text</p>`,
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
