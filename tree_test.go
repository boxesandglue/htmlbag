package htmlbag

import (
	"testing"

	"golang.org/x/net/html"
)

func TestParseBorder(t *testing.T) {
	testCases := []struct {
		input string
		width string
		style string
		color string
	}{
		{"solid black", "1pt", "solid", "black"},
		{"1pt solid black", "1pt", "solid", "black"},
		{"solid black 1pt", "1pt", "solid", "black"},
		{"solid 00pt black", "00pt", "solid", "black"},
		{"solid 1pt black", "1pt", "solid", "black"},
		{"black solid 1pt", "1pt", "solid", "black"},
		{"1pt green", "1pt", "none", "green"},
		{"green", "1pt", "none", "green"},
		{"0.5rem outset pink", "0.5rem", "outset", "pink"},
		{"thin outset pink", "0.5pt", "outset", "pink"},
		{"medium", "1pt", "none", "currentcolor"},
		{"thick dashed", "2pt", "dashed", "currentcolor"},
		{"dotted", "1pt", "dotted", "currentcolor"},
		{"dashed", "1pt", "dashed", "currentcolor"},
		{"dashed rgba(170, 50, 220, .6)", "1pt", "dashed", "rgba(170, 50, 220, .6)"},
		{"rgba(170, 50, 220, .6)", "1pt", "none", "rgba(170, 50, 220, .6)"},
	}
	for _, tC := range testCases {
		t.Run(tC.input, func(t *testing.T) {
			wd, sty, col := parseBorderAttribute(tC.input)
			if wd != tC.width || sty != tC.style || col != tC.color {
				t.Errorf(`parseBorderAttribute(%s) got "%s %s %s" want "%s %s %s"`, tC.input, wd, sty, col, tC.width, tC.style, tC.color)
			}
		})
	}
}

func TestResolveTextDecoration(t *testing.T) {
	testCases := []struct {
		attrs []html.Attribute
		line  string
		style string
	}{
		{[]html.Attribute{{Key: "!text-decoration", Val: "underline"}}, "underline", "solid"},
		{[]html.Attribute{{Key: "!text-decoration", Val: "underline dotted"}}, "underline", "dotted"},
		{[]html.Attribute{{Key: "!text-decoration", Val: "dashed underline"}}, "underline", "dashed"},
		{[]html.Attribute{{Key: "!text-decoration", Val: "line-through wavy"}}, "line-through", "wavy"},
		{[]html.Attribute{{Key: "!text-decoration", Val: "overline double"}}, "overline", "double"},
		{[]html.Attribute{{Key: "!text-decoration", Val: "none"}}, "none", ""},
		{
			[]html.Attribute{
				{Key: "!text-decoration-line", Val: "underline"},
				{Key: "!text-decoration-style", Val: "wavy"},
			},
			"underline", "wavy",
		},
		{
			[]html.Attribute{{Key: "!text-decoration-line", Val: "underline"}},
			"underline", "solid",
		},
	}
	for _, tc := range testCases {
		resolved, _ := ResolveAttributes(tc.attrs)
		if got := resolved["text-decoration-line"]; got != tc.line {
			t.Errorf("%v: text-decoration-line = %q, want %q", tc.attrs, got, tc.line)
		}
		if got := resolved["text-decoration-style"]; got != tc.style {
			t.Errorf("%v: text-decoration-style = %q, want %q", tc.attrs, got, tc.style)
		}
	}
}
