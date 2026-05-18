package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
	"golang.org/x/net/html"
)

// TestResolveCSSFontFamilyAliasSansSerif guards CSS Fonts 4 §3.1.1 generic
// keyword resolution: the spec keyword is "sans-serif" but htmlbag registers
// the family under "sans" (see fonts.go LoadIncludedFonts). Through 2026-05-13
// the resolver did a verbatim FindFontFamily lookup and reported
// "Font family not found, reverting to 'serif'" — CSS-conformant stylesheets
// silently got CrimsonPro (serif) instead of TeXGyreHeros (sans). Regression
// trigger: any user-authored CSS that writes the spec keyword.
func TestResolveCSSFontFamilyAliasSansSerif(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	want := fe.FindFontFamily("sans")
	if want == nil {
		t.Fatal("sans family not registered by LoadIncludedFonts")
	}
	got := resolveCSSFontFamily("sans-serif", fe)
	if got != want {
		t.Errorf("resolveCSSFontFamily(%q) = %v, want sans family", "sans-serif", got)
	}
}

// TestResolveCSSFontFamilyCommaList guards CSS Fonts 4 §3.1 prioritised-list
// semantics: font-family values are comma-separated; the first family that
// resolves wins, the rest are fallbacks. The classic browser-CSS pattern
// `font-family: "Helvetica Neue", Arial, sans-serif` should resolve to the
// generic at the end when neither named font is registered. Through
// 2026-05-13 the resolver passed the whole string verbatim to
// FindFontFamily and missed every list of two or more entries.
func TestResolveCSSFontFamilyCommaList(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	want := fe.FindFontFamily("sans")
	got := resolveCSSFontFamily(`"Helvetica Neue", Arial, sans-serif`, fe)
	if got != want {
		t.Errorf("resolveCSSFontFamily(comma list) = %v, want sans family (last entry, generic alias)", got)
	}
}

// TestResolveCSSFontFamilyDirect verifies that the registered internal names
// (sans, serif, monospace) still resolve directly without going through the
// alias table. The alias map is consulted only on a miss, so a direct hit
// short-circuits the lookup.
func TestResolveCSSFontFamilyDirect(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	for _, name := range []string{"sans", "serif", "monospace"} {
		want := fe.FindFontFamily(name)
		if want == nil {
			t.Fatalf("%q family not registered", name)
		}
		if got := resolveCSSFontFamily(name, fe); got != want {
			t.Errorf("resolveCSSFontFamily(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestResolveCSSFontFamilyUnknownReturnsNil verifies the contract that
// resolveCSSFontFamily returns nil when no candidate resolves — the caller
// (StylesToStyles) decides the fallback (currently 'serif' with an error
// log). The resolver itself must not silently invent a result.
func TestResolveCSSFontFamilyUnknownReturnsNil(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	if got := resolveCSSFontFamily(`"Comic Sans MS", cursive`, fe); got != nil {
		t.Errorf("resolveCSSFontFamily(unknown stack) = %v, want nil", got)
	}
}

// TestOutputBodyHonorsCommaFontFamily verifies that the body switch in
// Output() now routes font-family through resolveCSSFontFamily, so a
// real-world list like `"Helvetica Neue", Arial, sans-serif` ends up
// setting the sans default family. Through 2026-04 the body branch passed
// the whole list string straight to FindFontFamily, which always missed.
func TestOutputBodyHonorsCommaFontFamily(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	// Seed a base frame so SetDefaultFontFamily inside Output() mutates
	// a frame whose pointer is also reachable through cb.stylesStack.
	base := cb.stylesStack.PushStyles()
	body := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "body",
		Attributes: map[string]string{},
		Styles:     map[string]string{"font-family": `"Helvetica Neue", Arial, sans-serif`},
	}
	if _, err := Output(cb, body, cb.stylesStack, fe, nil); err != nil {
		t.Fatalf("Output(body): %v", err)
	}
	want := fe.FindFontFamily("sans")
	if got := base.DefaultFontFamily; got != want {
		t.Errorf("after Output(body): DefaultFontFamily = %v, want sans family", got)
	}
}

// TestOutputHtmlHonorsCommaFontFamily mirrors the body test for the html
// branch — both call sites previously used the raw FindFontFamily path.
func TestOutputHtmlHonorsCommaFontFamily(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	base := cb.stylesStack.PushStyles()
	htmlItem := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "html",
		Attributes: map[string]string{},
		Styles:     map[string]string{"font-family": "Georgia, serif"},
	}
	if _, err := Output(cb, htmlItem, cb.stylesStack, fe, nil); err != nil {
		t.Fatalf("Output(html): %v", err)
	}
	want := fe.FindFontFamily("serif")
	if got := base.DefaultFontFamily; got != want {
		t.Errorf("after Output(html): DefaultFontFamily = %v, want serif family", got)
	}
}
