package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// chainPages renders a spacer that leaves room for `room` 20pt lines at the
// foot of a 10-line page, then body, and returns the page each marker (H01…,
// L01…) landed on and the page count.
func chainPages(t *testing.T, css string, room int, body string) (map[string]int, int) {
	t.Helper()
	base := fmt.Sprintf(`@page { size: 200pt 200pt; margin: 0 }
body { margin: 0; font-family: serif; font-size: 10pt; line-height: 20pt }
p, div { margin: 0 }
h2 { font-size: 10pt; line-height: 20pt; margin: 0; break-after: avoid }
.spacer { height: %dpt }
`, 200-20*room)
	pages := renderHTMLPages(t, base+css, `<div class="spacer"></div>`+body)
	got := map[string]int{}
	for pi, pg := range pages {
		txt := pageText(pg)
		for i := 1; i <= 20; i++ {
			for _, prefix := range []string{"H", "L"} {
				if m := fmt.Sprintf("%s%02d", prefix, i); strings.Contains(txt, m) {
					got[m] = pi + 1
				}
			}
		}
	}
	return got, len(pages)
}

func chainBody(headings, lines int) string {
	var sb strings.Builder
	for i := 1; i <= headings; i++ {
		fmt.Fprintf(&sb, "<h2>H%02d</h2>", i)
	}
	var ls []string
	for i := 1; i <= lines; i++ {
		ls = append(ls, fmt.Sprintf("L%02d", i))
	}
	sb.WriteString("<p>" + strings.Join(ls, "<br>") + "</p>")
	return sb.String()
}

// Two headings in a row with margins between them: the first used to stay
// behind, since the lookahead from it saw only the margin and the second.
func TestBreakAfterAvoidChainMovesTogether(t *testing.T) {
	css := `h2 { margin-bottom: 20pt }`
	// Room for 5 lines; the chain with its margins and the paragraph's
	// two-line foothold needs 6.
	got, _ := chainPages(t, css, 5, chainBody(2, 6))
	for _, m := range []string{"H01", "H02", "L01"} {
		if got[m] != 2 {
			t.Errorf("%s on page %d, want 2 (%v)", m, got[m], got)
		}
	}
	// With room for 6 the chain stays and takes its foothold along.
	got, _ = chainPages(t, css, 6, chainBody(2, 6))
	for _, m := range []string{"H01", "H02", "L01", "L02"} {
		if got[m] != 1 {
			t.Errorf("room 6: %s on page %d, want 1 (%v)", m, got[m], got)
		}
	}
}

func TestBreakAfterAvoidChainOfThree(t *testing.T) {
	got, _ := chainPages(t, `h2 { margin-bottom: 10pt }`, 5, chainBody(3, 6))
	for _, m := range []string{"H01", "H02", "H03", "L01"} {
		if got[m] != 2 {
			t.Errorf("%s on page %d, want 2 (%v)", m, got[m], got)
		}
	}
}

// A heading directly followed by a long paragraph keeps its foothold: the
// lookahead used to weigh the whole paragraph when no margin sat between.
func TestBreakAfterAvoidFootholdWithoutMargin(t *testing.T) {
	got, _ := chainPages(t, "", 3, chainBody(1, 6))
	for _, m := range []string{"H01", "L01", "L02"} {
		if got[m] != 1 {
			t.Errorf("%s on page %d, want 1 (%v)", m, got[m], got)
		}
	}
}

// A chain taller than an empty page leaves a page that has content, and is
// then placed anyway, breaking where it has to.
func TestBreakAfterAvoidChainTallerThanPage(t *testing.T) {
	got, n := chainPages(t, "", 3, chainBody(12, 2))
	if got["H01"] != 2 {
		t.Errorf("H01 on page %d, want 2 (%v)", got["H01"], got)
	}
	if got["H10"] != 2 || got["H11"] != 3 || got["L02"] != 3 || n != 3 {
		t.Errorf("chain not placed from page 2 on: %v, %d pages", got, n)
	}
}
