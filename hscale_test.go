package htmlbag

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// hscaleRuns flattens te into "text@scale" runs, the scale inherited down the
// tree as bag inherits settings, "-" where no element set one.
func hscaleRuns(te *frontend.Text, scale string) []string {
	if v, ok := te.Settings[frontend.SettingHorizontalScale]; ok {
		scale = fmt.Sprint(v)
	}
	var out []string
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				out = append(out, t+"@"+scale)
			}
		case *frontend.Text:
			out = append(out, hscaleRuns(t, scale)...)
		}
	}
	return out
}

func hscaleText(t *testing.T, css, body string) (*frontend.Document, *frontend.Text) {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
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
	if err = cb.AddCSS(css); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body><p>` + body + `</p></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	return fe, te
}

func TestHorizontalScale(t *testing.T) {
	for _, tc := range []struct {
		name, css, body string
		want            []string
		warn            bool
	}{
		{"unset", ``, "a", []string{"a@-"}, false},
		{"percentage", `p{-bag-horizontal-scale:90%}`, "a", []string{"a@0.9"}, false},
		{"number", `p{-bag-horizontal-scale:0.9}`, "a", []string{"a@0.9"}, false},
		{"wider", `p{-bag-horizontal-scale:125%}`, "a", []string{"a@1.25"}, false},
		{"inherited", `p{-bag-horizontal-scale:90%}`, "a<span>b</span>", []string{"a@0.9", "b@0.9"}, false},
		{"no compounding", `p{-bag-horizontal-scale:90%} span{-bag-horizontal-scale:90%}`, "a<span>b<em>c</em></span>",
			[]string{"a@0.9", "b@0.9", "c@0.9"}, false},
		{"100% resets", `p{-bag-horizontal-scale:90%} span{-bag-horizontal-scale:100%}`, "a<span>b</span>c",
			[]string{"a@0.9", "b@1", "c@0.9"}, false},
		{"zero", `p{-bag-horizontal-scale:0}`, "a", []string{"a@-"}, true},
		{"zero percent keeps the inherited scale", `p{-bag-horizontal-scale:90%} span{-bag-horizontal-scale:0%}`, "a<span>b</span>",
			[]string{"a@0.9", "b@0.9"}, true},
		{"negative", `p{-bag-horizontal-scale:-90%}`, "a", []string{"a@-"}, true},
		{"not a number", `p{-bag-horizontal-scale:narrow}`, "a", []string{"a@-"}, true},
		{"inherit, unset and initial", `p{-bag-horizontal-scale:90%} i{-bag-horizontal-scale:inherit} b{-bag-horizontal-scale:unset} em{-bag-horizontal-scale:initial}`,
			"a<i>i</i><b>b</b><em>e</em>", []string{"a@0.9", "i@0.9", "b@0.9", "e@1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf, restore := captureLog()
			defer restore()
			_, te := hscaleText(t, tc.css, tc.body)
			if got := hscaleRuns(te, "-"); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if warned := strings.Contains(buf.String(), "-bag-horizontal-scale"); warned != tc.warn {
				t.Errorf("warned %v, want %v: %s", warned, tc.warn, buf.String())
			}
		})
	}
}

// The scale reaches the glyphs, and letter-spacing stays the absolute length
// it was given.
func TestHorizontalScaleLeavesLetterSpacing(t *testing.T) {
	fe, te := hscaleText(t, `p{font-family:monospace;font-size:10pt;letter-spacing:2pt;-bag-horizontal-scale:50%}`, "abc")
	vl, _, err := fe.FormatParagraph(te, bag.MustSP("200pt"))
	if err != nil {
		t.Fatal(err)
	}
	var scales []float64
	var kerns []bag.ScaledPoint
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.Glyph:
				scales = append(scales, v.HorizontalScale)
			case *node.Kern:
				kerns = append(kerns, v.Kern)
			case *node.HList:
				walk(v.List)
			case *node.VList:
				walk(v.List)
			}
		}
	}
	walk(vl)
	if !slices.Equal(scales, []float64{0.5, 0.5, 0.5}) {
		t.Errorf("glyph scales %v, want 0.5 each", scales)
	}
	if len(kerns) == 0 {
		t.Fatal("no letter-spacing kerns")
	}
	for _, k := range kerns {
		if k != bag.MustSP("2pt") {
			t.Errorf("letter-spacing kern %s, want 2pt", k)
		}
	}
}

func TestHorizontalScalePDF(t *testing.T) {
	html := `<p>` + strings.Repeat("words ", 30) + `</p>`
	plain := renderLineModelPDF(t, ``, html, nil)
	if bytes.Contains(plain, []byte("Tz")) && !bytes.Contains(plain, []byte("100 Tz")) {
		t.Fatal("a document without the property writes a horizontal scale")
	}
	if one := renderLineModelPDF(t, `p{-bag-horizontal-scale:100%}`, html, nil); !bytes.Equal(plain, one) {
		t.Error("a scale of 100% changes the PDF")
	}
	if narrow := renderLineModelPDF(t, `p{-bag-horizontal-scale:90%}`, html, nil); bytes.Equal(plain, narrow) {
		t.Error("a scale of 90% leaves the PDF unchanged")
	}
}
