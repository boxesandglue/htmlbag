package htmlbag

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/cascadia"
)

func TestNestedAtrule(t *testing.T) {

	str := `
	@page {
		size: a5;
		@bottom-right-corner {
			border: 4pt solid green;
			border-bottom-color: rebeccapurple;
		}

		/* @top-left-corner {
			border: 1pt solid green;
			border-bottom-color: rebeccapurple;
		} */

	@top-right-corner {
			border: 3pt solid green;
			border-bottom-color: rebeccapurple;
		}

		@bottom-left-corner {
			border: 2pt solid green;
			border-bottom-color: rebeccapurple;
		}

	}`
	toks := tokenizeCSSString(str)
	bl := consumeBlock(toks, false)
	if len(bl.childAtRules[0].childAtRules) != 3 {
		t.Errorf("want 3 child @ rules, got %d", len(bl.childAtRules[0].childAtRules))
	}
}

func TestFontFace(t *testing.T) {
	str := `@font-face {
		font-family: "Trickster";
		src:
		  local("Trickster"),
		  url("trickster-COLRv1.otf") format("opentype") tech(color-COLRv1),
		  url("trickster-outline.otf") format("opentype"),
		  url("trickster-outline.woff") format("woff");
	  }`
	cp := NewCSSParser()
	err := cp.AddCSSText(str)
	if err != nil {
		t.Error(err)
	}
	fontfaces := cp.FontFaces
	if got, want := len(fontfaces), 1; got != want {
		t.Errorf("len(c.FontFaces) = %d, want %d", got, want)
	}
	firstFontFace := fontfaces[0]
	if want, got := 4, len(firstFontFace.Source); got != want {
		t.Errorf("len(c.FontFaces[0].Source) = %d, want %d", got, want)
	}
	if want, got := "color-COLRv1", firstFontFace.Source[1].Tech; got != want {
		t.Errorf(`firstFontFace.Source[1].Tech = %s, want %s`, got, want)
	}
}

func TestFontFaceWeightRange(t *testing.T) {
	str := `
	@font-face {
		font-family: "VF";
		font-weight: 200 900;
		src: url("vf.ttf");
	}
	@font-face {
		font-family: "Single";
		font-weight: 500;
		src: url("single.ttf");
	}
	@font-face {
		font-family: "Spaced";
		font-weight: extra light;
		src: url("xl.ttf");
	}`
	cp := NewCSSParser()
	if err := cp.AddCSSText(str); err != nil {
		t.Error(err)
	}
	if got, want := len(cp.FontFaces), 3; got != want {
		t.Fatalf("len(c.FontFaces) = %d, want %d", got, want)
	}
	vf := cp.FontFaces[0]
	if vf.Weight != 200 || vf.WeightMax != 900 {
		t.Errorf("range = %d..%d, want 200..900", vf.Weight, vf.WeightMax)
	}
	single := cp.FontFaces[1]
	if single.Weight != 500 || single.WeightMax != 500 {
		t.Errorf("single = %d..%d, want 500..500", single.Weight, single.WeightMax)
	}
	spaced := cp.FontFaces[2]
	if spaced.Weight != 200 || spaced.WeightMax != 200 {
		t.Errorf("spaced keyword = %d..%d, want 200..200", spaced.Weight, spaced.WeightMax)
	}
}

func TestConsumeBlock_SimpleRules(t *testing.T) {
	css := `p { color: red; font-size: 12pt; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	if len(bl.blocks[0].rules) != 2 {
		t.Errorf("got %d rules, want 2", len(bl.blocks[0].rules))
	}
}

func TestConsumeBlock_MultipleSelectors(t *testing.T) {
	css := `h1 { color: blue; } p { color: red; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(bl.blocks))
	}
}

func TestConsumeBlock_ClassSelector(t *testing.T) {
	css := `.highlight { background: yellow; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := selectorString(bl.blocks[0].componentValues)
	if !strings.Contains(sel, ".highlight") {
		t.Errorf("selector = %q, want to contain '.highlight'", sel)
	}
}

func TestConsumeBlock_IDSelector(t *testing.T) {
	css := `#main { width: 100%; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := selectorString(bl.blocks[0].componentValues)
	if !strings.Contains(sel, "#main") {
		t.Errorf("selector = %q, want to contain '#main'", sel)
	}
}

// TestConsumeBlock_AttributeSelectors guards the selector round trip
// through selectorString: string values must be re-quoted (the scanner
// strips the quotes) and the five attribute match operators must be
// re-emitted (the scanner encodes them in the token type with an empty
// Value). Every serialized form must also survive cascadia's parser —
// that is the consumer the string is produced for.
func TestConsumeBlock_AttributeSelectors(t *testing.T) {
	cases := []struct{ css, want string }{
		{`a[href] { color: red; }`, `a[href]`},
		{`a[href="#deep"] { color: red; }`, `a[href="#deep"]`},
		{`a[href^="#"] { color: red; }`, `a[href^="#"]`},
		{`a[href$=".pdf"] { color: red; }`, `a[href$=".pdf"]`},
		{`a[href*="ref"] { color: red; }`, `a[href*="ref"]`},
		{`a[rel~="noopener"] { color: red; }`, `a[rel~="noopener"]`},
		{`a[lang|="en"] { color: red; }`, `a[lang|="en"]`},
		{`a[title="say \"hi\""] { color: red; }`, `a[title="say \"hi\""]`},
	}
	for _, c := range cases {
		toks := tokenizeCSSString(c.css)
		bl := consumeBlock(toks, false)
		if len(bl.blocks) != 1 {
			t.Fatalf("%s: got %d blocks, want 1", c.css, len(bl.blocks))
		}
		sel := selectorString(bl.blocks[0].componentValues)
		if sel != c.want {
			t.Errorf("selector = %q, want %q", sel, c.want)
		}
		if _, err := cascadia.ParseGroupWithPseudoElements(sel); err != nil {
			t.Errorf("cascadia rejects %q: %v", sel, err)
		}
	}
}

func TestConsumeBlock_DescendantSelector(t *testing.T) {
	css := `div p { margin: 0; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := strings.Join(strings.Fields(selectorString(bl.blocks[0].componentValues)), " ")
	if sel != "div p" {
		t.Errorf("selector = %q, want 'div p'", sel)
	}
}

func TestConsumeBlock_ChildCombinator(t *testing.T) {
	css := `ul > li { list-style: none; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := strings.Join(strings.Fields(selectorString(bl.blocks[0].componentValues)), " ")
	if sel != "ul > li" {
		t.Errorf("selector = %q, want 'ul > li'", sel)
	}
}

func TestConsumeBlock_RuleWithoutTrailingSemicolon(t *testing.T) {
	css := `p { color: red }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	if len(bl.blocks[0].rules) != 1 {
		t.Errorf("got %d rules, want 1", len(bl.blocks[0].rules))
	}
}

func TestConsumeBlock_EmptyBlock(t *testing.T) {
	css := `p { }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	if len(bl.blocks[0].rules) != 0 {
		t.Errorf("got %d rules, want 0", len(bl.blocks[0].rules))
	}
}

func TestSelectorString_IDNotDoubleHash(t *testing.T) {
	css := `#important { color: red; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := selectorString(bl.blocks[0].componentValues)
	if sel != "#important" {
		t.Errorf("selector = %q, want %q", sel, "#important")
	}
}

func TestApplyCSS_IDSelector(t *testing.T) {
	htmlStr := `<html><head></head><body><p id="important">text</p></body></html>`
	css := `#important { color: green; }`
	c := NewCSSParser()
	if err := c.AddCSSText(css); err != nil {
		t.Fatal(err)
	}
	doc, err := c.ProcessHTMLChunk(htmlStr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ApplyCSS(doc)
	if err != nil {
		t.Fatalf("ApplyCSS failed: %v", err)
	}
	p := doc.Find("#important")
	if val, exists := p.Attr("!color"); !exists || val != "green" {
		t.Errorf("#important !color = %q (exists=%v), want 'green'", val, exists)
	}
}

func TestConsumeBlock_PseudoClass(t *testing.T) {
	css := `a:hover { color: green; }`
	toks := tokenizeCSSString(css)
	bl := consumeBlock(toks, false)
	if len(bl.blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(bl.blocks))
	}
	sel := selectorString(bl.blocks[0].componentValues)
	if !strings.Contains(sel, ":hover") {
		t.Errorf("selector = %q, want to contain ':hover'", sel)
	}
}

// TestPageMarginLonghands: the margin longhands are valid page-context
// properties (CSS Paged Media 3 §7.2) and must reach the Page margin fields
// just like the margin shorthand. A longhand following the shorthand in the
// same rule overrides that side.
func TestPageMarginLonghands(t *testing.T) {
	str := `
	@page {
		size: a4;
		margin: 2cm;
		margin-right: 6cm;
	}
	@page :first {
		margin-top: 8cm;
	}`
	cp := NewCSSParser()
	if err := cp.AddCSSText(str); err != nil {
		t.Fatal(err)
	}
	base := cp.Pages[""]
	if got, want := base.MarginTop, "2cm"; got != want {
		t.Errorf("base MarginTop = %q, want %q", got, want)
	}
	if got, want := base.MarginRight, "6cm"; got != want {
		t.Errorf("base MarginRight = %q, want %q (longhand must override the shorthand)", got, want)
	}
	first := cp.Pages[":first"]
	if got, want := first.MarginTop, "8cm"; got != want {
		t.Errorf(":first MarginTop = %q, want %q", got, want)
	}
	if got := first.MarginLeft; got != "" {
		t.Errorf(":first MarginLeft = %q, want empty (inherited later via mergePageWithBase)", got)
	}
}

// TestPageURLResolvedAtParseTime: relative url() inside @page rules must be
// resolved against the declaring stylesheet (the dirstack top at parse time),
// not left raw for consumers that resolve after PopDir against the document
// directory (issue #3). Covers both the page attributes and margin box
// content tokens.
func TestPageURLResolvedAtParseTime(t *testing.T) {
	str := `
	@page {
		background-image: url(bg.pdf);
		@top-center {
			content: url("logo.svg");
		}
	}`
	dir := filepath.Join(string(filepath.Separator), "template", "dir")
	cp := NewCSSParser()
	cp.PushDir(dir)
	err := cp.AddCSSText(str)
	cp.PopDir()
	if err != nil {
		t.Fatal(err)
	}
	pg := cp.Pages[""]
	var bg string
	for _, attr := range pg.Attributes {
		if attr.Key == "!background-image" {
			bg = attr.Val
		}
	}
	if got, want := bg, "url("+filepath.Join(dir, "bg.pdf")+")"; got != want {
		t.Errorf("background-image = %q, want %q", got, want)
	}
	content := pg.PageAreaContent["top-center"]
	if len(content) != 1 {
		t.Fatalf("len(PageAreaContent[top-center]) = %d, want 1", len(content))
	}
	if got, want := content[0].Value, filepath.Join(dir, "logo.svg"); content[0].Type != ContentURL || got != want {
		t.Errorf("content token = (%d, %q), want (ContentURL, %q)", content[0].Type, got, want)
	}
}

// TestPageURLLeftAlone: absolute paths, fragment references, data: URIs and
// scheme URLs must pass through parse-time resolution unchanged, and without
// a dirstack a relative path stays raw.
func TestPageURLLeftAlone(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "abs", "bg.pdf")
	str := `
	@page {
		background-image: url(` + abs + `);
		-bag-a: url(#anchor);
		-bag-b: url(https://example.com/a.pdf);
		-bag-c: url(data:image/png;base64,AAAA);
	}
	@page nodir {
		background-image: url(raw.pdf);
	}`
	cp := NewCSSParser()
	cp.PushDir(filepath.Join(string(filepath.Separator), "template", "dir"))
	err := cp.AddCSSText(str)
	cp.PopDir()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"!background-image": "url(" + abs + ")",
		"!-bag-a":           "url(#anchor)",
		"!-bag-b":           "url(https://example.com/a.pdf)",
		"!-bag-c":           "url(data:image/png;base64,AAAA)",
	}
	for _, attr := range cp.Pages[""].Attributes {
		if w, ok := want[attr.Key]; ok && attr.Val != w {
			t.Errorf("%s = %q, want %q", attr.Key, attr.Val, w)
		}
	}

	cpNoDir := NewCSSParser()
	if err := cpNoDir.AddCSSText(str); err != nil {
		t.Fatal(err)
	}
	for _, attr := range cpNoDir.Pages["nodir"].Attributes {
		if attr.Key == "!background-image" {
			if got, want := attr.Val, "url(raw.pdf)"; got != want {
				t.Errorf("empty dirstack: background-image = %q, want %q", got, want)
			}
		}
	}
}
