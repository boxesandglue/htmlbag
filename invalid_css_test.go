package htmlbag

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

var invalidCSSValues = []string{
	"nonsense", "12", "-5", "0", "#xyz", "12foo", "url(nope)", `"str"`, "1 2 3 4 5", "calc(1 + 2)",
	"red | blue", "abc%", "1 2%", "10pt 50%", "!!", "50%", "2em", "auto", "thin", "none", "inherit",
}

// renderInvalid typesets one stylesheet and returns the panic, if any.
func renderInvalid(css, html string) (p any, err error) {
	defer func() { p = recover() }()
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		return nil, err
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		return nil, err
	}
	if err := cb.AddCSS(css); err != nil {
		return nil, err
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		return nil, err
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		return nil, err
	}
	return nil, fe.Finish()
}

// TestInvalidValuesDoNotPanic feeds every documented property values it cannot
// use. A stylesheet may be wrong, but that is a warning or an error, not a
// crash of the host program.
func TestInvalidValuesDoNotPanic(t *testing.T) {
	const doc = `<html><body><h1 class="x">Title</h1><p class="x">Text <span class="x">span</span></p>
<ul class="x"><li class="x">item</li></ul><table class="x"><tr class="x"><td class="x">cell</td></tr></table></body></html>`
	discardLog(t)
	// A run that stops early with an error would hide everything behind it.
	if p, err := renderInvalid("", doc); p != nil || err != nil {
		t.Fatalf("the document itself does not render: panic %v, error %v", p, err)
	}
	for _, spec := range Properties {
		for _, name := range spec.Names() {
			for _, v := range invalidCSSValues {
				var css string
				switch spec.Group {
				case GroupPage:
					css = fmt.Sprintf("@page { %s: %s; }", name, v)
				case GroupFontFace:
					css = fmt.Sprintf(`@font-face { font-family: Foo; src: local("CrimsonPro Regular"); %s: %s; } p { font-family: Foo, serif; }`, name, v)
				case GroupColor:
					css = fmt.Sprintf("@-bag-color foo { %s: %s; } p { color: foo; }", name, v)
				default:
					css = fmt.Sprintf(".x { %s: %s; }", name, v)
				}
				p, _ := renderInvalid(css, doc)
				if p == nil {
					continue
				}
				t.Errorf("%s: panic: %v", css, p)
			}
		}
	}
}

func discardLog(t *testing.T) {
	t.Helper()
	old := bag.Logger
	bag.SetLogger(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { bag.SetLogger(old) })
}

// warningsOf returns the warnings of a render of css and html.
func warningsOf(t *testing.T, css, html string) []string {
	t.Helper()
	var p any
	var err error
	log := strings.TrimSpace(captureWarnings(t, func() { p, err = renderInvalid(css, html) }))
	if p != nil || err != nil {
		t.Fatalf("panic %v, error %v", p, err)
	}
	if log == "" {
		return nil
	}
	return strings.Split(log, "\n")
}

func TestInvalidCSSWarns(t *testing.T) {
	const doc = `<html><body><p class="x">Text</p></body></html>`
	for _, tc := range []struct{ name, css, html, want string }{
		{"unknown display", ".x { display: flex; }", doc, "display value is not supported"},
		{"display of a table part on a block", ".x { display: table-cell; }", doc, "display value is not supported"},
		{"unexpected token", ".x { color: red | blue; }", doc, "unexpected token"},
		{"size-adjust", `@font-face { font-family: F; src: local("CrimsonPro Regular"); size-adjust: nonsense; }`, doc, "size-adjust needs a percentage"},
		{"font-style", `@font-face { font-family: F; src: local("CrimsonPro Regular"); font-style: slanted; }`, doc, "unknown font-style"},
		{"descriptor", `@font-face { font-family: F; src: local("CrimsonPro Regular"); frobnicate: 1; }`, doc, "unknown descriptor"},
		{"axis tag", ".x { font-variation-settings: 1 2; }", doc, "axis tag has four characters"},
		{"percentage", ".x { font-size: 10pt 50%; }", doc, "cannot read a percentage"},
		{"style attribute", "", `<html><body><p style="display: grid">Text</p></body></html>`, "display value is not supported"},
		{"page margin box", `@page { @top-center { content: "a" | "b"; } }`, doc, "unexpected token"},
		{"hyphenate-limit-chars", ".x { hyphenate-limit-chars: six; }", doc, "hyphenate-limit-chars value is not supported"},
		{"font-variant-numeric", ".x { font-variant-numeric: lining-nums oldstyle-nums; }", doc, "font-variant-numeric value is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(warningsOf(t, tc.css, tc.html), "\n")
			if !strings.Contains(got, tc.want) {
				t.Errorf("want a warning with %q, got %q", tc.want, got)
			}
		})
	}
}

// One bad rule on many elements is one mistake.
func TestInvalidCSSWarnsOnce(t *testing.T) {
	doc := "<html><body>" + strings.Repeat(`<p class="x">Text</p>`, 50) + "</body></html>"
	got := warningsOf(t, ".x { display: flex; color: red | blue; hyphenate-limit-chars: six; font-variant-numeric: bold; }", doc)
	if len(got) != 4 {
		t.Errorf("want 4 warnings, got %d: %q", len(got), got)
	}
}

// CSS-wide keywords and var() are valid whatever the property takes.
func TestCSSWideKeywordsDoNotWarn(t *testing.T) {
	const doc = `<html><body><p class="x">Text</p></body></html>`
	for _, value := range []string{"inherit", "initial", "unset", "revert", "revert-layer", "var(--v, 3)"} {
		for _, property := range []string{"hyphenate-limit-chars", "font-variant-numeric"} {
			if got := warningsOf(t, ".x { "+property+": "+value+"; }", doc); len(got) != 0 {
				t.Errorf("%s: %s warns: %q", property, value, got)
			}
		}
	}
}

// Style attributes come one by one, a syntax highlighter writes one per line.
func TestInvalidStyleAttributeWarnsOnce(t *testing.T) {
	doc := "<html><body>" + strings.Repeat(`<p style="display: flex; color: red | blue">Text</p>`, 50) + "</body></html>"
	got := warningsOf(t, ".x { display: flex; }", doc)
	if len(got) != 2 {
		t.Errorf("want 2 warnings, got %d: %q", len(got), got)
	}
}

func TestUnknownPropertyWarnsOnce(t *testing.T) {
	const css = `.x { grid-template-columns: 1fr 1fr; } .y { grid-template-columns: 1fr; }`
	doc := `<html><body>` + strings.Repeat(`<p class="x" style="cursor: pointer">Text</p>`, 50) + `</body></html>`
	got := strings.Join(warningsOf(t, css, doc), "\n")
	for _, property := range []string{"grid-template-columns", "cursor"} {
		if n := strings.Count(got, "property="+property); n != 1 {
			t.Errorf("%s reported %d times: %q", property, n, got)
		}
	}
}

// A misspelled property of our own is a mistake, a vendor prefix is not. The
// descriptors of the at-rules are checked where they are read, and the content
// of @keyframes has no meaning on paper.
func TestUnknownPropertyCheckIsNarrow(t *testing.T) {
	const doc = `<html><body><p class="x">Text</p></body></html>`
	if got := strings.Join(warningsOf(t, ".x { -bag-font-expansoin: 0%; }", doc), ""); !strings.Contains(got, "-bag-font-expansoin") {
		t.Errorf("want a warning for the misspelled property, got %q", got)
	}
	const quiet = `.x { -webkit-print-color-adjust: exact; -moz-hyphens: auto; --mine: 1; }
	@page { size: a5; margin: 1cm; @bottom-center { content: counter(page); font-size: 8pt; } }
	@font-face { font-family: F; src: local("CrimsonPro Regular"); font-display: swap; }
	@-bag-color brand { model: cmyk; c: 100; m: 0; y: 0; k: 0; }
	@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }`
	if got := warningsOf(t, quiet, doc); len(got) != 0 {
		t.Errorf("want no warnings, got %q", got)
	}
}

// The default stylesheet declares table parts and list items, and valid CSS
// must not cry wolf.
func TestValidCSSDoesNotWarn(t *testing.T) {
	const css = `:root { --d: block; --any: a | b; }
	p { display: var(--d); color: red !important; font: italic bold 10pt/12pt serif; }
	h1 { display: inline; } h2 { display: none; } h3 { display: flow-root; } h4 { display: inherit; }
	@font-face { font-family: F; src: local("CrimsonPro Regular"); font-style: oblique 10deg; size-adjust: 95%; unicode-range: U+4??, U+0025-00FF; }`
	const doc = `<html><body><h1>T</h1><h2>T</h2><h3>T</h3><h4>T</h4><p>Text</p><ul><li>item</li></ul><table><thead><tr><th>h</th></tr></thead><tr><td>c</td></tr></table></body></html>`
	if got := warningsOf(t, css, doc); len(got) != 0 {
		t.Errorf("want no warnings, got %q", got)
	}
}

// An @page rule without a paper name is about the margins, not an error.
func TestPageRuleWithoutPaperName(t *testing.T) {
	for _, tc := range []struct{ css, width string }{
		{"@page { margin: 2cm; }", "210mm"},
		{"@page { size: landscape; }", "297mm"},
		{"@page { size: a5; }", "148mm"},
	} {
		fe, err := frontend.NewForWriter(&bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		cb, err := New(fe, NewCSSParserWithDefaults())
		if err != nil {
			t.Fatal(err)
		}
		if err := cb.AddCSS(tc.css); err != nil {
			t.Fatal(err)
		}
		ps, err := cb.PageSize()
		if err != nil {
			t.Fatalf("%s: %v", tc.css, err)
		}
		if want := bag.MustSP(tc.width); ps.Width != want {
			t.Errorf("%s: width %s, want %s", tc.css, ps.Width, want)
		}
	}
}

func TestFontFaceObliqueIsItalic(t *testing.T) {
	cp := NewCSSParser()
	if err := cp.AddCSSText(`@font-face { font-family: F; src: url("f.ttf"); font-style: oblique 10deg; }`); err != nil {
		t.Fatal(err)
	}
	if got := cp.FontFaces[0].Style; got != "italic" {
		t.Errorf("got font-style %q", got)
	}
}
