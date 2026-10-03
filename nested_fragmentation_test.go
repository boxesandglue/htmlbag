package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
)

// The tests in this file are about blocks below the top level of the body: a
// paragraph or a box inside a container splits across pages as it would at
// the top level (CSS Fragmentation 3, class A and B breaks at any depth), a
// forced break inside a container is taken, and a heading or an anchor inside
// a container takes the page it lands on. Issues #37, #59 and #65.

const nestedCSS = `.b { border: 1pt solid black }
div, section, main, header, ul, ol, li { margin: 0; padding: 0 }
ul, ol { padding-left: 20pt }
li { font-size: 10pt; line-height: 12pt }`

// nestParas is n paragraphs prefix1 to prefixn.
func nestParas(prefix string, n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "<p>%s%d</p>", prefix, i)
	}
	return sb.String()
}

// nestLines is one paragraph of n lines prefix1 to prefixn.
func nestLines(prefix string, n int) string {
	s := make([]string, n)
	for i := range s {
		s[i] = fmt.Sprintf("%s%d", prefix, i+1)
	}
	return strings.Join(s, "<br>")
}

// linePages renders html on charCSS pages and returns the page of every line
// whose text starts with prefix, in painting order. It fails when a line ends
// below the content area.
func linePages(t *testing.T, html, prefix string) (pages int, on []int) {
	t.Helper()
	pp := renderHTMLPages(t, charCSS+nestedCSS, html)
	foot := charTop - bag.MustSP("160pt") - bag.MustSP("0.01pt")
	for _, l := range boxedLines(pp) {
		if l.bottom < foot {
			t.Errorf("%q on page %d ends at %s, below the content area", l.text, l.page, l.bottom)
		}
		// A list item's first line starts with its marker.
		got := strings.TrimLeft(l.text, "•0123456789.")
		if !strings.HasPrefix(got, prefix) {
			continue
		}
		want := fmt.Sprintf("%s%d", prefix, len(on)+1)
		if got != want {
			t.Fatalf("line %q, want %q", got, want)
		}
		on = append(on, l.page)
	}
	return len(pp), on
}

// countOn is how many entries of on are page.
func countOn(on []int, page int) int {
	n := 0
	for _, p := range on {
		if p == page {
			n++
		}
	}
	return n
}

// nestPageOf renders html and returns the page of the first line that is text.
func nestPageOf(t *testing.T, html, text string) (pages, page int) {
	t.Helper()
	pp := renderHTMLPages(t, charCSS+nestedCSS, html)
	for _, l := range boxedLines(pp) {
		if l.text == text {
			return len(pp), l.page
		}
	}
	t.Fatalf("no line %q", text)
	return 0, 0
}

// A heading and an element id inside a container take the page they land
// on, also when the container has siblings.
func TestNestedHeadingsAndAnchorsTakeTheirPage(t *testing.T) {
	cases := []struct {
		name, html string
		heads      map[string]int
		anchors    map[string]int
	}{
		{"section after a paragraph", `<p>X</p><section><h1 id="a">A</h1><p>y</p></section>`, map[string]int{"A": 1}, map[string]int{"a": 1}},
		{"heading after a paragraph in a section", `<p>X</p><section><p>y</p><h1>A</h1></section>`, map[string]int{"A": 1}, nil},
		{"two sections, the second on page 2", `<section><h1>A</h1><p>` + nestLines("L", 12) + `</p></section><section id="s2"><h1>B</h1><p>z</p></section>`, map[string]int{"A": 1, "B": 2}, map[string]int{"s2": 2}},
		{"heading in a split box", `<p>X</p><div class="b">` + nestParas("L", 14) + `<h1>B</h1></div>`, map[string]int{"B": 2}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cb := renderHTMLPagesCB(t, charCSS+nestedCSS, c.html)
			got := map[string]int{}
			for _, h := range cb.Headings {
				got[h.Text] = h.Page
			}
			for text, page := range c.heads {
				if got[text] != page {
					t.Errorf("heading %q on page %d, want %d", text, got[text], page)
				}
			}
			gotA := map[string]int{}
			for _, a := range cb.Anchors {
				gotA[a.ID] = a.Page
			}
			for id, page := range c.anchors {
				if gotA[id] != page {
					t.Errorf("anchor %q on page %d, want %d", id, gotA[id], page)
				}
			}
		})
	}
}
