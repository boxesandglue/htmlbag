package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestWhiteSpacePreservesNewlines covers CSS Text 3 §3 at the point where the
// text is extracted from the markup, which is where the collapsing happens.
// `pre-line` is the interesting one: the spaces fold but the newline has to
// survive, because it is a forced break. Previously only `pre` was recognised
// and the collapsing pass rewrote every newline as a space, so a hard break in
// prose was destroyed before the typesetter ever saw it.
func TestWhiteSpacePreservesNewlines(t *testing.T) {
	const src = "a  b\nc"
	for _, tc := range []struct {
		whiteSpace string
		want       string
	}{
		{"normal", "a b c"},
		{"nowrap", "a b c"},
		{"pre", "a  b\nc"},
		{"pre-line", "a b\nc"},
	} {
		te := renderToText(t, `<!DOCTYPE html><html><body><p style="white-space: `+tc.whiteSpace+`">`+src+`</p></body></html>`)
		got := strings.Join(collectStrings(te.Items), "")
		if got != tc.want {
			t.Errorf("white-space: %s produced %q, want %q", tc.whiteSpace, got, tc.want)
		}
	}
}

// TestWhiteSpaceSetting checks the mode reaches the typesetter, since the
// width of a space and whether a line may break at one are decided there.
func TestWhiteSpaceSetting(t *testing.T) {
	for _, tc := range []struct {
		whiteSpace string
		want       frontend.WhiteSpace
	}{
		{"normal", frontend.WhiteSpaceNormal},
		{"nowrap", frontend.WhiteSpaceNowrap},
		{"pre", frontend.WhiteSpacePre},
		{"pre-wrap", frontend.WhiteSpacePreWrap},
		{"pre-line", frontend.WhiteSpacePreLine},
	} {
		te := renderToText(t, `<!DOCTYPE html><html><body><p style="white-space: `+tc.whiteSpace+`">x</p></body></html>`)
		got, ok := findSetting(te, frontend.SettingWhiteSpace)
		if !ok {
			t.Errorf("white-space: %s set no SettingWhiteSpace", tc.whiteSpace)
			continue
		}
		if got != tc.want {
			t.Errorf("white-space: %s set %v, want %v", tc.whiteSpace, got, tc.want)
		}
	}
}

// TestTextDecorationPropagates covers CSS Text Decoration 3 §2.2: a decoration
// applies to in-flow descendants. The property was absent from the style
// inheritance, so it reset to none on every child and <u>, <s> and <del> never
// decorated their own text despite the UA stylesheet declaring them. A
// descendant's own decoration is drawn in addition to its ancestors', and
// one value can hold several lines (boxesandglue#96).
func TestTextDecorationPropagates(t *testing.T) {
	const (
		u  = frontend.TextDecorationUnderline
		o  = frontend.TextDecorationOverline
		lt = frontend.TextDecorationLineThrough
	)
	for _, tc := range []struct {
		html string
		want map[string]frontend.TextDecorationLine
	}{
		{"<u>text</u>", map[string]frontend.TextDecorationLine{"text": u}},
		{"<s>text</s>", map[string]frontend.TextDecorationLine{"text": lt}},
		{"<del>text</del>", map[string]frontend.TextDecorationLine{"text": lt}},
		{`<span style="text-decoration-line: overline">text</span>`, map[string]frontend.TextDecorationLine{"text": o}},
		{"<u>a <b>b</b></u>", map[string]frontend.TextDecorationLine{"a": u, "b": u}},
		{`<span style="text-decoration: underline line-through">text</span>`, map[string]frontend.TextDecorationLine{"text": u | lt}},
		{`<span style="text-decoration-line: underline overline">text</span>`, map[string]frontend.TextDecorationLine{"text": u | o}},
		{`<span style="text-decoration: line-through"><u>text</u></span>`, map[string]frontend.TextDecorationLine{"text": u | lt}},
		{`<u><span style="text-decoration: line-through">text</span></u>`, map[string]frontend.TextDecorationLine{"text": u | lt}},
		{"<s>a <u>b</u> c</s>", map[string]frontend.TextDecorationLine{"a": lt, "b": u | lt, "c": lt}},
		// `none` cannot take an ancestor's line away.
		{`<u>a <span style="text-decoration: none">b</span></u>`, map[string]frontend.TextDecorationLine{"a": u, "b": u}},
	} {
		te := renderToText(t, "<!DOCTYPE html><html><body><p>"+tc.html+"</p></body></html>")
		got := map[string]frontend.TextDecorationLine{}
		decorationsByRun(te, nil, got)
		for run, want := range tc.want {
			if got[run] != want {
				t.Errorf("%s: run %q has lines %b, want %b", tc.html, run, got[run], want)
			}
		}
	}
}

// decorationsByRun records the lines each text run in te is decorated with.
// inherited are the decorations of the parent text, which a text without its
// own SettingTextDecorations takes over.
func decorationsByRun(te *frontend.Text, inherited []frontend.TextDecoration, got map[string]frontend.TextDecorationLine) {
	if v, ok := te.Settings[frontend.SettingTextDecorations]; ok {
		inherited, _ = v.([]frontend.TextDecoration)
	}
	for _, itm := range te.Items {
		switch c := itm.(type) {
		case string:
			var lines frontend.TextDecorationLine
			for _, d := range inherited {
				lines |= d.Line
			}
			if run := strings.TrimSpace(c); run != "" {
				got[run] |= lines
			}
		case *frontend.Text:
			decorationsByRun(c, inherited, got)
		}
	}
}

// findSetting looks for the first non-zero value of key anywhere in the tree.
func findSetting(te *frontend.Text, key frontend.SettingType) (any, bool) {
	if v, ok := te.Settings[key]; ok && v != nil {
		switch t := v.(type) {
		case frontend.WhiteSpace:
			if t != frontend.WhiteSpaceNormal {
				return v, true
			}
		case frontend.TextDecorationLine:
			if t != frontend.TextDecorationLineNone {
				return v, true
			}
		default:
			return v, true
		}
	}
	for _, itm := range te.Items {
		if child, ok := itm.(*frontend.Text); ok {
			if v, found := findSetting(child, key); found {
				return v, true
			}
		}
	}
	if v, ok := te.Settings[key]; ok {
		return v, true
	}
	return nil, false
}
