package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
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
