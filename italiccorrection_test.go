package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag/fonts/crimsonproitalic"
	"github.com/boxesandglue/htmlbag/fonts/crimsonproregular"
)

// TestItalicCorrectionKern — with SettingItalicCorrection a kern appears
// between a trailing italic glyph and a directly following upright glyph;
// without the setting the two glyphs stay adjacent.
func TestItalicCorrectionKern(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("NewForWriter: %v", err)
	}
	fam := fe.NewFontFamily("crimson")
	if err := fam.AddMember(&frontend.FontSource{Data: crimsonproregular.TTF, Name: "regular"}, 400, frontend.FontStyleNormal); err != nil {
		t.Fatal(err)
	}
	if err := fam.AddMember(&frontend.FontSource{Data: crimsonproitalic.TTF, Name: "italic"}, 400, frontend.FontStyleItalic); err != nil {
		t.Fatal(err)
	}

	build := func(correct bool) *node.VList {
		te := frontend.NewText()
		te.Settings[frontend.SettingFontFamily] = fam
		te.Settings[frontend.SettingSize] = bag.MustSP("12pt")
		if correct {
			te.Settings[frontend.SettingItalicCorrection] = true
		}
		it := frontend.NewText()
		it.Settings[frontend.SettingStyle] = frontend.FontStyleItalic
		it.Items = append(it.Items, "wolf")
		te.Items = append(te.Items, it, ")")
		vl, _, err := fe.FormatParagraph(te, bag.MustSP("200pt"))
		if err != nil {
			t.Fatalf("FormatParagraph: %v", err)
		}
		return vl
	}

	countBoundaryKerns := func(vl *node.VList) int {
		count := 0
		var walk func(n node.Node)
		walk = func(n node.Node) {
			for ; n != nil; n = n.Next() {
				switch v := n.(type) {
				case *node.HList:
					walk(v.List)
				case *node.Kern:
					if g1, ok := n.Prev().(*node.Glyph); ok {
						if g2, ok := n.Next().(*node.Glyph); ok && g1.Font != g2.Font && v.Kern > 0 {
							count++
						}
					}
				}
			}
		}
		walk(vl.List)
		return count
	}

	if got := countBoundaryKerns(build(false)); got != 0 {
		t.Errorf("without correction: %d boundary kerns, want 0", got)
	}
	if got := countBoundaryKerns(build(true)); got != 1 {
		t.Errorf("with correction: %d boundary kerns, want 1", got)
	}
}
