package htmlbag

import (
	"strings"
	"testing"
)

// break-inside: avoid keeps a block that is a child of the body in one piece,
// a paragraph as well as a container, when it fits on an empty page. A page
// is 13 lines; the filler takes 8 of them (#75).
func TestBreakInsideAvoidAtTheTopLevel(t *testing.T) {
	filler := `<p>` + nestLines("F", 8) + `</p>`
	for _, c := range []struct{ name, html string }{
		{"paragraph", `<p style="break-inside: avoid">` + nestLines("L", 6) + `</p>`},
		{"bordered container", `<div class="b" style="break-inside: avoid">` + nestParas("L", 6) + `</div>`},
		{"paragraph in a div", `<div><p style="break-inside: avoid">` + nestLines("L", 6) + `</p></div>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, on := linePages(t, filler+c.html, "L")
			if len(on) != 6 || countOn(on, 2) != 6 {
				t.Errorf("lines on pages %v, want all 6 on page 2", on)
			}
		})
	}
}

// A block with break-inside: avoid that is taller than a page still splits,
// and a forced break inside it is taken.
func TestBreakInsideAvoidGivesWay(t *testing.T) {
	t.Run("taller than a page", func(t *testing.T) {
		pages, on := linePages(t, `<p>Before</p><p style="break-inside: avoid">`+nestLines("L", 20)+`</p>`, "L")
		if len(on) != 20 || pages != 2 || countOn(on, 1) == 0 {
			t.Errorf("%d pages, lines on pages %v; want 2 pages with lines on page 1", pages, on)
		}
	})
	t.Run("forced break inside", func(t *testing.T) {
		_, on := linePages(t, `<div class="b" style="break-inside: avoid"><p>L1</p><p style="break-before: page">L2</p></div>`, "L")
		if len(on) != 2 || on[0] != 1 || on[1] != 2 {
			t.Errorf("lines on pages %v, want [1 2]", on)
		}
	})
}

// In FlowText, a paragraph with break-inside: avoid that does not fit below
// what is in the region goes to the next region whole (#75).
func TestFlowTextBreakInsideAvoid(t *testing.T) {
	cb, _ := newFlowBuilder(t, "")
	tr := flow(t, cb, `<p>`+nestLines("F", 8)+`</p><p style="break-inside: avoid">`+nestLines("L", 6)+`</p>`, wide("156pt"))
	var perRegion []int
	for _, f := range tr.filled {
		n := 0
		for _, l := range boxLines(f) {
			if strings.HasPrefix(l.text, "L") {
				n++
			}
		}
		perRegion = append(perRegion, n)
	}
	if len(perRegion) < 2 || perRegion[0] != 0 || perRegion[1] != 6 {
		t.Errorf("lines L per region %v, want none in the first and 6 in the second", perRegion)
	}
}
