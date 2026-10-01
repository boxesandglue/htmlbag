package htmlbag

import "testing"

// With orphans: 1 a paragraph may leave a single line at the foot, but only
// one that fits there: when not even its first line fits below the block
// before it, the whole paragraph starts on the next page or region.
func TestSplitFirstLineThatDoesNotFitMovesOn(t *testing.T) {
	// 13 lines leave 4pt of the 160pt, less than a 12pt line.
	body := `<p>` + charLines("A", 13) + `</p><p style="orphans: 1; widows: 1">` + charLines("B", 5) + `</p>`
	t.Run("page", func(t *testing.T) {
		l := firstLine(t, placedLines(renderHTMLPages(t, charCSS, body)), "B")
		if l.page != 2 || l.top != charTop {
			t.Errorf("first line of the paragraph on page %d at %s, want page 2 at %s", l.page, l.top, charTop)
		}
	})
	t.Run("region", func(t *testing.T) {
		cb, _ := newFlowBuilder(t, "")
		tr := flow(t, cb, body, wide("160pt"))
		if len(tr.filled) != 2 {
			t.Fatalf("filled %d regions, want 2", len(tr.filled))
		}
		if used := tr.filled[0].Used; used > sp("160pt") {
			t.Errorf("first region used %s, past its 160pt", used)
		}
		lines := boxLines(tr.filled[1])
		if len(lines) == 0 || lines[0].text != "B" || lines[0].top != 0 {
			t.Errorf("second region starts with %+v, want the line B at its top", lines)
		}
	})
}
