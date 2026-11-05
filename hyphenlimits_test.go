package htmlbag

import (
	"slices"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/lang"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestParseHyphenLimits(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  *hyphenLimits
	}{
		{"auto", nil},
		{"auto auto auto", nil},
		{"6", &hyphenLimits{6, -1, -1}},
		{"6 3", &hyphenLimits{6, 3, 3}},
		{"6 3 2", &hyphenLimits{6, 3, 2}},
		{"auto 3", &hyphenLimits{-1, 3, 3}},
		{"6 auto 2", &hyphenLimits{6, -1, 2}},
		{"6 auto", &hyphenLimits{6, -1, -1}},
		{"0 0 0", &hyphenLimits{0, 0, 0}},
		{"1 2 3 4", nil},
		{"six", nil},
		{"-1", nil},
		{"", nil},
	} {
		got := parseHyphenLimits(tc.value)
		if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
			t.Errorf("parseHyphenLimits(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

// languageOf finds the language of the Text with the id in te.
func languageOf(t *testing.T, te *frontend.Text, id string) *lang.Lang {
	t.Helper()
	var found *lang.Lang
	var walk func(*frontend.Text)
	walk = func(n *frontend.Text) {
		if got, _ := n.Settings[frontend.SettingElementID].(string); got == id {
			found, _ = n.Settings[frontend.SettingLanguage].(*lang.Lang)
		}
		for _, itm := range n.Items {
			if c, ok := itm.(*frontend.Text); ok {
				walk(c)
			}
		}
	}
	walk(te)
	if found == nil {
		t.Fatalf("no language on element %q", id)
	}
	return found
}

func points(l *lang.Lang, word string) []int {
	var p []int
	at := 0
	for _, step := range l.Hyphenate(word) {
		at += step
		p = append(p, at)
	}
	return p
}

func TestHyphenateLimitChars(t *testing.T) {
	fe, te := runStylePass(t, `<!DOCTYPE html><html lang="en"><body>
<div style="hyphenate-limit-chars: 6 2 4">
<p id="inherited">x</p>
<p id="other" lang="de">x</p>
<p id="reset" style="hyphenate-limit-chars: auto">x</p>
<p id="long" style="hyphenate-limit-chars: 12">x</p>
</div>
<p id="plain">x</p>
<p id="same" style="hyphenate-limit-chars: 6 2 4">x</p>
</body></html>`)
	inherited := languageOf(t, te, "inherited")
	if got, want := points(inherited, "hyphenation"), []int{2, 6, 7}; !slices.Equal(got, want) {
		t.Errorf("inherited limits: %v, want %v", got, want)
	}
	if got := languageOf(t, te, "other"); got.Name != "de" || got.Lefthyphenmin != 2 || got.Wordhyphenmin != 6 {
		t.Errorf("limits of another language: %s, %d, %d", got.Name, got.Lefthyphenmin, got.Wordhyphenmin)
	}
	plain, err := fe.GetLanguageCached("en")
	if err != nil {
		t.Fatal(err)
	}
	if got := languageOf(t, te, "reset"); got != plain {
		t.Errorf("auto keeps the limits of the parent")
	}
	if got := languageOf(t, te, "plain"); got != plain {
		t.Errorf("an element without limits has others")
	}
	if got := points(languageOf(t, te, "long"), "hyphenation"); len(got) != 0 {
		t.Errorf("a word below the minimum has points: %v", got)
	}
	if got, want := points(plain, "hyphenation"), []int{6, 7}; !slices.Equal(got, want) {
		t.Errorf("limits reached the plain language: %v, want %v", got, want)
	}
	if languageOf(t, te, "same") != inherited {
		t.Errorf("two elements with the same limits have two languages")
	}
}

// Without a language tag, the limits apply to the document language, and a
// child goes back to it with auto.
func TestHyphenateLimitCharsWithoutLang(t *testing.T) {
	fe, te := runStylePass(t, `<!DOCTYPE html><html><body>
<div id="limited" style="hyphenate-limit-chars: 6 2 4"><p id="reset" style="hyphenate-limit-chars: auto">x</p></div>
</body></html>`)
	if got, want := points(languageOf(t, te, "limited"), "hyphenation"), []int{2, 6, 7}; !slices.Equal(got, want) {
		t.Errorf("limits on the document language: %v, want %v", got, want)
	}
	if languageOf(t, te, "reset") != fe.Doc.DefaultLanguage {
		t.Errorf("auto keeps the limits of the parent")
	}
}
