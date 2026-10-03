package htmlbag

import "testing"

// A paragraph that cannot leave orphans lines on the page and widows lines
// on the next has no break inside it there: a three-line paragraph with the
// default widows and orphans of 2 and room for two lines moves on whole,
// on a page and in a region.
func TestShortParagraphMovesOnWhenWidowsAndOrphansClash(t *testing.T) {
	// 11 lines leave 28pt of the 160pt, room for two 12pt lines.
	body := `<p>` + charLines("A", 11) + `</p><p>` + charLines("B", 3) + `</p>`
	t.Run("page", func(t *testing.T) {
		l := firstLine(t, placedLines(renderHTMLPages(t, charCSS, body)), "B")
		if l.page != 2 || l.top != charTop {
			t.Errorf("the paragraph starts on page %d at %s, want page 2 at %s", l.page, l.top, charTop)
		}
	})
	t.Run("region", func(t *testing.T) {
		cb, _ := newFlowBuilder(t, "")
		tr := flow(t, cb, body, wide("160pt"))
		if len(tr.filled) != 2 {
			t.Fatalf("filled %d regions, want 2", len(tr.filled))
		}
		if lines := boxLines(tr.filled[1]); len(lines) != 3 || lines[0].text != "B" {
			t.Errorf("second region holds %+v, want the paragraph's three lines", lines)
		}
	})
	t.Run("widows 1", func(t *testing.T) {
		l := firstLine(t, placedLines(renderHTMLPages(t, charCSS, `<p>`+charLines("A", 11)+`</p><p style="widows: 1">`+charLines("B", 3)+`</p>`)), "B")
		if l.page != 1 {
			t.Errorf("with widows: 1 the paragraph starts on page %d, want 1", l.page)
		}
	})
}

// A heading kept with a paragraph that is moved on whole moves on with it:
// the paragraph offers the break before the heading no foothold.
func TestShortParagraphTakesKeptHeadingAlong(t *testing.T) {
	// 10 lines leave 40pt, room for the heading and two lines.
	body := `<p>` + charLines("A", 10) + `</p><h1 style="page-break-after: avoid">H</h1><p>` + charLines("B", 3) + `</p>`
	lines := placedLines(renderHTMLPages(t, charCSS, body))
	h, b := firstLine(t, lines, "H"), firstLine(t, lines, "B")
	if h.page != 2 || h.top != charTop || b.page != 2 {
		t.Errorf("heading on page %d at %s, paragraph on page %d; want both on page 2, the heading at %s", h.page, h.top, b.page, charTop)
	}
}

// Widows and orphans count lines, so a box or a list whose children are
// blocks still breaks between them, and a paragraph in a box splits by its
// own lines.
func TestBlockChildrenSplitDespiteWidows(t *testing.T) {
	const css = charCSS + `
div { border: 1pt solid black; }
ul, li { margin: 0; padding: 0; font-size: 10pt; line-height: 12pt; }`
	cases := []struct {
		name, body, kept string
		onSecond         int // lines on page 2
	}{
		// D has room for three of its four lines and keeps two for widows.
		{"box", `<p>` + charLines("A", 2) + `</p><div><p>` + charLines("B", 4) + `</p><p>` + charLines("C", 4) + `</p><p>` + charLines("D", 4) + `</p></div>`, "D", 2},
		{"list", `<p>` + charLines("A", 11) + `</p><ul><li>B</li><li>C</li><li>D</li></ul>`, "C", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := boxedLines(renderHTMLPages(t, css, c.body))
			if k := firstLine(t, lines, c.kept); k.page != 1 {
				t.Errorf("%s starts on page %d, want 1", c.kept, k.page)
			}
			n := 0
			for _, l := range lines {
				if l.page == 2 {
					n++
				}
			}
			if n != c.onSecond {
				t.Errorf("%d lines on page 2, want %d", n, c.onSecond)
			}
		})
	}
}
