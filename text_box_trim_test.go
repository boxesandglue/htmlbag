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

// A split block whose rest is re-broken at the wider page 2 (the reflow in
// outputBlockSplit, which takes only a block with a border or background)
// keeps its last line trimmed, as it is without the reflow.
func TestTextBoxTrimSplitReflow(t *testing.T) {
	html := `<p style="text-box-trim: trim-both; background-color: #eee">` + charLines("A", 14) + `</p>`
	last := func(css string) placedLine {
		lines := placedLines(renderHTMLPages(t, css, html))
		first, l := lines[0], lines[len(lines)-1]
		if l.page != 2 {
			t.Fatalf("the last line is on page %d, want 2", l.page)
		}
		if l.width != sp("160pt") {
			t.Fatalf("the last line is %s wide, want 160pt (the first is %s)", l.width, first.width)
		}
		return l
	}
	want := lineSize(last(trimCSS))
	if want >= sp("16pt") {
		t.Fatalf("the last line without a reflow is %s, want it trimmed", want)
	}
	if got := lineSize(last(trimCSS + `@page :first { margin-left: 60pt }`)); got != want {
		t.Errorf("the last line after a reflow is %s, want %s", got, want)
	}
}

// text-box-trim on a block container trims its first and last formatted
// line, which sit in its first and last in-flow child, through containers
// in between. Padding or a border on the child's side ends the trim's reach,
// as does a table; the container's own padding does not, and a float is
// passed over (CSS Inline 3, boxesandglue/htmlbag#82).
func TestTextBoxTrimContainer(t *testing.T) {
	full := sp("16pt")
	render := func(html string) []placedLine {
		return placedLines(renderHTMLPages(t, trimCSS, html))
	}
	// The sizes a paragraph's own trims give the first and the last line.
	trimmedFirst := lineSize(render(`<p style="text-box-trim: trim-start">A<br>x</p>`)[0])
	trimmedLast := lineSize(render(`<p style="text-box-trim: trim-end">y<br>z</p>`)[1])
	if trimmedFirst >= full || trimmedLast >= full {
		t.Fatalf("a paragraph's trims give %s and %s, want less than %s", trimmedFirst, trimmedLast, full)
	}
	const pair = `<p>A<br>x</p><p>y<br>z</p>`
	for _, c := range []struct {
		name, html string
		start, end bool
	}{
		{"none", `<div>` + pair + `</div>`, false, false},
		{"trim-both", `<div style="text-box-trim: trim-both">` + pair + `</div>`, true, true},
		{"trim-start", `<div style="text-box-trim: trim-start">` + pair + `</div>`, true, false},
		{"trim-end", `<div style="text-box-trim: trim-end">` + pair + `</div>`, false, true},
		{"through a container", `<div style="text-box-trim: trim-both"><div><p>A<br>x</p></div><div><div><p>y<br>z</p></div></div></div>`, true, true},
		{"padding-top on the first child", `<div style="text-box-trim: trim-both"><p style="padding-top: 2pt">A<br>x</p><p>y<br>z</p></div>`, false, true},
		{"merged with the child's own", `<div style="text-box-trim: trim-start"><p>A<br>x</p><p style="text-box-trim: trim-end">y<br>z</p></div>`, true, true},
		{"after a float", `<div style="text-box-trim: trim-both"><div style="float: right; width: 30pt">Fq</div>` + pair + `</div>`, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			lines := render(c.html + `<p>B</p>`)
			var a, z placedLine
			for _, l := range lines {
				switch l.text {
				case "A":
					a = l
				case "z":
					z = l
				}
			}
			want := map[bool]bag.ScaledPoint{false: full, true: trimmedFirst}
			if got := lineSize(a); got != want[c.start] {
				t.Errorf("first line %s, want %s (trimmed %t)", got, want[c.start], c.start)
			}
			want[true] = trimmedLast
			if got := lineSize(z); got != want[c.end] {
				t.Errorf("last line %s, want %s (trimmed %t)", got, want[c.end], c.end)
			}
			for _, l := range lines {
				if (l.text == "x" || l.text == "y" || l.text == "B") && lineSize(l) != full {
					t.Errorf("line %s is %s, want %s", l.text, lineSize(l), full)
				}
			}
		})
	}
}

// A box with a border is one placed unit here, so its trims are measured on
// its size: the container's own padding and border do not stop the trim,
// a border on the child holding the last line does.
func TestTextBoxTrimContainerBorders(t *testing.T) {
	full := sp("16pt")
	render := func(html string) []placedLine {
		return placedLines(renderHTMLPages(t, trimCSS, html+`<p>B</p>`))
	}
	trimmedFirst := lineSize(render(`<p style="text-box-trim: trim-start">A<br>x</p>`)[0])
	trimmedLast := lineSize(render(`<p style="text-box-trim: trim-end">y<br>z</p>`)[1])
	const pair = `<p>A<br>x</p><p>y<br>z</p>`
	t.Run("own padding and border", func(t *testing.T) {
		box := `; padding: 4pt; border: 1pt solid black">` + pair + `</div>`
		plain := firstLine(t, render(`<div style="`+box), "Axyz")
		trimmed := firstLine(t, render(`<div style="text-box-trim: trim-both`+box), "Axyz")
		if got, want := lineSize(trimmed), lineSize(plain)-(full-trimmedFirst)-(full-trimmedLast); got != want {
			t.Errorf("the box is %s, want %s", got, want)
		}
	})
	t.Run("border-bottom on the last child", func(t *testing.T) {
		lines := render(`<div style="text-box-trim: trim-both"><p>A<br>x</p><div style="border-bottom: 1pt solid black"><p>y<br>z</p></div></div>`)
		if got := lineSize(firstLine(t, lines, "A")); got != trimmedFirst {
			t.Errorf("first line %s, want %s", got, trimmedFirst)
		}
		if got, want := lineSize(firstLine(t, lines, "yz")), 2*full; got != want {
			t.Errorf("the bordered child is %s, want %s untrimmed", got, want)
		}
	})
}

// A trimmed container moves the block after it up by its trims: they are
// taken off its box, not only off the lines.
func TestTextBoxTrimContainerHeight(t *testing.T) {
	render := func(style string) []placedLine {
		return placedLines(renderHTMLPages(t, trimCSS, `<div style="`+style+`"><p>A<br>x</p><p>y<br>z</p></div><p>B</p>`))
	}
	plain, trimmed := render(""), render("text-box-trim: trim-both")
	var shift bag.ScaledPoint
	for i := range 4 {
		shift += lineSize(plain[i]) - lineSize(trimmed[i])
	}
	if shift <= 0 {
		t.Fatalf("the trims take %s off the lines, want more than 0", shift)
	}
	b, b0 := firstLine(t, trimmed, "B"), firstLine(t, plain, "B")
	if got := b.top - b0.top; got != shift {
		t.Errorf("B moves up by %s, want %s", got, shift)
	}
}

// A table at the container's start holds no formatted line of it: the trim
// does not reach past it to the paragraph below, nor into its cells.
func TestTextBoxTrimContainerTableFirst(t *testing.T) {
	render := func(style string) []placedLine {
		return placedLines(renderHTMLPages(t, trimCSS, `<div style="`+style+`"><table><tr><td><p>T</p></td></tr></table><p>A<br>x</p></div>`))
	}
	plain, trimmed := render(""), render("text-box-trim: trim-start")
	if len(plain) != len(trimmed) {
		t.Fatalf("%d lines trimmed, %d without", len(trimmed), len(plain))
	}
	for i := range plain {
		if got, want := lineSize(trimmed[i]), lineSize(plain[i]); got != want {
			t.Errorf("line %s is %s, want %s as without the trim", trimmed[i].text, got, want)
		}
	}
}
