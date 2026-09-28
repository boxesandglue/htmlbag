package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag/fonts/camingocoderegular"
)

// Under font-synthesis-style: auto, a family without an italic slants its
// upright in the italic's place; otherwise the upright stands in unchanged.
func TestFontSynthesisStyleSlantsAMissingItalic(t *testing.T) {
	for _, tc := range []struct {
		style string
		slant bool
	}{
		{"", false},
		{"font-synthesis-style: none", false},
		{"font-synthesis-style: auto", true},
		{"font-synthesis: weight style", true},
		{"font-synthesis: none", false},
	} {
		fe, err := frontend.NewForWriter(&bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		ff := fe.NewFontFamily("Upright")
		upright := &frontend.FontSource{Data: camingocoderegular.TTF, Name: "CamingoCode Regular"}
		if err := ff.AddMember(upright, 400, frontend.FontStyleNormal); err != nil {
			t.Fatal(err)
		}
		cb, err := New(fe, NewCSSParserWithDefaults())
		if err != nil {
			t.Fatal(err)
		}
		if err := cb.InitPage(); err != nil {
			t.Fatal(err)
		}
		te, err := cb.HTMLToText(`<p style="font-family: Upright; ` + tc.style + `"><i>slanted</i></p>`)
		if err != nil {
			t.Fatal(err)
		}
		vl, _, err := fe.FormatParagraph(te, bag.MustSP("200pt"))
		if err != nil {
			t.Fatal(err)
		}
		glyphs := 0
		var walk func(n node.Node)
		walk = func(n node.Node) {
			for ; n != nil; n = n.Next() {
				switch v := n.(type) {
				case *node.Glyph:
					glyphs++
					if got := v.Font.Slant != 0; got != tc.slant {
						t.Errorf("%q: slanted %v, want %v", tc.style, got, tc.slant)
						return
					}
				case *node.HList:
					walk(v.List)
				case *node.VList:
					walk(v.List)
				}
			}
		}
		walk(vl)
		if glyphs == 0 {
			t.Fatalf("%q: no glyphs", tc.style)
		}
	}
}
