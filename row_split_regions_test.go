package htmlbag

import (
	"strings"
	"testing"
)

// splitRowLines counts the lines of the split row a region holds: each of
// its lines is one glyph, A or x, and the header row is Hq.
func splitRowLines(f Filled) int {
	n := 0
	for _, l := range boxLines(f) {
		if !strings.Contains(l.text, "Hq") {
			n += len(l.text)
		}
	}
	return n
}

// The rest of a row with break-inside: auto, split at the end of one region,
// is split again by the measure of the region it goes on in, not of the one
// before (#66).
func TestFlowTextSplitRowRestTakesTheRegionsMeasure(t *testing.T) {
	body := `<table><thead><tr><th>Hq</th></tr></thead><tbody><tr style="break-inside: auto"><td>` + charLines("A", 8) + `</td></tr></tbody></table>`
	cases := []struct {
		name    string
		regions []Region
		filled  int
	}{
		{"taller region", []Region{wide("60pt"), wide("1000pt")}, 2},
		{"shorter region", []Region{wide("60pt"), wide("30pt"), wide("1000pt")}, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "td, th { padding: 0 }")
			tr := flowCapped(t, cb, body, c.regions...)
			if len(tr.filled) != c.filled {
				t.Errorf("filled %d regions, want %d", len(tr.filled), c.filled)
			}
			total := 0
			for i, f := range tr.filled {
				h := c.regions[min(i, len(c.regions)-1)].Height
				if f.Used > h {
					t.Errorf("region %d: Used %s, more than its height %s", i+1, f.Used, h)
				}
				total += splitRowLines(f)
			}
			if total != 8 {
				t.Errorf("the regions hold %d lines of the row, want 8", total)
			}
		})
	}
}
