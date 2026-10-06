package htmlbag

import (
	"bytes"
	"testing"
)

// A float is painted over the background and border of the block beside it,
// as CSS paints floats after the in-flow blocks (CSS 2.1 Appendix E), though
// it comes first in its container's list (#81).
func TestAFloatIsPaintedOverTheBlockBesideIt(t *testing.T) {
	floatBorder := []byte("1 0 0 rg")      // the float's red border
	background := []byte("0.93 0.93 1 rg") // the paragraph's background
	paragraphBorder := []byte("0 0 1 rg")  // the paragraph's blue border
	for name, html := range map[string]string{
		"a sibling float": `<div style="float: right; width: 60pt; height: 40pt; border: 1pt solid red">Float</div>` +
			`<p style="margin: 0; background-color: #eef; border: 1pt solid blue">` + floatProse + `</p>`,
		// A paragraph with a border keeps its float inline, so this one has a
		// background only. The float's own background keeps it from carrying
		// the paragraph's, which would be found first.
		"a lifted float": `<p style="margin: 0; background-color: #eef">` +
			`<span style="float: right; width: 60pt; border: 1pt solid red; background-color: white">Float</span>` + floatProse + `</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			pdf := renderLineModelPDF(t, "", html, func(cb *CSSBuilder) { cb.frontend.Doc.CompressLevel = 0 })
			f := bytes.Index(pdf, floatBorder)
			b := bytes.Index(pdf, background)
			if f < 0 || b < 0 {
				t.Fatalf("missing paint: float border at %d, background at %d", f, b)
			}
			if f < b {
				t.Errorf("the float is painted at %d, before the background of the paragraph beside it at %d", f, b)
			}
			if pb := bytes.Index(pdf, paragraphBorder); pb > f {
				t.Errorf("the float is painted at %d, before the border of the paragraph beside it at %d", f, pb)
			}
		})
	}
}
