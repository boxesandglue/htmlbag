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
