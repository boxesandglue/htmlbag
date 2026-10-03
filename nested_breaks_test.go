package htmlbag

import (
	"strings"
	"testing"
)

// A forced break between two blocks is taken at any depth (CSS Fragmentation
// 3 §3.1), on pages.
func TestNestedForcedBreaks(t *testing.T) {
	cases := []struct {
		name, html, text string
		pages, page      int
	}{
		{"before, first child of a div", `<p>X</p><div><p style="break-before: page">A</p></div>`, "A", 2, 2},
		{"before, second child of a div", `<p>X</p><div><p>Y</p><p style="break-before: page">A</p></div>`, "A", 2, 2},
		{"before, heading of the second section", `<section><h1>One</h1><p>a</p></section><section><h1 style="break-before: page">Two</h1><p>b</p></section>`, "Two", 2, 2},
		{"after, first child of a div", `<p>X</p><div><p style="break-after: page">A</p><p>B</p></div>`, "B", 2, 2},
		{"after, last child of a div", `<p>X</p><div><p>A</p><p style="break-after: page">B</p></div><p>C</p>`, "C", 2, 2},
		{"before, in a bordered box", `<p>X</p><div class="b"><p>A</p><p style="break-before: page">B</p></div>`, "B", 2, 2},
		{"before, two levels down", `<p>X</p><div><div><p>Y</p><p style="break-before: page">A</p></div></div>`, "A", 2, 2},
		{"always, the old keyword", `<p>X</p><div><p>Y</p><p style="page-break-before: always">A</p></div>`, "A", 2, 2},
		{"before, first block of the document", `<section><h1 style="break-before: page">One</h1><p>a</p></section>`, "One", 1, 1},
		{"top level, unchanged", `<p>X</p><p style="break-before: page">A</p>`, "A", 2, 2},
		{"before, in the body's only div", `<div><p>X</p><p style="break-before: page">A</p></div>`, "A", 2, 2},
		{"before, in a div in a bordered box", `<p>X</p><div class="b"><div><p>Y</p><p style="break-before: page">A</p></div></div>`, "A", 2, 2},
		{"after, first section", `<section><p>a</p><p style="break-after: page">b</p></section><section><p>Two</p></section>`, "Two", 2, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pages, page := nestPageOf(t, c.html, c.text)
			if pages != c.pages || page != c.page {
				t.Errorf("%d pages, %s on page %d; want %d pages, page %d", pages, c.text, page, c.pages, c.page)
			}
		})
	}
}

// In FlowText, a nested forced break asks for the next region with its
// keyword, and the break-before of the first block, at any depth, goes to
// the first Next.
func TestFlowTextNestedForcedBreaks(t *testing.T) {
	cases := []struct {
		name, body string
		brks       []string
	}{
		{"column, nested", `<p>X</p><div><div style="break-before: column"><p>A</p></div></div>`, []string{"", "column"}},
		{"page after, nested", `<p>X</p><div><p style="break-after: page">A</p><p>B</p></div>`, []string{"", "page"}},
		{"first block, nested", `<div><p style="break-before: right">A</p></div><p>B</p>`, []string{"right"}},
		{"first block, two levels down", `<section><div><p style="break-before: column">A</p></div></section>`, []string{"column"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, nestedCSS)
			tr := flowCapped(t, cb, c.body, wide("1000pt"))
			if strings.Join(tr.brks, ",") != strings.Join(c.brks, ",") {
				t.Errorf("Next got %q, want %q", tr.brks, c.brks)
			}
		})
	}
}
