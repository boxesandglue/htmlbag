package htmlbag

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func newColorTestDoc(t *testing.T, css string) (*CSS, *frontend.Document) {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	c := NewCSSParser()
	if err := c.AddCSSText(css); err != nil {
		t.Fatalf("AddCSSText: %v", err)
	}
	if err := AddColorsFromCSS(c, fe); err != nil {
		t.Fatalf("AddColorsFromCSS: %v", err)
	}
	return c, fe
}

// @-bag-color mirrors DefineColor: every model ends up as the matching PDF
// color operator, and a plain value goes through the normal CSS color parser.
func TestBagColorModels(t *testing.T) {
	_, fe := newColorTestDoc(t, `
		@-bag-color muted { value: #6A6A6A; }
		@-bag-color brand { model: cmyk; c: 0; m: 80%; y: 90; k: 10; }
		@-bag-color shade { model: gray; g: 50; }
		@-bag-color byte  { model: RGB; r: 255; g: 0; b: 0; }
		@-bag-color pct   { model: rgb; r: 0; g: 100; b: 0; }
		@-bag-color alias { value: brand; }
		@-bag-color dev   { value: device-cmyk(1 0 0 0); }
	`)
	testdata := []struct{ name, want string }{
		{"muted", "0.42 0.42 0.42 rg"},
		{"brand", "0 0.8 0.9 0.1 k"},
		{"shade", "0.5 g"},
		{"byte", "1 0 0 rg"},
		{"pct", "0 1 0 rg"},
		{"alias", "0 0.8 0.9 0.1 k"},
		{"dev", "1 0 0 0 k"},
	}
	for _, tc := range testdata {
		col := fe.GetColor(tc.name)
		if col == nil {
			t.Errorf("GetColor(%q) = nil", tc.name)
			continue
		}
		if got := col.PDFStringNonStroking(); got != tc.want {
			t.Errorf("GetColor(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A spot color must be registered once, even though the colors get
// registered again for every HTML chunk.
func TestBagColorSpot(t *testing.T) {
	c, fe := newColorTestDoc(t, `
		@-bag-color spot { model: spotcolor; colorname: "PANTONE 300 C"; c: 100; m: 44; y: 0; k: 0; }
		@-bag-color ink  { model: spotcolor; }
	`)
	if err := AddColorsFromCSS(c, fe); err != nil {
		t.Fatalf("second AddColorsFromCSS: %v", err)
	}
	spot := fe.GetColor("spot")
	if spot.Space != color.ColorSpotcolor || spot.Basecolor != "PANTONE 300 C" {
		t.Errorf("spot = %+v", spot)
	}
	if spot.C != 1 || spot.M != 0.44 {
		t.Errorf("fallback tint = %v %v, want 1 0.44", spot.C, spot.M)
	}
	if got, want := spot.PDFStringNonStroking(), "/CS1 cs 1 scn "; got != want {
		t.Errorf("spot operator = %q, want %q", got, want)
	}
	ink := fe.GetColor("ink")
	if ink.Basecolor != "ink" || ink.SpotcolorID != 2 {
		t.Errorf("ink = %+v, want Basecolor ink and SpotcolorID 2", ink)
	}
}

// The name is usable wherever CSS takes a color.
func TestBagColorInStyles(t *testing.T) {
	_, fe := newColorTestDoc(t, `@-bag-color muted { value: #6A6A6A; }`)
	ih := baseStyles()
	if err := StylesToStyles(ih, StyleMap{"color": textValue("muted"), "background-color": textValue("muted")}, fe, ih.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	if got := rgb(t, ih.color); got != "rgba(107,107,107,1)" {
		t.Errorf("color = %s", got)
	}
	if got := rgb(t, ih.BackgroundColor); got != "rgba(107,107,107,1)" {
		t.Errorf("background-color = %s", got)
	}
}

func TestBagColorErrors(t *testing.T) {
	testdata := []struct{ css, want string }{
		{`@-bag-color { value: red; }`, "missing color name"},
		{`@-bag-color x { model: hsl; }`, `model "hsl" not recognized`},
		{`@-bag-color x { r: 1; }`, "a model or a value is required"},
		{`@-bag-color x { model: cmyk; c: 1; }`, "needs the descriptor m"},
		{`@-bag-color x { value: red; opacity: 1; }`, `unknown descriptor "opacity"`},
		{`@-bag-color x { value: nosuchcolor; }`, `cannot parse color value "nosuchcolor"`},
	}
	for _, tc := range testdata {
		c := NewCSSParser()
		err := c.AddCSSText(tc.css)
		if err == nil {
			fe, _ := frontend.NewForWriter(&bytes.Buffer{})
			err = AddColorsFromCSS(c, fe)
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.css, err, tc.want)
		}
	}
}

// A spot color reaches the PDF as a Separation color space: its name may hold
// spaces, and the page resources refer to the object. Both were broken, so a
// spot color was not painted at all.
func TestSpotColorInPDF(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.AddCSS(`@-bag-color ink { model: spotcolor; colorname: "PANTONE 300 C"; c: 100; m: 44; y: 0; k: 0; }
p { color: ink }`); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<p>Text</p>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Separation /PANTONE#20300#20C ")) {
		t.Error("the Separation array does not name the ink")
	}
	if !regexp.MustCompile(`/CS1 \d+ 0 R`).Match(buf.Bytes()) {
		t.Error("the page resources do not refer to the color space")
	}
}

// overprint: yes puts the color's graphics state in front of its operators,
// for names, for values and for spot colors.
func TestBagColorOverprint(t *testing.T) {
	_, fe := newColorTestDoc(t, `
		@-bag-color ink   { model: cmyk; c: 0; m: 0; y: 0; k: 100; overprint: yes; }
		@-bag-color plain { model: cmyk; c: 0; m: 0; y: 0; k: 100; overprint: no; }
		@-bag-color val   { value: device-cmyk(0 0 0 1); overprint: yes; }
		@-bag-color spot  { model: spotcolor; colorname: "PANTONE 300 C"; overprint: yes; }
	`)
	on := "/" + string(pdf.OverprintOn.ResourceName()) + " gs "
	testdata := []struct {
		name string
		want bool
	}{{"ink", true}, {"plain", false}, {"val", true}, {"spot", true}}
	for _, tc := range testdata {
		got := fe.GetColor(tc.name).PDFStringNonStroking()
		if strings.HasPrefix(got, on) != tc.want {
			t.Errorf("%s: %q, overprint want %v", tc.name, got, tc.want)
		}
	}
	if !fe.Doc.Overprint {
		t.Error("the document does not know about the overprinting color")
	}
}

// A color made from a name is a color of its own: overprint must not reach
// the shared predefined color.
func TestBagColorOverprintKeepsSharedColor(t *testing.T) {
	_, fe := newColorTestDoc(t, `@-bag-color ink { value: darkblue; overprint: yes; }`)
	if fe.GetColor("darkblue").Overprint {
		t.Error("darkblue overprints after being used as a value")
	}
	if !fe.GetColor("ink").Overprint {
		t.Error("ink lost its overprint")
	}
}

func TestBagColorOverprintValue(t *testing.T) {
	c := NewCSSParser()
	err := c.AddCSSText(`@-bag-color ink { model: cmyk; c: 0; m: 0; y: 0; k: 100; overprint: maybe; }`)
	if err == nil || !strings.Contains(err.Error(), "overprint must be yes or no") {
		t.Errorf("got %v", err)
	}
}

// The keyword paints nothing. A background simply leaves its rectangle out,
// but text without a color operator would come out black.
func TestColorTransparentKeyword(t *testing.T) {
	_, fe := newColorTestDoc(t, ``)
	ih := baseStyles()
	if err := StylesToStyles(ih, StyleMap{"color": textValue("transparent"), "background-color": textValue("transparent")}, fe, ih.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	if got, want := ih.color.PDFStringNonStroking(), "/GSca0 gs 0 0 0 rg"; got != want {
		t.Errorf("color = %q, want %q", got, want)
	}
	if ih.BackgroundColor.Space != color.ColorNone {
		t.Errorf("background-color has space %v, want none", ih.BackgroundColor.Space)
	}
}

// countInPDF renders html and counts s in the uncompressed PDF.
func countInPDF(t *testing.T, html, s string) int {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	fe.Doc.CompressLevel = 0
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatal(err)
	}
	return bytes.Count(buf.Bytes(), []byte(s))
}

// background-color is not inherited. A child painting the background of its
// parent again cannot be seen with an opaque color, but a transparent one
// comes out darker. Rows and row groups paint nothing themselves, so their
// cells paint for them.
func TestBackgroundIsPaintedOnce(t *testing.T) {
	const bg = `style="background-color:rgba(255,0,0,.5)"`
	testdata := []struct {
		name, html string
		want       int
	}{
		{"block with a paragraph", `<div ` + bg + `><p>a</p><p>b</p></div>`, 1},
		{"nested blocks", `<div ` + bg + `><div><div><p>a</p></div></div></div>`, 1},
		{"list", `<ul ` + bg + `><li>a</li><li>b</li></ul>`, 1},
		// One box per run, each from the font of the run. They abut.
		{"inline with a child", `<p><span ` + bg + `>a <b>b</b> c</span></p>`, 3},
		{"table", `<table ` + bg + `><tr><td>a</td><td>b</td></tr></table>`, 1},
		{"row", `<table><tr ` + bg + `><td>a</td><td>b</td></tr><tr><td>c</td></tr></table>`, 2},
		{"row group", `<table><tbody ` + bg + `><tr><td>a</td></tr><tr><td>b</td></tr></tbody></table>`, 2},
		{"head", `<table><thead ` + bg + `><tr><th>a</th></tr></thead><tbody><tr><td>b</td></tr></tbody></table>`, 1},
		{"table in a cell of a row", `<table><tr ` + bg + `><td><table><tr><td>a</td></tr></table></td></tr></table>`, 1},
	}
	for _, tc := range testdata {
		t.Run(tc.name, func(t *testing.T) {
			if got := countInPDF(t, tc.html, "/GSca0_5 gs"); got != tc.want {
				t.Errorf("background painted %d times, want %d", got, tc.want)
			}
		})
	}
}

// CSS stacks row group, row and cell. In each table the operators are counted:
// the backgrounds that can be seen are painted once per cell.
func TestCellBackgroundStack(t *testing.T) {
	const red, green, blue = "rgba(255,0,0,.5)", "rgba(0,255,0,.25)", "rgba(0,0,255,.75)"
	for name, tc := range map[string]struct {
		html       string
		half       int // /GSca0_5
		quarter    int // /GSca0_25
		threeQuart int // /GSca0_75
	}{
		"transparent cell on a transparent row": {
			html: `<table><tr style="background-color:` + red + `"><td style="background-color:` + green + `">a</td><td>b</td></tr></table>`,
			half: 2, quarter: 1,
		},
		"opaque cell hides the row": {
			html: `<table><tr style="background-color:` + red + `"><td style="background-color:#fff">a</td></tr></table>`,
		},
		"transparent row on a transparent row group": {
			html: `<table><tbody style="background-color:` + red + `"><tr style="background-color:` + green + `"><td>a</td></tr></tbody></table>`,
			half: 1, quarter: 1,
		},
		"opaque row hides the row group": {
			html:    `<table><tbody style="background-color:` + red + `"><tr style="background-color:#fff"><td style="background-color:` + green + `">a</td></tr></tbody></table>`,
			quarter: 1,
		},
		"all three": {
			html: `<table><tbody style="background-color:` + red + `"><tr style="background-color:` + green + `"><td style="background-color:` + blue + `">a</td></tr></tbody></table>`,
			half: 1, quarter: 1, threeQuart: 1,
		},
		"sibling rows keep their own stack": {
			html: `<table><tbody style="background-color:` + red + `"><tr style="background-color:` + green + `"><td>a</td></tr><tr style="background-color:` + blue + `"><td>b</td></tr></tbody></table>`,
			half: 2, quarter: 1, threeQuart: 1,
		},
		"a fully transparent cell shows the row": {
			html: `<table><tr style="background-color:` + red + `"><td style="background-color:rgba(0,0,0,0)">a</td></tr></table>`,
			half: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := countInPDF(t, tc.html, "/GSca0_5 gs"); got != tc.half {
				t.Errorf("half alpha painted %d times, want %d", got, tc.half)
			}
			if got := countInPDF(t, tc.html, "/GSca0_25 gs"); got != tc.quarter {
				t.Errorf("quarter alpha painted %d times, want %d", got, tc.quarter)
			}
			if got := countInPDF(t, tc.html, "/GSca0_75 gs"); got != tc.threeQuart {
				t.Errorf("three quarter alpha painted %d times, want %d", got, tc.threeQuart)
			}
		})
	}
}

// The layers of a cell are painted in the order of the stack.
func TestCellBackgroundOrder(t *testing.T) {
	const html = `<table><tr style="background-color:rgba(255,0,0,.5)"><td style="background-color:rgba(0,255,0,.25)">a</td></tr></table>`
	vl := buildHTML(t, floatBuilder(t), html)
	pre := backgroundRule(vl)
	row, cell := strings.Index(pre, "/GSca0_5 gs"), strings.Index(pre, "/GSca0_25 gs")
	if row < 0 || cell < row {
		t.Errorf("row at %d, cell at %d in %q, want the row first", row, cell, pre)
	}
}

// background-color: transparent is the color "none", which draws nothing. As a
// rectangle it came out black.
func TestTransparentCellShowsTheRow(t *testing.T) {
	const html = `<table><tr style="background-color:#ff0000"><td style="background-color:transparent">a</td></tr></table>`
	vl := buildHTML(t, floatBuilder(t), html)
	pre := backgroundRule(vl)
	if !strings.Contains(pre, "1 0 0 rg") {
		t.Errorf("the cell does not paint the row: %q", pre)
	}
}

// backgroundRule finds the rule that paints the background below n.
func backgroundRule(n node.Node) string {
	for ; n != nil; n = n.Next() {
		switch v := n.(type) {
		case *node.Rule:
			if strings.Contains(v.Pre, " re f") {
				return v.Pre
			}
		case *node.VList:
			if pre := backgroundRule(v.List); pre != "" {
				return pre
			}
		case *node.HList:
			if pre := backgroundRule(v.List); pre != "" {
				return pre
			}
		}
	}
	return ""
}

// The background reaches the bottom edge of the box, which lies the depth of
// the content below its baseline (htmlbag#106).
func TestBackgroundCoversDepth(t *testing.T) {
	fe, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	vl := node.NewVList()
	vl.Width = bag.MustSP("100pt")
	vl.Height = bag.MustSP("20pt")
	vl.Depth = bag.MustSP("5pt")
	got := backgroundRule(cb.HTMLBorder(vl, HTMLValues{BackgroundColor: fe.GetColor("red")}))
	if want := "0 -25 100 25 re f"; !strings.Contains(got, want) {
		t.Errorf("background rule %q, want the rectangle %q", got, want)
	}
}
