package htmlbag

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
)

const twoPages = `<p>One.</p><p style="page-break-before: always">Two.</p>`

// ExtraOffset is the room around the trimmed sheet: the bleed and, with crop
// marks, 1cm for the marks, which start at the bleed but at least 5pt out.
func TestPageMarksOffset(t *testing.T) {
	bleed := bag.MustSP("3mm")
	marks := bag.MustSP("1cm")
	for _, tc := range []struct {
		name, css string
		want      bag.ScaledPoint
	}{
		{"nothing", ``, 0},
		{"bleed", `bleed: 3mm`, bleed},
		{"marks", `marks: crop`, marks + defaultBleed},
		{"both", `bleed: 3mm; marks: crop`, marks + bleed},
		{"auto bleed", `bleed: auto; marks: crop`, marks + defaultBleed},
		{"no bleed", `bleed: 0; marks: crop`, marks + 5*bag.Factor},
		{"bleed below the mark distance", `bleed: 1mm; marks: crop`, marks + 5*bag.Factor},
		{"no marks", `bleed: auto; marks: none`, 0},
	} {
		pages := renderHTMLPages(t, `@page { size: a6; margin: 10mm; `+tc.css+` }`, twoPages)
		if len(pages) != 2 {
			t.Fatalf("%s: got %d pages, want 2", tc.name, len(pages))
		}
		for i, pg := range pages {
			if pg.ExtraOffset != tc.want {
				t.Errorf("%s: page %d offset %s, want %s", tc.name, i+1, pg.ExtraOffset, tc.want)
			}
		}
	}
}

// A pseudo page inherits the generic rule, so the rule of the first page sees
// a bleed declared on @page, and declares it itself for the whole document.
func TestPageMarksFirstPageRule(t *testing.T) {
	bleed := bag.MustSP("3mm")
	for _, css := range []string{
		`@page { size: a6; bleed: 3mm } @page :first { margin: 20mm }`,
		`@page { size: a6 } @page :first { bleed: 3mm }`,
	} {
		for i, pg := range renderHTMLPages(t, css, twoPages) {
			if pg.ExtraOffset != bleed {
				t.Errorf("%s: page %d offset %s, want %s", css, i+1, pg.ExtraOffset, bleed)
			}
		}
	}
}

func TestPageMarksWarn(t *testing.T) {
	const doc = `<html><body><p>Text</p></body></html>`
	for _, tc := range []struct{ name, css, want string }{
		{"cross marks", `@page { marks: crop cross }`, "only crop marks"},
		{"no length", `@page { bleed: wide }`, "not a length"},
		{"negative", `@page { bleed: -3mm }`, "not a length"},
	} {
		got := strings.Join(warningsOf(t, tc.css, doc), "\n")
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: got warnings %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A rule without bleed and marks declares nothing, so a setting made on the
// document by other means stays.
func TestPageMarksKeepDocumentSetting(t *testing.T) {
	bleed := bag.MustSP("5mm")
	cb := renderForOutline(t, `@page { size: a6 }`, `<p>Text</p>`, func(cb *CSSBuilder) {
		cb.frontend.Doc.Bleed = bleed
	})
	if cb.frontend.Doc.Bleed != bleed {
		t.Errorf("bleed %s, want %s", cb.frontend.Doc.Bleed, bleed)
	}
}

// pageObject returns the placed object with the given origin.
func pageObject(t *testing.T, pg *document.Page, origin string) document.Object {
	t.Helper()
	for _, obj := range pg.Objects {
		if obj.Vlist == nil {
			continue
		}
		if o, _ := obj.Vlist.Attributes["origin"].(string); o == origin {
			return obj
		}
	}
	t.Fatalf("no object %q on the page", origin)
	return document.Object{}
}

// The page background runs into the bleed on all sides, on every page. Without
// a bleed it has to stop at the sheet, where the references have it.
func TestPageBackgroundFillsBleed(t *testing.T) {
	wd, ht := bag.MustSP("105mm"), bag.MustSP("148mm")
	for _, bleed := range []string{"0mm", "3mm"} {
		b := bag.MustSP(bleed)
		css := `@page { size: a6; background-color: #ffe0e0; background-image: url("testdata/float.png"); bleed: ` + bleed + ` }`
		for i, pg := range renderHTMLPages(t, css, twoPages) {
			col := pageObject(t, pg, "page background color")
			if col.X != -b || col.Y != ht+b {
				t.Errorf("bleed %s, page %d: color at %s/%s", bleed, i+1, col.X, col.Y)
			}
			want := fmt.Sprintf("%s %s %s %s re", bag.ScaledPoint(0), bag.ScaledPoint(0), wd+2*b, -ht-2*b)
			if got := pageBackgroundColor(pg); !strings.Contains(got, want) {
				t.Errorf("bleed %s, page %d: color %q, want %q", bleed, i+1, got, want)
			}
			img := pageObject(t, pg, "page background image")
			if img.X != -b || img.Y != ht+b {
				t.Errorf("bleed %s, page %d: image at %s/%s", bleed, i+1, img.X, img.Y)
			}
			if img.Vlist.Width != wd+2*b || img.Vlist.Height+img.Vlist.Depth != ht+2*b {
				t.Errorf("bleed %s, page %d: image %s x %s", bleed, i+1, img.Vlist.Width, img.Vlist.Height+img.Vlist.Depth)
			}
		}
	}
}

// outlineTop returns the height a /XYZ destination of the first outline entry
// jumps to.
func outlineTop(t *testing.T, cb *CSSBuilder) float64 {
	t.Helper()
	outlines := cb.frontend.Doc.PDFWriter.Outlines
	if len(outlines) == 0 {
		t.Fatal("no outline")
	}
	var obj, gen int
	var top float64
	if _, err := fmt.Sscanf(outlines[0].Dest, "[%d %d R /XYZ null %g null]", &obj, &gen, &top); err != nil {
		t.Fatalf("destination %q: %v", outlines[0].Dest, err)
	}
	return top
}

// An outline destination counts from the media box, and the media box grows by
// the bleed below the sheet, so the destination has to rise with it.
func TestOutlineDestinationWithBleed(t *testing.T) {
	const html = `<h1>Title</h1><p>Text</p>`
	top := func(rule string) float64 {
		t.Helper()
		return outlineTop(t, renderForOutline(t, `@page { size: a6; margin: 10mm; `+rule+` }`, html, nil))
	}
	got := top(`bleed: 3mm`) - top(``)
	want := bag.MustSP("3mm").ToPT()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("destination moved by %g, want %g", got, want)
	}
}

// Crop marks are drawn in the colorant All, so they print on a spot color's
// plate too. A document without marks does not carry the color space.
func TestCropMarksOnEveryPlate(t *testing.T) {
	const all = "/Separation /All "
	withMarks := renderPDF(t, `@page { size: a6; marks: crop }`, `<p>Text</p>`)
	if !bytes.Contains(withMarks, []byte(all)) {
		t.Error("no Separation of the colorant All")
	}
	if !regexp.MustCompile(`/CSAll \d+ 0 R`).Match(withMarks) {
		t.Error("the page resources do not refer to the color space")
	}
	if plain := renderPDF(t, `@page { size: a6 }`, `<p>Text</p>`); bytes.Contains(plain, []byte(all)) {
		t.Error("a document without marks has the color space")
	}
}

// OutputPageBox is what a caller with its own pages gets of the @page rule. A
// rule that draws nothing must not touch the page, the QA references of xts
// have such rules.
func TestOutputPageBox(t *testing.T) {
	for _, tc := range []struct {
		name, css string
		want      int
	}{
		{"nothing", `@page { size: a6; margin: 1cm }`, 0},
		{"color", `@page { size: a6; background-color: #ffe0e0 }`, 1},
		{"border", `@page { size: a6; border: 1pt solid red }`, 1},
		{"both", `@page { size: a6; background-color: #ffe0e0; border: 1pt solid red }`, 2},
	} {
		cb := renderForOutline(t, tc.css, `<p>Text</p>`, nil)
		pg := cb.frontend.Doc.NewPage()
		wd, ht := bag.MustSP("105mm"), bag.MustSP("148mm")
		page, ok := cb.css.NamedPage("")
		if !ok {
			t.Fatalf("%s: no @page rule", tc.name)
		}
		pd := PageDimensions{Width: wd, Height: ht, MarginTop: onecm, MarginBottom: onecm, MarginLeft: onecm, MarginRight: onecm}
		if err := cb.OutputPageBox(pd, page); err != nil {
			t.Fatal(err)
		}
		if got := len(pg.Objects); got != tc.want {
			t.Errorf("%s: %d objects, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPDFAColorAndLinks(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	fe.Doc.Format = document.FormatPDFA3b
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.AddCSS(`p { color: #336699 }`); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<p><a href="https://example.com">Link</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal(err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`/DefaultRGB \d+ 0 R`).Match(buf.Bytes()) {
		t.Error("the page resources have no DefaultRGB")
	}
	if !regexp.MustCompile(`(?s)/Annot.*/F 4\b|/F 4\b.*/Annot`).Match(buf.Bytes()) {
		t.Error("the link has no print flag")
	}
}
