package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

// deepestSetting returns the value of key on the innermost Text that carries
// it. findSetting stops at the outermost, which for an inherited boolean is
// the value the ancestors were given rather than the one in force on the run.
func deepestSetting(te *frontend.Text, key frontend.SettingType) (any, bool) {
	val, found := te.Settings[key]
	ok := found
	for _, itm := range te.Items {
		if child, isText := itm.(*frontend.Text); isText {
			if v, f := deepestSetting(child, key); f {
				val, ok = v, true
			}
		}
	}
	return val, ok
}

// TestLeadingModel checks -bag-leading-model reaches the typesetter. bag has
// two line models — the leading all after the line (its default, and TeX's) or
// split above and below it (CSS 2.1 §10.8.1) — and SettingHalfLeading chooses
// between them. csshtml's UA stylesheet declares the property on body, so
// without a reader here nothing ever selected the CSS model.
func TestLeadingModel(t *testing.T) {
	for _, tc := range []struct {
		decl string
		want bool
	}{
		{"-bag-leading-model: half", true},
		{"-bag-leading-model: trailing", false},
	} {
		te := renderToText(t, `<!DOCTYPE html><html><body style="`+tc.decl+`"><p>x</p></body></html>`)
		got, ok := deepestSetting(te, frontend.SettingHalfLeading)
		if !ok {
			t.Errorf("%s set no SettingHalfLeading", tc.decl)
			continue
		}
		if got != tc.want {
			t.Errorf("%s set %v, want %v", tc.decl, got, tc.want)
		}
	}
}

// The property inherits, and a descendant can opt back out.
func TestLeadingModelInherits(t *testing.T) {
	te := renderToText(t, `<!DOCTYPE html><html><body style="-bag-leading-model: half"><div><p>x</p></div></body></html>`)
	if got, ok := deepestSetting(te, frontend.SettingHalfLeading); !ok || got != true {
		t.Errorf("inherited value = %v (found %v), want true", got, ok)
	}

	te = renderToText(t, `<!DOCTYPE html><html><body style="-bag-leading-model: half"><p style="-bag-leading-model: trailing">x</p></body></html>`)
	if got, ok := deepestSetting(te, frontend.SettingHalfLeading); !ok || got != false {
		t.Errorf("overridden value = %v (found %v), want false", got, ok)
	}
}
