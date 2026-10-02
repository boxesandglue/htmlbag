package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// tall is cell content well over 16pt: three lines of 11pt text.
const tall = `one<br>two<br>three`

func TestTableRowFixedHeight(t *testing.T) {
	css := cellWidthCSS + `
tr.fixed { -bag-fixed-height: 16pt }
tr.off { -bag-fixed-height: none }
tbody.fixed { -bag-fixed-height: 16pt }`
	html := `<table>
<tr style="-bag-fixed-height: 16pt"><td>` + tall + `</td></tr>
<tr style="height: 16pt"><td>` + tall + `</td></tr>
<tr style="height: 3cm; -bag-fixed-height: 16pt"><td>` + tall + `</td></tr>
<tr class="fixed off"><td>` + tall + `</td></tr>
<tr><td style="-bag-fixed-height: 16pt">` + tall + `</td></tr>
<tr style="-bag-fixed-height: 16pt"><td style="height: 3cm">` + tall + `</td></tr>
<tr><td>` + tall + `</td></tr>
<tr style="-bag-fixed-height: 50%"><td>` + tall + `</td></tr>
<tr style="-bag-fixed-height: 0"><td>` + tall + `</td></tr>
</table>
<table><tbody class="fixed"><tr><td>` + tall + `</td></tr></tbody></table>`
	buf, restore := captureLog()
	heights := tableRowHeights(t, renderHTMLPages(t, css, html))
	restore()
	if len(heights) != 10 {
		t.Fatalf("got %d rows, want 10", len(heights))
	}
	if n := strings.Count(buf.String(), "-bag-fixed-height needs a positive length"); n != 2 {
		t.Errorf("got %d warnings, want 2 (50%% and 0): %s", n, buf.String())
	}
	sixteen := bag.MustSP("16pt")
	natural := heights[6]
	if natural <= sixteen {
		t.Fatalf("natural row height %s is not taller than 16pt", natural)
	}
	for _, tc := range []struct {
		name string
		row  int
		want bag.ScaledPoint
	}{
		{"fixed row with overflowing content", 0, sixteen},
		{"height alone grows", 1, natural},
		{"fixed wins over height", 2, sixteen},
		{"none in a later rule", 3, natural},
		{"on a cell", 4, natural},
		{"cell height in a fixed row", 5, sixteen},
		{"a percentage is ignored", 7, natural},
		{"zero is ignored", 8, natural},
		{"not inherited from tbody", 9, natural},
	} {
		if heights[tc.row] != tc.want {
			t.Errorf("%s: row is %s, want %s", tc.name, heights[tc.row], tc.want)
		}
	}
}
