package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

// lastSetting returns the value of key on the deepest Text that sets it, in
// document order, and whether any Text does.
func lastSetting(te *frontend.Text, key frontend.SettingType) (any, bool) {
	v, ok := te.Settings[key]
	for _, itm := range te.Items {
		if c, isText := itm.(*frontend.Text); isText {
			if cv, cok := lastSetting(c, key); cok {
				v, ok = cv, true
			}
		}
	}
	return v, ok
}

// -bag-linebreak-tolerance and -bag-linebreak-hyphen-penalty of 0 are values:
// they reach the paragraph, also over a value an ancestor sets (#104).
func TestLinebreakSettingsZeroInCSS(t *testing.T) {
	for _, tc := range []struct {
		name, css string
		tolerance any
		penalty   any
	}{
		{"zero", `p { -bag-linebreak-tolerance: 0; -bag-linebreak-hyphen-penalty: 0 }`, 0.0, 0},
		{"zero over an ancestor", `body { -bag-linebreak-tolerance: 8; -bag-linebreak-hyphen-penalty: 200 }
			p { -bag-linebreak-tolerance: 0; -bag-linebreak-hyphen-penalty: 0 }`, 0.0, 0},
		{"inherited", `body { -bag-linebreak-tolerance: 8; -bag-linebreak-hyphen-penalty: 200 }`, 8.0, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, te := hscaleText(t, tc.css, "text")
			tol, okT := lastSetting(te, frontend.SettingLinebreakTolerance)
			pen, okP := lastSetting(te, frontend.SettingHyphenPenalty)
			if !okT || tol != tc.tolerance || !okP || pen != tc.penalty {
				t.Errorf("tolerance %v (set %t), hyphen penalty %v (set %t); want %v, %v", tol, okT, pen, okP, tc.tolerance, tc.penalty)
			}
		})
	}
	t.Run("unset", func(t *testing.T) {
		_, te := hscaleText(t, ``, "text")
		if _, ok := lastSetting(te, frontend.SettingLinebreakTolerance); ok {
			t.Error("tolerance set without CSS")
		}
		if _, ok := lastSetting(te, frontend.SettingHyphenPenalty); ok {
			t.Error("hyphen penalty set without CSS")
		}
	})
}
