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

// TestResolveCSSFontFamilyListOrder verifies the stack resolver preserves
// declaration order and skips unresolvable entries silently. Per-glyph
// fallback walks the stack top-down for each grapheme cluster, so order
// is load-bearing — the primary at index 0 is what SettingFontFamily
// reports.
func TestResolveCSSFontFamilyListOrder(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	sans := fe.FindFontFamily("sans")
	serif := fe.FindFontFamily("serif")
	mono := fe.FindFontFamily("monospace")
	stack := resolveCSSFontFamilyList(`"Unknown One", monospace, "Helvetica Neue", sans-serif, serif`, fe)
	want := []*frontend.FontFamily{mono, sans, serif}
	if len(stack) != len(want) {
		t.Fatalf("stack length = %d, want %d (got %v)", len(stack), len(want), stack)
	}
	for i, ff := range want {
		if stack[i] != ff {
			t.Errorf("stack[%d] = %v, want %v", i, stack[i], ff)
		}
	}
}

// TestResolveCSSFontFamilyListDedup verifies the stack deduplicates: a family
// listed twice (directly + via its generic alias) contributes once at its
// first declared position. Deduplication is required for determinism —
// the per-glyph fallback would otherwise probe coverage on the same face
// twice.
func TestResolveCSSFontFamilyListDedup(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	sans := fe.FindFontFamily("sans")
	stack := resolveCSSFontFamilyList(`sans, sans-serif, "sans"`, fe)
	if len(stack) != 1 {
		t.Fatalf("stack length = %d, want 1 (got %v)", len(stack), stack)
	}
	if stack[0] != sans {
		t.Errorf("stack[0] = %v, want sans family", stack[0])
	}
}

// TestResolveCSSFontFamilyListEmpty verifies the stack returns nil for inputs
// where no candidate resolves (cursive/fantasy generics, unknown families).
// The wrapper resolveCSSFontFamily must mirror this with a nil return so
// the existing StylesToStyles fallback to 'serif' still triggers.
func TestResolveCSSFontFamilyListEmpty(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	if got := resolveCSSFontFamilyList(`"Comic Sans MS", cursive, fantasy`, fe); got != nil {
		t.Errorf("resolveCSSFontFamilyList(unknown stack) = %v, want nil", got)
	}
	if got := resolveCSSFontFamily(`"Comic Sans MS", cursive, fantasy`, fe); got != nil {
		t.Errorf("resolveCSSFontFamily(unknown stack) = %v, want nil", got)
	}
}

// TestStylesToStylesPopulatesFontFamilyStack verifies that the high-level
// CSS-cascade path populates BOTH ih.fontfamily (primary, for the existing
// single-family code path) and ih.fontfamilyStack (full prioritised list,
// for per-glyph fallback). Single-family inputs leave the stack as a
// one-entry slice so the `len > 1` gate in ApplySettings keeps the
// settings map free of the new SettingFontFamilyStack key — that gate is
// what keeps single-family content on the unchanged single-shape path.
func TestStylesToStylesPopulatesFontFamilyStack(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	sans := fe.FindFontFamily("sans")
	serif := fe.FindFontFamily("serif")
	mono := fe.FindFontFamily("monospace")

	ih := &FormattingStyles{Fontsize: 10 * 65536}
	if err := StylesToStyles(ih, map[string]string{"font-family": `monospace, sans-serif, serif`}, fe, ih.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	if ih.fontfamily != mono {
		t.Errorf("primary fontfamily = %v, want monospace", ih.fontfamily)
	}
	want := []*frontend.FontFamily{mono, sans, serif}
	if len(ih.fontfamilyStack) != len(want) {
		t.Fatalf("fontfamilyStack length = %d, want %d", len(ih.fontfamilyStack), len(want))
	}
	for i, ff := range want {
		if ih.fontfamilyStack[i] != ff {
			t.Errorf("fontfamilyStack[%d] = %v, want %v", i, ih.fontfamilyStack[i], ff)
		}
	}

	// Gate: single-family CSS must not emit the new setting so the
	// downstream shape orchestrator stays on the single-shape path.
	ih2 := &FormattingStyles{Fontsize: 10 * 65536}
	if err := StylesToStyles(ih2, map[string]string{"font-family": `sans-serif`}, fe, ih2.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	settings := frontend.TypesettingSettings{}
	ApplySettings(settings, ih2)
	if _, ok := settings[frontend.SettingFontFamilyStack]; ok {
		t.Errorf("ApplySettings emitted SettingFontFamilyStack for single-family CSS")
	}

	// Two-or-more families must emit the new setting so per-glyph
	// fallback can pick it up.
	settings2 := frontend.TypesettingSettings{}
	ApplySettings(settings2, ih)
	if _, ok := settings2[frontend.SettingFontFamilyStack]; !ok {
		t.Errorf("ApplySettings did NOT emit SettingFontFamilyStack for multi-family CSS")
	}
}
