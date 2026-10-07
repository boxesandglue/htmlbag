package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

const trimAtBreak = "-bag-text-box-trim-at-break: trim-end"

// fragTexts is the text of the lines of each fragment.
func fragTexts(frags [][]placedLine) [][]string {
	out := make([][]string, len(frags))
	for i, f := range frags {
		for _, l := range f {
			out[i] = append(out[i], l.text)
		}
	}
	return out
}

// A block with -bag-text-box-trim-at-break that ends mid-page keeps its
// height: nothing moves.
func TestTrimAtBreakMidPage(t *testing.T) {
	eachTarget(t, func(t *testing.T, regions bool) {
		body := `<p style="%s">` + charLines("A", 3) + `</p><p>B</p>`
		plain := trimFragments(t, trimCSS, fmt.Sprintf(body, ""), regions)
		got := trimFragments(t, trimCSS, fmt.Sprintf(body, trimAtBreak), regions)
		if fmt.Sprint(got) != fmt.Sprint(plain) {
			t.Errorf("lines %v, want them as without the property, %v", got, plain)
		}
	})
}

// Split at an unforced break, the last line before it fits by its text and
// is trimmed at its end; the block's first line, the first line after the
// break and the block's last line keep their size.
func TestTrimAtBreakSplit(t *testing.T) {
	full, room := sp("16pt"), sp("158pt")
	for _, c := range []struct {
		style   string
		lines   int
		trimmed bool
	}{
		{trimAtBreak, 10, true},
		{trimAtBreak + "; background-color: #eee", 10, true},
		{"-bag-text-box-trim-at-break: none", 9, false},
		{"", 9, false},
	} {
		t.Run(c.style, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				frags := trimFragments(t, trimCSS, `<p style="`+c.style+`">`+charLines("A", 14)+`</p>`, regions)
				if len(frags) != 2 || len(frags[0]) != c.lines {
					t.Fatalf("%d fragments, %d lines on the first, want 2 and %d", len(frags), len(frags[0]), c.lines)
				}
				one, two := frags[0], frags[1]
				last := one[len(one)-1]
				if got := downSize(last); c.trimmed == (got == full) {
					t.Errorf("the last line before the break is %s, trimmed %t", got, c.trimmed)
				}
				if last.bottom > room {
					t.Errorf("the last line before the break ends %s down, past the %s room", last.bottom, room)
				}
				for _, l := range []placedLine{one[0], two[0], two[len(two)-1]} {
					if got := downSize(l); got != full {
						t.Errorf("line %q is %s, want %s", l.text, got, full)
					}
				}
			})
		})
	}
}

// A whole block whose last line fits only by its text stays where it is,
// with that line trimmed, and the block after it starts the next page. A
// one-line block, which does not split, as well as one that would.
func TestTrimAtBreakWholeBlock(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       string
	}{
		{"one line", `<p>` + charLines("A", 9) + `</p><p style="%s">B</p><p>C</p>`, "[[A x x x x x x x x B] [C]]"},
		{"four lines", `<p>` + charLines("A", 6) + `</p><p style="%s">` + charLines("B", 4) + `</p><p>C</p>`, "[[A x x x x x B x x x] [C]]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				frags := trimFragments(t, trimCSS, fmt.Sprintf(c.body, trimAtBreak), regions)
				if got := fmt.Sprint(fragTexts(frags)); got != c.want {
					t.Fatalf("lines %s, want %s", got, c.want)
				}
				if last := frags[0][len(frags[0])-1]; last.bottom > sp("158pt") || downSize(last) >= sp("16pt") {
					t.Errorf("the last line on the first page is %s and ends %s down, want it trimmed inside 158pt", downSize(last), last.bottom)
				}
				plain := trimFragments(t, trimCSS, fmt.Sprintf(c.body, ""), regions)
				if fmt.Sprint(fragTexts(plain)) == c.want {
					t.Errorf("without the property the lines are the same, %v", fragTexts(plain))
				}
			})
		})
	}
}

// A paragraph with the property cut through inside a container that splits
// (planSplit and cutBlock): after a 16pt line, nine lines fit by the last
// one's text.
func TestTrimAtBreakNested(t *testing.T) {
	eachTarget(t, func(t *testing.T, regions bool) {
		frags := trimFragments(t, trimCSS, `<p>Intro</p><div><p style="`+trimAtBreak+`">`+charLines("A", 20)+`</p></div>`, regions)
		if len(frags) < 2 || len(frags[0]) != 10 {
			t.Fatalf("%d lines on the first of %d fragments, want 10", len(frags[0]), len(frags))
		}
		if got := downSize(frags[1][0]); got != sp("16pt") {
			t.Errorf("the first line after the break is %s, want 16pt", got)
		}
	})
}

// A negative trim (lines set tighter than the font's content area) gains no
// room and grows nothing: the lines are as without the property.
func TestTrimAtBreakNegative(t *testing.T) {
	const css = `@page { size: 200pt 190.1pt; margin: 20pt; }
body { margin: 0 } p { margin: 0; font-family: serif; font-size: 10pt; line-height: 6pt }`
	eachTarget(t, func(t *testing.T, regions bool) {
		body := `<p style="%s">` + charLines("A", 30) + `</p>`
		plain := trimFragmentsIn(t, css, fmt.Sprintf(body, ""), regions, "150.1pt")
		got := trimFragmentsIn(t, css, fmt.Sprintf(body, trimAtBreak), regions, "150.1pt")
		if fmt.Sprint(got) != fmt.Sprint(plain) {
			t.Errorf("lines %v, want them as without the property, %v", got, plain)
		}
	})
}

// Under box-decoration-break: clone with text-box-trim: trim-end every
// fragment is trimmed already, and the property changes nothing: the lines
// are not trimmed twice.
func TestTrimAtBreakUnderClone(t *testing.T) {
	const clone = "text-box-trim: trim-end; box-decoration-break: clone"
	eachTarget(t, func(t *testing.T, regions bool) {
		for _, body := range []string{
			`<p style="%s">` + charLines("A", 14) + `</p>`,
			`<p>` + charLines("A", 6) + `</p><p style="%s">` + charLines("B", 4) + `</p><p>C</p>`,
		} {
			want := trimFragments(t, trimCSS, fmt.Sprintf(body, clone), regions)
			got := trimFragments(t, trimCSS, fmt.Sprintf(body, clone+"; "+trimAtBreak), regions)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("lines %v, want them as without the property, %v", got, want)
			}
		}
	})
}

// A block whose last line fits by its text but that break-after: avoid
// moves on with the block after it is trimmed only where it stays: on the
// next page it stands mid-page, untrimmed, as without the property. Without
// break-after: avoid it stays, trimmed.
func TestTrimAtBreakMovedOn(t *testing.T) {
	for _, c := range []struct {
		name, body string
	}{
		{"one line", `<p>` + charLines("A", 9) + `</p><p style="%s">B</p><p>` + charLines("C", 3) + `</p>`},
		{"three lines", `<p>` + charLines("A", 7) + `</p><p style="%s">` + charLines("B", 3) + `</p><p>` + charLines("C", 3) + `</p>`},
		{"float after", `<p>` + charLines("A", 9) + `</p><p style="%s">B</p><div style="float: left; width: 40pt; height: 20pt"></div><p>` + charLines("C", 3) + `</p>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			eachTarget(t, func(t *testing.T, regions bool) {
				const avoid = "break-after: avoid"
				want := trimFragments(t, trimCSS, fmt.Sprintf(c.body, avoid), regions)
				got := trimFragments(t, trimCSS, fmt.Sprintf(c.body, trimAtBreak+"; "+avoid), regions)
				if len(got) < 2 || got[1][0].text != "B" {
					t.Fatalf("lines %v, want B to start the second fragment", fragTexts(got))
				}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("lines %v, want them as without the property, %v", got, want)
				}
				stays := trimFragments(t, trimCSS, fmt.Sprintf(c.body, trimAtBreak), regions)
				if last := stays[0][len(stays[0])-1]; !strings.HasPrefix(last.text, "B") && last.text != "x" || downSize(last) >= sp("16pt") {
					t.Errorf("without break-after: avoid the first fragment ends %q, %s, want B's last line trimmed", last.text, downSize(last))
				}
			})
		})
	}
}
