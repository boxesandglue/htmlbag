package htmlbag

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
)

// An inline background covers the font's ascent and descent, the content
// area browsers paint, not the em box: the rule spans the glyphs' font's
// ContentAscent above the baseline and ContentDescent below it.
func TestInlineBackgroundIsTheContentArea(t *testing.T) {
	for _, family := range []string{"serif", "sans"} {
		t.Run(family, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			te, err := cb.HTMLToText(`<html><body><p style="font-family: ` + family + `">Some <span style="background-color: yellow">highlighted</span> text</p></body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			vl, err := cb.CreateVlist(te, sp("200pt"))
			if err != nil {
				t.Fatal(err)
			}
			var rule *node.Rule
			var glyph *node.Glyph
			var walk func(n node.Node)
			walk = func(n node.Node) {
				for ; n != nil; n = n.Next() {
					switch v := n.(type) {
					case *node.VList:
						walk(v.List)
					case *node.HList:
						walk(v.List)
					case *node.Rule:
						if o, _ := v.Attributes["origin"].(string); o == "inline background" && rule == nil {
							rule = v
						}
					case *node.Glyph:
						if glyph == nil && v.Font != nil {
							glyph = v
						}
					}
				}
			}
			walk(vl)
			if rule == nil || glyph == nil {
				t.Fatal("no inline background or no glyph")
			}
			// "x y w h re f": y is -depth, h the height plus the depth.
			f := strings.Fields(rule.Pre)
			var y, h float64
			for i, w := range f {
				if w == "re" && i >= 4 {
					y, _ = strconv.ParseFloat(f[i-3], 64)
					h, _ = strconv.ParseFloat(f[i-1], 64)
				}
			}
			fnt := glyph.Font
			wantY, wantH := -fnt.ContentDescent.ToPT(), (fnt.ContentAscent + fnt.ContentDescent).ToPT()
			if math.Abs(y-wantY) > 0.01 || math.Abs(h-wantH) > 0.01 {
				t.Errorf("background y %.2f, height %.2f; want %.2f, %.2f (the content area), the em box being %s", y, h, wantY, wantH, fnt.Size)
			}
		})
	}
}
