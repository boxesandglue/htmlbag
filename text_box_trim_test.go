package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// trimCSS sets 16pt lines on a 158pt content area, ten lines needing 160pt.
const trimCSS = `@page { size: 200pt 198pt; margin: 20pt; }
body { margin: 0 } p { margin: 0; font-family: serif; font-size: 10pt; line-height: 16pt }`

// lineSize is a placed line's box, height and depth together.
func lineSize(l placedLine) bag.ScaledPoint { return l.top - l.bottom }

// A trimmed block ending mid-page: its first line loses the space above the
// font's text-over edge and its last line the space below its text-under
// edge, so the block after it moves up by both. Not inherited.
func TestTextBoxTrimMidPage(t *testing.T) {
	full := sp("16pt")
	render := func(style string) []placedLine {
		return placedLines(renderHTMLPages(t, trimCSS, `<p style="`+style+`">A<br>x<br>x</p><p>B</p>`))
	}
	plain := render("")
	for _, c := range []struct {
		style      string
		start, end bool
	}{
		{"text-box-trim: none", false, false},
		{"text-box-trim: trim-start", true, false},
		{"text-box-trim: trim-end", false, true},
		{"text-box-trim: trim-both", true, true},
		{"text-box: trim-both text", true, true},
		{"text-box: text", true, true},
		{"text-box: normal", false, false},
		{"text-box-trim: trim-both; text-box-edge: auto", true, true},
	} {
		t.Run(c.style, func(t *testing.T) {
			lines := render(c.style)
			first, last := lines[0], lines[2]
			var shift bag.ScaledPoint
			if got := lineSize(first); c.start == (got == full) {
				t.Errorf("first line %s, start trimmed %t", got, c.start)
			}
			shift += full - lineSize(first)
			if got := lineSize(last); c.end == (got == full) {
				t.Errorf("last line %s, end trimmed %t", got, c.end)
			}
			shift += full - lineSize(last)
			if got := lineSize(lines[1]); got != full {
				t.Errorf("middle line %s, want %s", got, full)
			}
			b, b0 := firstLine(t, lines, "B"), firstLine(t, plain, "B")
			if got := b.top - b0.top; got != shift {
				t.Errorf("B moves up by %s, want %s", got, shift)
			}
			if got := lineSize(b); got != full {
				t.Errorf("B, without the property, is %s, want %s", got, full)
			}
		})
	}
}

// A trimmed block split across pages: only the first fragment's start and
// the last fragment's end are trimmed (box-decoration-break: slice).
func TestTextBoxTrimSplitAcrossPages(t *testing.T) {
	full := sp("16pt")
	lines := placedLines(renderHTMLPages(t, trimCSS, `<p style="text-box-trim: trim-both">`+charLines("A", 14)+`</p>`))
	var page1, page2 []placedLine
	for _, l := range lines {
		switch l.page {
		case 1:
			page1 = append(page1, l)
		case 2:
			page2 = append(page2, l)
		}
	}
	if len(page1) == 0 || len(page2) == 0 || len(page1)+len(page2) != 14 {
		t.Fatalf("%d and %d lines on pages 1 and 2, want 14 over both", len(page1), len(page2))
	}
	if got := lineSize(page1[0]); got >= full {
		t.Errorf("the first line is %s, want less than %s", got, full)
	}
	for _, l := range []placedLine{page1[len(page1)-1], page2[0]} {
		if got := lineSize(l); got != full {
			t.Errorf("a line at the break is %s, want %s", got, full)
		}
	}
	if got := lineSize(page2[len(page2)-1]); got >= full {
		t.Errorf("the last line is %s, want less than %s", got, full)
	}
}

// Lines set tighter than the font's content area have negative trims: the
// trimmed block ends at its text, outside its line boxes, so it grows.
func TestTextBoxTrimNegative(t *testing.T) {
	const css = `@page { size: 200pt 198pt; margin: 20pt; }
body { margin: 0 } p { margin: 0; font-family: serif; font-size: 10pt; line-height: 6pt }`
	render := func(style string) []placedLine {
		return placedLines(renderHTMLPages(t, css, `<p style="`+style+`">A<br>x</p><p>B</p>`))
	}
	plain, trimmed := render(""), render("text-box-trim: trim-both")
	var grow bag.ScaledPoint
	for i := range 2 {
		got, was := lineSize(trimmed[i]), lineSize(plain[i])
		if got <= was {
			t.Errorf("line %d is %s trimmed, want more than its %s", i+1, got, was)
		}
		grow += got - was
	}
	b, b0 := firstLine(t, trimmed, "B"), firstLine(t, plain, "B")
	if got := b0.top - b.top; got != grow {
		t.Errorf("B moves down by %s, want %s", got, grow)
	}
}
