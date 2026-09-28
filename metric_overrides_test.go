package htmlbag

import (
	"bytes"
	"math"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

// The ascent-override, descent-override and line-gap-override descriptors
// reach the FontSource as fractions of the em; normal, or a value that is not
// a non-negative percentage, leaves the face's own (-1).
func TestFontFaceMetricOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, descriptors string
		want              *frontend.MetricsOverride
	}{
		{"none", ``, nil},
		{"all three", `ascent-override: 107.91%; descent-override: 25.1%; line-gap-override: 0%;`, &frontend.MetricsOverride{Ascent: 1.0791, Descent: 0.251, LineGap: 0}},
		{"one", `ascent-override: 90%;`, &frontend.MetricsOverride{Ascent: 0.9, Descent: -1, LineGap: -1}},
		{"normal", `ascent-override: normal; descent-override: 20%;`, &frontend.MetricsOverride{Ascent: -1, Descent: 0.2, LineGap: -1}},
		{"invalid", `ascent-override: -5%; descent-override: 12pt; line-gap-override: 10%;`, &frontend.MetricsOverride{Ascent: -1, Descent: -1, LineGap: 0.1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe, err := frontend.NewForWriter(&bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			cp := NewCSSParser()
			if err := cp.AddCSSText(`@font-face { font-family: "M"; src: url("m.ttf"); ` + tc.descriptors + ` }`); err != nil {
				t.Fatal(err)
			}
			if err := AddFontFamiliesFromCSS(cp, fe); err != nil {
				t.Fatal(err)
			}
			fs, err := fe.FindFontFamily("M").GetFontSource(400, frontend.FontStyleNormal)
			if err != nil {
				t.Fatal(err)
			}
			got := fs.Metrics
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("Metrics = %v, want %v", got, tc.want)
			}
			if got == nil {
				return
			}
			near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
			if !near(got.Ascent, tc.want.Ascent) || !near(got.Descent, tc.want.Descent) || !near(got.LineGap, tc.want.LineGap) {
				t.Errorf("Metrics = %+v, want %+v", *got, *tc.want)
			}
		})
	}
}
