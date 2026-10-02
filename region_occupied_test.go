package htmlbag

import (
	"errors"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

func occupied(r Region) Region { r.Occupied = true; return r }

// cappedRegions are testRegions that fail after max regions, so a flow that
// would never end fails instead.
type cappedRegions struct {
	testRegions
	max int
}

func (cr *cappedRegions) Next(brk string) (Region, error) {
	if len(cr.brks) == cr.max {
		return Region{}, errors.New("the flow does not end")
	}
	return cr.testRegions.Next(brk)
}

// flowCapped pours body into regions of the given sizes, the last one as
// often as needed, and fails after 50 regions.
func flowCapped(t *testing.T, cb *CSSBuilder, body string, sizes ...Region) *testRegions {
	t.Helper()
	te, err := cb.HTMLToText(`<html><body>` + body + `</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	cr := &cappedRegions{testRegions: testRegions{sizes: sizes}, max: 50}
	if err := cb.FlowText(te, cr); err != nil {
		t.Fatalf("FlowText: %v", err)
	}
	return &cr.testRegions
}

// landing returns the region, counted from 1, and the top of the first line
// that contains mark.
func landing(t *testing.T, tr *testRegions, mark string) (int, bag.ScaledPoint) {
	t.Helper()
	for i, f := range tr.filled {
		for _, l := range boxLines(f) {
			if strings.Contains(l.text, mark) {
				return i + 1, l.top
			}
		}
	}
	t.Fatalf("no region holds %q", mark)
	return 0, 0
}

// Each check that moves a block on from a region that holds something moves
// it on from an occupied region too. In a region that is not occupied, the
// block stays where it is.
func TestFlowTextOccupiedRegionMovesBlocksOn(t *testing.T) {
	var rows strings.Builder
	for i := 0; i < 3; i++ {
		rows.WriteString(`<tr><td>Rq</td></tr>`)
	}
	// A header row too tall for the region by itself.
	table := `<table><thead><tr><th style="line-height: 30pt">Hq</th></tr></thead><tbody>` + rows.String() + `</tbody></table>`
	cases := []struct {
		name, body string
		sizes      []Region
		// mark is the line that moves on, at the region it is in when that
		// region is not occupied.
		mark string
		at   int
	}{
		{"a block too tall", `<p style="line-height: 30pt">Aq</p>`,
			[]Region{wide("20pt"), wide("1000pt")}, "Aq", 1},
		{"a float and the block beside it", `<div style="float: left; width: 40pt">Fq</div><p style="line-height: 30pt">Aq</p>`,
			[]Region{wide("20pt"), wide("1000pt")}, "Fq", 1},
		{"a break-after: avoid chain", `<h1 style="break-after: avoid">Hq</h1><p style="line-height: 30pt">Aq</p>`,
			[]Region{wide("20pt"), wide("1000pt")}, "Hq", 1},
		{"orphans", `<p>` + charLines("A", 6) + `</p><p>y</p>`,
			[]Region{wide("20pt"), wide("1000pt")}, "A", 1},
		{"a first line that does not fit", `<p style="line-height: 30pt; orphans: 1; widows: 1">` + charLines("A", 3) + `</p><p>y</p>`,
			[]Region{wide("20pt"), wide("1000pt")}, "A", 1},
		{"widows", `<p style="widows: 3">` + charLines("A", 3) + `</p><p>y</p>`,
			[]Region{wide("30pt"), wide("1000pt")}, "A", 1},
		{"table headers", table,
			[]Region{wide("20pt"), wide("1000pt")}, "Hq", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, occ := range []bool{false, true} {
				sizes := append([]Region{}, c.sizes...)
				want := c.at
				if occ {
					sizes[c.at-1] = occupied(sizes[c.at-1])
					want++
				}
				cb, _ := newFlowBuilder(t, "")
				tr := flowCapped(t, cb, c.body, sizes...)
				if got, _ := landing(t, tr, c.mark); got != want {
					t.Errorf("occupied %v: %q is in region %d, want %d", occ, c.mark, got, want)
				}
			}
		})
	}
}

// A block moves on from an occupied region once. In the region after that
// move it is placed as in an empty one, occupied or not, so a flow through
// regions that are all occupied ends.
func TestFlowTextOccupiedRegionsEnd(t *testing.T) {
	cases := []struct {
		name, body string
		height     string
		lines      int // lines of A in the region it lands in
	}{
		{"a block too tall", `<p style="line-height: 30pt">Aq</p><p>y</p>`, "20pt", 1},
		{"orphans", `<p style="orphans: 2">` + charLines("A", 6) + `</p><p>y</p>`, "20pt", 1},
		{"a bordered div", `<div style="border: 1pt solid black">` + charLines("A", 6) + `</div><p>y</p>`, "20pt", 1},
		{"widows", `<p style="widows: 3">` + charLines("A", 3) + `</p><p>y</p>`, "30pt", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flowCapped(t, cb, c.body, occupied(wide(c.height)))
			region, _ := landing(t, tr, "A")
			if region != 2 {
				t.Fatalf("A is in region %d, want 2", region)
			}
			if lines := boxLines(tr.filled[1]); len(lines) != c.lines {
				t.Errorf("region 2 holds %d lines, want %d", len(lines), c.lines)
			}
			landing(t, tr, "y")
		})
	}
}

// A paragraph that is the flow's only block keeps its orphans in an occupied
// region and moves on whole, as it does among other blocks.
func TestFlowTextOccupiedRegionSoleParagraph(t *testing.T) {
	for name, body := range map[string]string{
		"paragraph":        `<p>` + charLines("A", 6) + `</p>`,
		"paragraph in div": `<div><p>` + charLines("A", 6) + `</p></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flowCapped(t, cb, body, occupied(wide("20pt")), wide("1000pt"))
			if len(tr.filled) != 2 {
				t.Fatalf("filled %d regions, want 2", len(tr.filled))
			}
			if lines := boxLines(tr.filled[0]); len(lines) != 0 {
				t.Errorf("region 1 holds %d lines, want none", len(lines))
			}
			if lines := boxLines(tr.filled[1]); len(lines) != 6 {
				t.Errorf("region 2 holds %d lines, want 6", len(lines))
			}
		})
	}
}

// The top of an occupied region is not the edge of the fragmentainer, so the
// margin-top of a block that moves there is kept, whether it went along or was
// spent at the foot of the region before, and collapses with MarginBefore, as
// in the first region.
func TestFlowTextOccupiedRegionKeepsMargin(t *testing.T) {
	withBefore := func(r Region, m string) Region { r.MarginBefore = sp(m); return r }
	b := func(margin string) string {
		return `<p style="margin-top: ` + margin + `">` + charLines("B", 2) + `</p><p>y</p>`
	}
	cases := []struct {
		name  string
		body  string
		sizes []Region
		top   string
	}{
		{"margin goes along", `<p>Zq</p>` + b("10pt"),
			[]Region{wide("20pt"), wide("1000pt")}, "0pt"},
		{"margin goes along, occupied", `<p>Zq</p>` + b("10pt"),
			[]Region{wide("20pt"), occupied(wide("1000pt"))}, "10pt"},
		{"margin goes along, smaller MarginBefore", `<p>Zq</p>` + b("10pt"),
			[]Region{wide("20pt"), withBefore(occupied(wide("1000pt")), "4pt")}, "10pt"},
		{"margin goes along, larger MarginBefore", `<p>Zq</p>` + b("10pt"),
			[]Region{wide("20pt"), withBefore(occupied(wide("1000pt")), "15pt")}, "15pt"},
		{"margin spent at the foot", `<p>Zq</p>` + b("5pt"),
			[]Region{wide("20pt"), wide("1000pt")}, "0pt"},
		{"margin spent at the foot, occupied", `<p>Zq</p>` + b("5pt"),
			[]Region{wide("20pt"), occupied(wide("1000pt"))}, "5pt"},
		{"first block, occupied", b("10pt"),
			[]Region{occupied(wide("20pt")), occupied(wide("1000pt"))}, "10pt"},
		{"first block, every region occupied", b("10pt"),
			[]Region{occupied(wide("20pt"))}, "10pt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flowCapped(t, cb, c.body, c.sizes...)
			region, top := landing(t, tr, "B")
			if region != 2 || top != sp(c.top) {
				t.Errorf("B starts in region %d at %s, want region 2 at %s", region, top, c.top)
			}
		})
	}
}
