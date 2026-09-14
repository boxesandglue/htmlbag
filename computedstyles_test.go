package htmlbag

import (
	"testing"
)

// resolveCSSText is the test-side shortcut through the cascade: parse a
// declaration block and expand its shorthands, exactly what ApplyCSS hands the
// renderer for one element.
func resolveCSSText(css string) StyleMap {
	return resolveDeclarations(declarationsFromText(css))
}

func TestBorderShorthand(t *testing.T) {
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
		// A function-valued color is one component, so it survives whole
		// instead of being reassembled from the words after it.
		{"dashed rgba(170, 50, 220, .6)", "1pt", "dashed", "rgba( 170 , 50 , 220 , .6 )"},
		{"rgba(170, 50, 220, .6)", "1pt", "none", "rgba( 170 , 50 , 220 , .6 )"},
	}
	for _, tC := range testCases {
		t.Run(tC.input, func(t *testing.T) {
			wd, sty, col := borderShorthand(tokenizeCSSString(tC.input))
			if wd.String() != tC.width || sty.String() != tC.style || col.String() != tC.color {
				t.Errorf(`borderShorthand(%s) got "%s %s %s" want "%s %s %s"`,
					tC.input, wd, sty, col, tC.width, tC.style, tC.color)
			}
		})
	}
}

// A color function keeps its arguments together through the shorthand, which
// the old text split could not do: it shattered the value on spaces and only
// a colorMatcher special case kept `background` working at all.
func TestShorthandKeepsFunctionValuesIntact(t *testing.T) {
	got := resolveCSSText("background: cmyk(0%, 100%, 100%, 0%)")
	if want := "cmyk( 0% , 100% , 100% , 0% )"; got.Get("background-color") != want {
		t.Errorf("background-color = %q, want %q", got.Get("background-color"), want)
	}
	got = resolveCSSText("border-color: cmyk(0%, 0%, 0%, 100%) red")
	if want := "cmyk( 0% , 0% , 0% , 100% )"; got.Get("border-top-color") != want {
		t.Errorf("border-top-color = %q, want %q", got.Get("border-top-color"), want)
	}
	if want := "red"; got.Get("border-right-color") != want {
		t.Errorf("border-right-color = %q, want %q", got.Get("border-right-color"), want)
	}
}

func TestResolveTextDecoration(t *testing.T) {
	testCases := []struct {
		css   string
		line  string
		style string
	}{
		{"text-decoration: underline", "underline", "solid"},
		{"text-decoration: underline dotted", "underline", "dotted"},
		{"text-decoration: dashed underline", "underline", "dashed"},
		{"text-decoration: line-through wavy", "line-through", "wavy"},
		{"text-decoration: overline double", "overline", "double"},
		{"text-decoration: none", "none", ""},
		{"text-decoration-line: underline; text-decoration-style: wavy", "underline", "wavy"},
		{"text-decoration-line: underline", "underline", "solid"},
	}
	for _, tc := range testCases {
		t.Run(tc.css, func(t *testing.T) {
			resolved := resolveCSSText(tc.css)
			if got := resolved.Get("text-decoration-line"); got != tc.line {
				t.Errorf("text-decoration-line = %q, want %q", got, tc.line)
			}
			if got := resolved.Get("text-decoration-style"); got != tc.style {
				t.Errorf("text-decoration-style = %q, want %q", got, tc.style)
			}
		})
	}
}

// Declaration order decides inside one block: the shorthand resets the
// longhand that came before it, and a longhand after it wins.
func TestShorthandOrderWithinOneBlock(t *testing.T) {
	got := resolveCSSText("border-left-style: dotted; border-left: thick green")
	if want := "none"; got.Get("border-left-style") != want {
		t.Errorf("shorthand after longhand: border-left-style = %q, want %q",
			got.Get("border-left-style"), want)
	}
	got = resolveCSSText("border-left: thick green; border-left-style: dotted")
	if want := "dotted"; got.Get("border-left-style") != want {
		t.Errorf("longhand after shorthand: border-left-style = %q, want %q",
			got.Get("border-left-style"), want)
	}
}

// The border shorthand is order-independent (CSS Backgrounds 3 §4.5). The old
// word scanner bailed out the moment it recognised a color, so `#fff solid`
// lost its style: everything after the color went unread.
func TestBorderShorthandIsOrderIndependent(t *testing.T) {
	for _, in := range []string{"#fff solid 2pt", "solid 2pt #fff", "2pt #fff solid"} {
		t.Run(in, func(t *testing.T) {
			wd, sty, col := borderShorthand(tokenizeCSSString(in))
			if wd.String() != "2pt" || sty.String() != "solid" || col.String() != "#fff" {
				t.Errorf(`borderShorthand(%s) got "%s %s %s", want "2pt solid #fff"`, in, wd, sty, col)
			}
		})
	}
}
