package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// Regression test for orphaned table headers: outputTableRows placed the
// header rows like ordinary rows, so when the header still fit into the
// space left on the page but the first data row did not, the page ended
// with a lone header and the data started on the next page under a repeated
// header. The header block must instead move to the next page together with
// the first data row.
//
// The filler count that lands the table exactly in that window depends on
// font metrics, so the test sweeps a range of counts wide enough to contain
// the window wherever it drifts: somewhere in the range the table start
// crosses from "fits after the fillers" to "starts on the next page", and
// the orphan window sits on that transition.
func TestTableHeaderNotOrphaned(t *testing.T) {
	css := `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 11pt }
th, td { border-bottom: 0.5pt solid black; padding: 2pt }`
	cellText := strings.Repeat("lorem ipsum dolor sit amet consetetur sadipscing elitr sed diam nonumy ", 5)
	rowMarkers := []string{"ROWONE", "ROWTWO", "ROWTHREE"}

	for fillers := 27; fillers <= 40; fillers++ {
		t.Run(fmt.Sprintf("%dfillers", fillers), func(t *testing.T) {
			var sb strings.Builder
			for i := range fillers {
				fmt.Fprintf(&sb, "<p>FILLER%03d</p>", i)
			}
			sb.WriteString("<table><thead><tr><th>HEADCELL</th></tr></thead><tbody>")
			for _, m := range rowMarkers {
				fmt.Fprintf(&sb, "<tr><td>%s %s</td></tr>", m, cellText)
			}
			sb.WriteString("</tbody></table>")

			pages := renderHTMLPages(t, css, sb.String())
			var all strings.Builder
			for pi, pg := range pages {
				txt := pageText(pg)
				all.WriteString(txt)
				if !strings.Contains(txt, "HEADCELL") {
					continue
				}
				hasRow := false
				for _, m := range rowMarkers {
					if strings.Contains(txt, m) {
						hasRow = true
						break
					}
				}
				if !hasRow {
					t.Errorf("page %d carries the table header but no data row", pi+1)
				}
			}
			for _, m := range rowMarkers {
				if !strings.Contains(all.String(), m) {
					t.Errorf("%s missing from the output", m)
				}
			}
		})
	}
}
