package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// floatWidth builds body and returns the width of its float box.
func floatWidth(t *testing.T, body string) bag.ScaledPoint {
	t.Helper()
	box := floatBox(buildHTML(t, floatBuilder(t), body))
	if box == nil {
		t.Fatalf("no float in %s", body)
	}
	return box.Width
}

// A float without a width shrinks to its content (CSS 2.1 §10.3.5): as wide
// as its longest line, so the text beside it gets the rest of the measure.
func TestAFloatWithoutAWidthShrinksToItsContent(t *testing.T) {
	short := floatWidth(t, `<div><div style="float:left">Short</div><p>`+floatProse+`</p></div>`)
	if short <= 0 || short >= bag.MustSP(floatMeasure)/4 {
		t.Fatalf("a float holding one short word is %s wide in a %s measure", short, floatMeasure)
	}
	// The longest line decides, whichever it is.
	longest := floatWidth(t, `<div><div style="float:left">longer line here</div><p>x</p></div>`)
	threeLines := floatWidth(t, `<div><div style="float:left">A<br>longer line here<br>B</div><p>x</p></div>`)
	if threeLines != longest {
		t.Errorf("a float of three lines is %s wide, want its longest line's %s", threeLines, longest)
	}
	// Padding and border are around the content; a centered line is measured
	// without the glue that centers it.
	plain := floatWidth(t, `<div><div style="float:left">Short</div><p>x</p></div>`)
	boxed := floatWidth(t, `<div><div style="float:left; padding: 0 3pt; border: 1pt solid black">Short</div><p>x</p></div>`)
	if want := plain + bag.MustSP("8pt"); boxed != want {
		t.Errorf("with 3pt padding and a 1pt border on each side the float is %s wide, want %s", boxed, want)
	}
	if centered := floatWidth(t, `<div><div style="float:left; text-align: center">Short</div><p>x</p></div>`); centered != plain {
		t.Errorf("a centered float is %s wide, want %s as left aligned", centered, plain)
	}
	// The lines beside it clear it.
	indents := lineIndents(buildHTML(t, floatBuilder(t), `<div><div style="float:left">Short</div><p>`+floatProse+`</p></div>`))
	if len(indents) == 0 || indents[0] < short || indents[0] > bag.MustSP(floatMeasure)/2 {
		t.Errorf("the first line beside a %s float is indented by %v", short, indents)
	}
}

// A float whose content needs the whole measure, or holds something the
// measuring pass cannot read off a line (a table in a bordered box: a bare
// table shrinks the float by itself), is set at the measure, as before; and
// the text after it, with no room beside it, goes below it rather than into
// a column of no width.
func TestTextGoesBelowAFloatThatLeavesNoRoom(t *testing.T) {
	for name, float := range map[string]string{
		"long text": `<div style="float:left">` + floatProse + `</div>`,
		"table":     `<div style="float:left; border: 1pt solid black"><table><tr><td>a</td><td>b</td></tr></table></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			vl := buildHTML(t, floatBuilder(t), `<div>`+float+`<p>after</p></div>`)
			if box := floatBox(vl); box == nil || box.Width < bag.MustSP(floatMeasure)-bag.MustSP("1pt") {
				t.Fatalf("float %v, want one as wide as the %s measure", box, floatMeasure)
			}
			indents := lineIndents(vl)
			if len(indents) != 1 || indents[0] != 0 {
				t.Errorf("the paragraph after the float has indents %v, want one line at the full measure", indents)
			}
		})
	}
}

// The measuring pass registers nothing: an id in a shrinking float is one
// anchor, a heading one outline entry, and a tagged float one structure
// element per block.
func TestMeasuringAFloatRegistersNothing(t *testing.T) {
	cb := floatBuilder(t)
	buildHTML(t, cb, `<div><div style="float:left"><h2 id="h">Head</h2><p id="p">Short</p></div><p>x</p></div>`)
	anchors := map[string]int{}
	for _, a := range cb.Anchors {
		anchors[a.ID]++
	}
	if anchors["h"] != 1 || anchors["p"] != 1 {
		t.Errorf("anchors %v, want h and p once each", anchors)
	}
	if len(cb.Headings) != 1 {
		t.Errorf("%d headings, want 1", len(cb.Headings))
	}

	root := renderForStructTree(t, `<!DOCTYPE html><html><body><div style="float:left"><p>Short</p></div><p>beside</p></body></html>`)
	if root == nil {
		t.Fatal("no structure tree")
	}
	var ps int
	walkStruct(root, func(se *document.StructureElement) {
		if se.Role == "P" {
			ps++
		}
	})
	if ps != 2 {
		t.Errorf("%d P structure elements, want 2", ps)
	}
}
