package htmlbag

import (
	"bytes"
	"math"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestFontFaceSizeAdjustPropagation guards the @font-face handoff: csshtml
// parses size-adjust (stored as 1 - percentage/100), but the value only has
// an effect when AddFontFamiliesFromCSS copies it onto the FontSource, where
// frontend.shapeFontFor scales the font size at shape time. The weight-range
// form exercises the per-weight instance path (instanceAt) as well.
func TestFontFaceSizeAdjustPropagation(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	cp := csshtml.NewCSSParser()
	if err := cp.AddCSSText(`@font-face {
		font-family: "VF";
		font-weight: 200 900;
		size-adjust: 130%;
		src: url("vf.ttf");
	}`); err != nil {
		t.Fatal(err)
	}
	if err := AddFontFamiliesFromCSS(cp, fe); err != nil {
		t.Fatal(err)
	}
	ff := fe.FindFontFamily("VF")
	if ff == nil {
		t.Fatal("font family VF not registered")
	}
	fs, err := ff.GetFontSource(400, frontend.FontStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	// 130% → 1 - 1.3 = -0.3; shapeFontFor computes size * (1 - SizeAdjust).
	if want := 1 - 1.3; math.Abs(fs.SizeAdjust-want) > 1e-9 {
		t.Errorf("SizeAdjust = %v, want %v", fs.SizeAdjust, want)
	}
}
