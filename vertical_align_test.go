package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// applyVerticalAlign runs one declaration block through the style resolver
// against a fresh 14pt parent style and returns the resulting styles. The
// parent font size (14pt) is passed as curFontSize, mimicking what
// collectHorizontalNodes does for an inline element.
func applyVerticalAlign(t *testing.T, decls map[string]string) *FormattingStyles {
	t.Helper()
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	parentSize := bag.MustSP("14pt")
	ih := &FormattingStyles{DefaultFontSize: bag.MustSP("10pt"), Fontsize: parentSize}
	if err := StylesToStyles(ih, decls, fe, parentSize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	return ih
}

// near allows for the float32 rounding in ParseRelativeSize's em parsing.
func near(a, b bag.ScaledPoint) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 100 // 100sp ≈ 0.0015pt
}

// glu#2: sub/super must shift by a fraction of the *parent's* font size.
// Script markup almost always shrinks font-size too, and the shift must not
// shrink with it (14pt × 58% × 1/5 was the barely-visible 1.6pt of the bug
// report).
func TestVerticalAlignScriptShiftFromParentSize(t *testing.T) {
	parent := bag.MustSP("14pt")

	sup := applyVerticalAlign(t, map[string]string{
		"font-size":      "58%",
		"vertical-align": "super",
	})
	if want := parent / 3; sup.yoffset != want {
		t.Errorf("super: yoffset = %s, want %s (parent em / 3)", sup.yoffset, want)
	}

	sub := applyVerticalAlign(t, map[string]string{
		"font-size":      "58%",
		"vertical-align": "sub",
	})
	if want := -parent / 5; sub.yoffset != want {
		t.Errorf("sub: yoffset = %s, want %s (-parent em / 5)", sub.yoffset, want)
	}
}

// glu#2: length and percentage values used to be dropped silently. em
// resolves against the element's own font size, % against its line-height.
func TestVerticalAlignLengthAndPercentage(t *testing.T) {
	em := applyVerticalAlign(t, map[string]string{
		"font-size":      "58%",
		"vertical-align": "0.4em",
	})
	// own size = 14pt × 58% = 8.12pt; shift = 0.4 × 8.12pt
	if want := bag.MultiplyFloat(bag.MustSP("8.12pt"), 0.4); !near(em.yoffset, want) {
		t.Errorf("0.4em: yoffset = %s, want %s", em.yoffset, want)
	}

	pt := applyVerticalAlign(t, map[string]string{
		"vertical-align": "-2pt",
	})
	if want := bag.MustSP("-2pt"); pt.yoffset != want {
		t.Errorf("-2pt: yoffset = %s, want %s", pt.yoffset, want)
	}

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	pct := &FormattingStyles{
		DefaultFontSize:  bag.MustSP("10pt"),
		Fontsize:         bag.MustSP("10pt"),
		lineheightFactor: 1.2,
	}
	if err := StylesToStyles(pct, map[string]string{"vertical-align": "50%"}, fe, pct.Fontsize); err != nil {
		t.Fatalf("StylesToStyles: %v", err)
	}
	// 50% of line-height = 0.5 × 1.2 × 10pt
	if want := bag.MustSP("6pt"); !near(pct.yoffset, want) {
		t.Errorf("50%%: yoffset = %s, want %s", pct.yoffset, want)
	}
}

// Keywords that are not handled must stay no-ops instead of reaching the
// numeric parser ("inherit" arrives here via the UA default stylesheet's
// td,th,tr rule).
func TestVerticalAlignUnknownKeywordIsNoop(t *testing.T) {
	for _, kw := range []string{"inherit", "baseline", "revert"} {
		ih := applyVerticalAlign(t, map[string]string{"vertical-align": kw})
		if ih.yoffset != 0 {
			t.Errorf("%s: yoffset = %s, want 0", kw, ih.yoffset)
		}
	}
}

// The baseline shift inherits (children align to the parent's shifted
// baseline) and nested scripts add their own shift computed from their own
// parent's size.
func TestVerticalAlignNestedScriptsStack(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	body := &FormattingStyles{DefaultFontSize: bag.MustSP("10pt"), Fontsize: bag.MustSP("14pt")}

	outer := body.Clone()
	if err := StylesToStyles(outer, map[string]string{
		"font-size":      "58%",
		"vertical-align": "super",
	}, fe, body.Fontsize); err != nil {
		t.Fatalf("StylesToStyles (outer): %v", err)
	}

	// A plain inline child (e.g. <i> inside <sup>) keeps the shift.
	child := outer.Clone()
	if child.yoffset != outer.yoffset {
		t.Errorf("inherited: yoffset = %s, want %s", child.yoffset, outer.yoffset)
	}

	// <sup> inside <sup>: the inner shift is relative to the outer one.
	inner := outer.Clone()
	if err := StylesToStyles(inner, map[string]string{
		"vertical-align": "super",
	}, fe, outer.Fontsize); err != nil {
		t.Fatalf("StylesToStyles (inner): %v", err)
	}
	if want := body.Fontsize/3 + outer.Fontsize/3; inner.yoffset != want {
		t.Errorf("nested: yoffset = %s, want %s", inner.yoffset, want)
	}
}
