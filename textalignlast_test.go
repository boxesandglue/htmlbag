package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// lineSide says where the text of a line sits between the edges of its box.
func lineSide(t *testing.T, line *node.HList) string {
	t.Helper()
	var x, left, right bag.ScaledPoint
	seen := false
	for e := line.List; e != nil; e = e.Next() {
		switch v := e.(type) {
		case *node.Glyph:
			if !seen {
				left, seen = x, true
			}
			x += v.Width
			right = x
		case *node.Glue:
			x += v.Width
		case *node.Kern:
			x += v.Kern
		}
	}
	if !seen {
		t.Fatal("a line without text")
	}
	// Stretching rounds every glue down to a scaled point.
	near := bag.MustSP("0.01pt")
	gapLeft, gapRight := left, line.Width-right
	switch {
	case gapLeft <= near && gapRight <= near:
		return "full"
	case gapLeft <= near:
		return "left"
	case gapRight <= near:
		return "right"
	case max(gapLeft-gapRight, gapRight-gapLeft) <= near:
		return "center"
	}
	return "between"
}

func lineSides(t *testing.T, css, body string) []string {
	t.Helper()
	var sides []string
	for _, line := range lineModelLines(t, `p{font-family:serif;font-size:10pt}`+css, body, nil) {
		sides = append(sides, lineSide(t, line))
	}
	return sides
}

const lastLineText = `<p>aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm</p>`

func TestTextAlignLast(t *testing.T) {
	const just = `p{text-align:justify}`
	for _, tc := range []struct {
		name, css, last string
		first           string
	}{
		{"auto ends at the start", just, "left", "full"},
		{"left", just + `p{text-align-last:left}`, "left", "full"},
		{"right", just + `p{text-align-last:right}`, "right", "full"},
		{"center", just + `p{text-align-last:center}`, "center", "full"},
		{"justify", just + `p{text-align-last:justify}`, "full", "full"},
		{"start", just + `p{text-align-last:start}`, "left", "full"},
		{"end", just + `p{text-align-last:end}`, "right", "full"},
		{"a left paragraph", `p{text-align:left}p{text-align-last:right}`, "right", "left"},
		{"a left paragraph, justified", `p{text-align:left}p{text-align-last:justify}`, "full", "left"},
		{"a centered paragraph", `p{text-align:center}p{text-align-last:left}`, "left", "center"},
		{"a right paragraph", `p{text-align:right}p{text-align-last:center}`, "center", "right"},
		{"auto follows a right paragraph", `p{text-align:right}`, "right", "right"},
		{"justify-all", `p{text-align:justify-all}`, "full", "full"},
		{"inherited", `body{text-align:justify;text-align-last:center}`, "center", "full"},
		{"text-align on a child keeps the inherited value", just + `body{text-align-last:center}`, "center", "full"},
		{"replaced", just + `body{text-align-last:center}p{text-align-last:right}`, "right", "full"},
		{"auto replaces", just + `body{text-align-last:center}p{text-align-last:auto}`, "left", "full"},
		{"inherit", just + `body{text-align-last:center}p{text-align-last:inherit}`, "center", "full"},
		{"an invalid value changes nothing", just + `body{text-align-last:center}p{text-align-last:middle}`, "center", "full"},
		{"text-align after it", `p{text-align-last:center;text-align:justify}`, "center", "full"},
		{"text-align before it", `p{text-align:justify;text-align-last:center}`, "center", "full"},
		{"justify-all after it", `p{text-align-last:center;text-align:justify-all}`, "full", "full"},
		{"justify-all before it", `p{text-align:justify-all;text-align-last:center}`, "center", "full"},
		{"justify-all loses the cascade", `p{text-align:justify-all;text-align:right}`, "right", "right"},
		{"text-align below justify-all", `body{text-align:justify-all}p{text-align:right}`, "full", "right"},
		{"rtl, start", just + `p{direction:rtl;text-align-last:start}`, "right", "full"},
		{"rtl, end", just + `p{direction:rtl;text-align-last:end}`, "left", "full"},
		{"rtl, left stays left", just + `p{direction:rtl;text-align-last:left}`, "left", "full"},
		{"rtl, auto", just + `p{direction:rtl}`, "right", "full"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sides := lineSides(t, tc.css, lastLineText)
			if len(sides) < 3 {
				t.Fatalf("%d lines, want several", len(sides))
			}
			n := len(sides) - 1
			if sides[n] != tc.last {
				t.Errorf("last line: %s, want %s", sides[n], tc.last)
			}
			for i, side := range sides[:n] {
				if side != tc.first {
					t.Errorf("line %d: %s, want %s", i, side, tc.first)
				}
			}
		})
	}
}

// A line before a forced break counts as a last line.
func TestTextAlignLastForcedBreak(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want []string
	}{
		{`p{text-align:justify}`, []string{"left", "left"}},
		{`p{text-align:justify;text-align-last:center}`, []string{"center", "center"}},
		{`p{text-align:justify;text-align-last:right}`, []string{"right", "right"}},
		{`p{text-align:justify;text-align-last:justify}`, []string{"full", "full"}},
		{`p{text-align:left;text-align-last:justify}`, []string{"full", "full"}},
	} {
		got := lineSides(t, tc.css, `<p>aa bb<br>cc dd</p>`)
		if len(got) != 2 || got[0] != tc.want[0] || got[1] != tc.want[1] {
			t.Errorf("%s: %v, want %v", tc.css, got, tc.want)
		}
	}
}

// A line without a space to stretch stays where it starts.
func TestTextAlignLastJustifyOneWord(t *testing.T) {
	for css, want := range map[string]string{
		`p{text-align-last:justify}`:               "left",
		`p{text-align-last:justify;direction:rtl}`: "right",
	} {
		if got := lineSides(t, css, `<p>aa</p>`); len(got) != 1 || got[0] != want {
			t.Errorf("%s: %v, want %s", css, got, want)
		}
	}
}

// The value only aligns the last line, the breaks stay where they were: a
// short last line does not make the paragraph look for other breaks.
func TestTextAlignLastKeepsTheBreaks(t *testing.T) {
	glyphs := func(css string) []int {
		var counts []int
		for _, line := range lineModelLines(t, `p{font-family:serif;font-size:10pt;text-align:justify}`+css, lastLineText, nil) {
			n := 0
			for e := line.List; e != nil; e = e.Next() {
				if _, ok := e.(*node.Glyph); ok {
					n++
				}
			}
			counts = append(counts, n)
		}
		return counts
	}
	want := glyphs(``)
	for _, css := range []string{`p{text-align-last:justify}`, `p{text-align-last:center}`, `p{text-align-last:right}`} {
		got := glyphs(css)
		if len(got) != len(want) {
			t.Fatalf("%s: %v, want %v", css, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: %v, want %v", css, got, want)
				break
			}
		}
	}
}
