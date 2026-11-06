package htmlbag

import (
	"bytes"
	"slices"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestParseFontVariantNumeric(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  []string
		valid bool
	}{
		{"normal", nil, true},
		{"initial", nil, true},
		{"oldstyle-nums", []string{"onum=1"}, true},
		{"Tabular-Nums LINING-NUMS", []string{"tnum=1", "lnum=1"}, true},
		{"lining-nums tabular-nums diagonal-fractions ordinal slashed-zero", []string{"lnum=1", "tnum=1", "frac=1", "ordn=1", "zero=1"}, true},
		{"proportional-nums stacked-fractions", []string{"pnum=1", "afrc=1"}, true},
		{"lining-nums oldstyle-nums", nil, false},
		{"proportional-nums tabular-nums", nil, false},
		{"diagonal-fractions stacked-fractions", nil, false},
		{"ordinal ordinal", nil, false},
		{"normal ordinal", nil, false},
		{"inherit", nil, false},
		{"bold", nil, false},
		{"", nil, false},
	} {
		got, valid := parseFontVariantNumeric(tc.value)
		if !slices.Equal(got, tc.want) || valid != tc.valid {
			t.Errorf("parseFontVariantNumeric(%q) = %v, %v, want %v, %v", tc.value, got, valid, tc.want, tc.valid)
		}
	}
}

// featuresOf returns the features the text "1" is shaped with, inherited as
// bag inherits them.
func featuresOf(t *testing.T, te *frontend.Text) []string {
	t.Helper()
	var found []string
	var ok bool
	var walk func(te *frontend.Text, inherited []string)
	walk = func(te *frontend.Text, inherited []string) {
		if v, has := te.Settings[frontend.SettingOpenTypeFeature].([]string); has {
			inherited = v
		}
		for _, itm := range te.Items {
			switch v := itm.(type) {
			case string:
				if v == "1" {
					found, ok = inherited, true
				}
			case *frontend.Text:
				walk(v, inherited)
			}
		}
	}
	walk(te, nil)
	if !ok {
		t.Fatal(`no text "1"`)
	}
	return found
}

func TestFontVariantNumericCascade(t *testing.T) {
	for _, tc := range []struct {
		name, css string
		want      []string
	}{
		{"on the element", `#x{font-variant-numeric:oldstyle-nums}`, []string{"onum=1"}},
		{"inherited", `div{font-variant-numeric:oldstyle-nums}`, []string{"onum=1"}},
		{"replaces, does not add", `div{font-variant-numeric:oldstyle-nums} #x{font-variant-numeric:tabular-nums}`, []string{"tnum=1"}},
		{"normal", `div{font-variant-numeric:oldstyle-nums} #x{font-variant-numeric:normal}`, nil},
		{"initial", `div{font-variant-numeric:oldstyle-nums} #x{font-variant-numeric:initial}`, nil},
		{"inherit", `div{font-variant-numeric:oldstyle-nums} #x{font-variant-numeric:inherit}`, []string{"onum=1"}},
		{"invalid keeps the parent's", `div{font-variant-numeric:oldstyle-nums} #x{font-variant-numeric:lining-nums oldstyle-nums}`, []string{"onum=1"}},
		{"font-feature-settings last", `#x{font-feature-settings:"onum" 0; font-variant-numeric:oldstyle-nums}`, []string{"onum=1", "onum=0"}},
		{"font-feature-settings: normal stays", `div{font-variant-numeric:oldstyle-nums} #x{font-feature-settings:normal}`, []string{"onum=1"}},
		{"font-variant", `#x{font-variant:oldstyle-nums tabular-nums}`, []string{"onum=1", "tnum=1"}},
		{"font-variant resets", `div{font-variant-numeric:oldstyle-nums} #x{font-variant:normal}`, nil},
		{"font-variant, then the longhand", `#x{font-variant:oldstyle-nums; font-variant-numeric:lining-nums}`, []string{"lnum=1"}},
		{"the longhand, then font-variant", `#x{font-variant-numeric:lining-nums; font-variant:oldstyle-nums}`, []string{"onum=1"}},
		{"font resets", `#x{font-variant-numeric:oldstyle-nums; font:10pt serif}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			fe, err := frontend.NewForWriter(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if err = LoadIncludedFonts(fe); err != nil {
				t.Fatal(err)
			}
			cb, err := New(fe, NewCSSParserWithDefaults())
			if err != nil {
				t.Fatal(err)
			}
			if err = cb.AddCSS(tc.css); err != nil {
				t.Fatal(err)
			}
			te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><div><p id="x">1</p></div></body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			if got := featuresOf(t, te); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Small capitals keep the numeric features of their text.
func TestFontVariantNumericWithSmallCaps(t *testing.T) {
	sty := &FormattingStyles{numericFeatures: []string{"onum=1"}, fontfeatures: []string{"kern=0"}}
	if got, want := append(sty.openTypeFeatures(), "smcp=1"), []string{"onum=1", "kern=0", "smcp=1"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := sty.fontfeatures, []string{"kern=0"}; !slices.Equal(got, want) {
		t.Errorf("the list of the style changed: %q", got)
	}
}

// glyphsOf is the glyph ids and the width of a paragraph in the shaper's hands.
func glyphsOf(t *testing.T, css, text string) ([]int, bag.ScaledPoint) {
	t.Helper()
	lines := lineModelLines(t, `p{font-family:serif;font-size:10pt}`+css, `<p>`+text+`</p>`, nil)
	var glyphs []int
	var width bag.ScaledPoint
	for _, line := range lines {
		for e := line.List; e != nil; e = e.Next() {
			if g, ok := e.(*node.Glyph); ok {
				glyphs = append(glyphs, g.Codepoint)
				width += g.Width
			}
		}
	}
	return glyphs, width
}

// The keywords reach the shaper: Crimson Pro sets lining figures by default.
func TestFontVariantNumericShapes(t *testing.T) {
	plain, _ := glyphsOf(t, ``, "1890")
	oldstyle, _ := glyphsOf(t, `p{font-variant-numeric:oldstyle-nums}`, "1890")
	if slices.Equal(oldstyle, plain) {
		t.Errorf("oldstyle-nums changed nothing: %v", plain)
	}
	overruled, _ := glyphsOf(t, `p{font-variant-numeric:oldstyle-nums; font-feature-settings:"onum" 0}`, "1890")
	if !slices.Equal(overruled, plain) {
		t.Errorf("font-feature-settings did not win: %v, want %v", overruled, plain)
	}
	_, one := glyphsOf(t, `p{font-variant-numeric:proportional-nums}`, "1")
	_, zero := glyphsOf(t, `p{font-variant-numeric:proportional-nums}`, "0")
	_, tabularOne := glyphsOf(t, `p{font-variant-numeric:tabular-nums}`, "1")
	_, tabularZero := glyphsOf(t, `p{font-variant-numeric:tabular-nums}`, "0")
	if one == zero {
		t.Errorf("proportional 1 and 0 have the same width %v", one)
	}
	if tabularOne != tabularZero {
		t.Errorf("tabular 1 and 0: %v and %v", tabularOne, tabularZero)
	}
}
