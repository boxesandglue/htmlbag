package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// findSettings returns the settings of the first text in te, depth first,
// that holds a string containing want.
func findSettings(te *frontend.Text, want string) frontend.TypesettingSettings {
	for _, it := range te.Items {
		switch v := it.(type) {
		case string:
			if v == want {
				return te.Settings
			}
		case *frontend.Text:
			if s := findSettings(v, want); s != nil {
				return s
			}
		}
	}
	return nil
}

// word-spacing becomes SettingWordSpacing, -bag-word-spacing becomes
// SettingWordSpacingLimits, and both are inherited.
func TestWordSpacingProperties(t *testing.T) {
	te := renderToText(t, `<!DOCTYPE html><html><body>
<p style="word-spacing: 2pt; -bag-word-spacing: 80% 100% 133%">outer <span>inner</span></p>
<p style="word-spacing: 50%; -bag-word-spacing: 120% 100% 133%">bad</p>
</body></html>`)
	for _, word := range []string{"inner"} {
		s := findSettings(te, word)
		if s == nil {
			t.Fatalf("no text %q", word)
		}
		if got := s[frontend.SettingWordSpacing]; got != bag.MustSP("2pt") {
			t.Errorf("%s: word spacing %v, want 2pt", word, got)
		}
		if got := s[frontend.SettingWordSpacingLimits]; got != (frontend.WordSpacing{Min: 0.8, Opt: 1, Max: 1.33}) {
			t.Errorf("%s: limits %v, want 0.8 1 1.33", word, got)
		}
	}
	s := findSettings(te, "bad")
	if s == nil {
		t.Fatal("no text bad")
	}
	if _, ok := s[frontend.SettingWordSpacing]; ok {
		t.Errorf("a percentage set word spacing %v", s[frontend.SettingWordSpacing])
	}
	if _, ok := s[frontend.SettingWordSpacingLimits]; ok {
		t.Errorf("decreasing limits were set: %v", s[frontend.SettingWordSpacingLimits])
	}
}
