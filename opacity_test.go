package htmlbag

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

// renderOpacityPDF renders html to an uncompressed PDF.
func renderOpacityPDF(t *testing.T, html string) string {
	t.Helper()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	fe.Doc.CompressLevel = 0
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}
	if err := fe.Finish(); err != nil {
		t.Fatalf("frontend.Finish: %v", err)
	}
	return buf.String()
}

var paintsGroup = regexp.MustCompile(`q /GSca(\S+)CA\S+ gs /FmOp\d+ Do Q`)

// groupAlphas returns the alpha values with which out paints opacity groups,
// in the order of the content streams.
func groupAlphas(out string) []string {
	var alphas []string
	for _, m := range paintsGroup.FindAllStringSubmatch(out, -1) {
		alphas = append(alphas, m[1])
	}
	return alphas
}

func TestParseOpacity(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
		ok   bool
	}{
		{"0.5", 0.5, true},
		{" .25 ", 0.25, true},
		{"40%", 0.4, true},
		{"0", 0, true},
		{"1.5", 1, true},
		{"-1", 0, true},
		{"half", 0, false},
		{"", 0, false},
	} {
		got, ok := parseOpacity(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseOpacity(%q) = %v, %v, want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// opacity paints the box of a block as one object: its background, border
// and text go into one group, also without a border or background, and also
// on a table. An inline element and a table cell are not read.
func TestOpacityBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
		want []string
	}{
		{"box", `<div style="opacity: .5; background-color: #0a0; border: 1pt solid blue">Text</div>`, []string{"0_5"}},
		{"bare", `<p style="opacity: 30%">Text</p>`, []string{"0_3"}},
		{"nested", `<div style="opacity: .5">A <div style="opacity: .25">B</div></div>`, []string{"0_25", "0_5"}}, // sorted
		{"table", `<table style="opacity: .5"><tr><td>A</td></tr></table>`, []string{"0_5"}},
		{"opaque", `<p style="opacity: 1">Text</p>`, nil},
		{"span", `<p>A <span style="opacity: .5">B</span></p>`, nil},
		{"inline svg", `<p>A <svg style="opacity: .5" width="10" height="10"><rect width="10" height="10"/></svg></p>`, []string{"0_5"}},
		{"svg image", `<p>A <img style="opacity: .5; width: 1cm" src="testdata/float.svg"></p>`, []string{"0_5"}},
		{"svg image in percent", `<p>A <img style="opacity: .5; width: 10%" src="testdata/float.svg"></p>`, []string{"0_5"}},
		{"svg image alone in a cell", `<table><tr><td><img style="opacity: .5; width: 50%" src="testdata/float.svg"></td></tr></table>`, []string{"0_5"}},
		{"cell", `<table><tr><td style="opacity: .5">A</td></tr></table>`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := renderOpacityPDF(t, `<html><body>`+tc.html+`</body></html>`)
			got := groupAlphas(out)
			slices.Sort(got)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("groups with alpha %v, want %v", got, tc.want)
			}
		})
	}
}

// A block split over pages is painted as a group on each page.
func TestOpacitySplitBlock(t *testing.T) {
	text := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 300)
	out := renderOpacityPDF(t, `<html><body>
<div style="opacity: .5; background-color: #ccc">`+text+`</div></body></html>`)
	pages := len(regexp.MustCompile(`/Type\s*/Page[^s]`).FindAllString(out, -1))
	if pages < 2 {
		t.Fatalf("the block fits on %d page", pages)
	}
	if got := len(groupAlphas(out)); got != pages {
		t.Errorf("%d groups on %d pages", got, pages)
	}
}

// A bitmap is drawn with the alpha constant of a graphics state, without a
// group, also when its size waits for the width of its container.
func TestOpacityBitmapImage(t *testing.T) {
	alpha := regexp.MustCompile(`q /GSca0_5CA0_5 gs\nq [^\n]*/ImgBag1 Do Q\nQ`)
	for _, img := range []string{
		`<img style="opacity: .5; width: 1cm" src="testdata/float.png">`,
		`<img style="opacity: .5; width: 10%" src="testdata/float.png">`,
		`<img style="opacity: .5; display: block; width: 1cm" src="testdata/float.png">`,
	} {
		out := renderOpacityPDF(t, `<html><body><p>A `+img+`</p></body></html>`)
		if !alpha.MatchString(out) {
			t.Errorf("%s is not drawn with alpha 0.5", img)
		}
	}
}
