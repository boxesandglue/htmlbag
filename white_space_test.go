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
// decorated their own text despite the UA stylesheet declaring them.
func TestTextDecorationPropagates(t *testing.T) {
	for _, tc := range []struct {
		html string
		want frontend.TextDecorationLine
	}{
		{"<u>text</u>", frontend.TextDecorationUnderline},
		{"<s>text</s>", frontend.TextDecorationLineThrough},
		{"<del>text</del>", frontend.TextDecorationLineThrough},
		{`<span style="text-decoration-line: overline">text</span>`, frontend.TextDecorationOverline},
		{"<u>a <b>b</b></u>", frontend.TextDecorationUnderline},
	} {
		te := renderToText(t, "<!DOCTYPE html><html><body><p>"+tc.html+"</p></body></html>")
		got, ok := findSetting(te, frontend.SettingTextDecorationLine)
		if !ok || got != tc.want {
			t.Errorf("%s set %v (found=%v), want %v", tc.html, got, ok, tc.want)
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
