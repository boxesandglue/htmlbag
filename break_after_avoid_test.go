package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

const breakAfterCSS = `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 11pt }
h2 { break-after: avoid }
.card { border: 1pt solid black; padding: 4pt }
.fixed { height: 12cm; background: #eeeeee }`

// headingIsOrphaned reports whether the heading marker is the last content on
// its page while more content follows on a later page.
func headingIsOrphaned(pages []*document.Page, marker string) bool {
	for i, pg := range pages {
		txt := pageText(pg)
		idx := strings.Index(txt, marker)
		if idx < 0 {
			continue
		}
		// Something after the heading on the same page: not orphaned.
		if strings.TrimSpace(txt[idx+len(marker):]) != "" {
			return false
		}
		// Heading ends the page and more pages follow: orphaned.
		return i < len(pages)-1
	}
	return false
}

// A heading with break-after: avoid must never end up alone at the bottom of a
// page, whatever kind of block follows it and wherever it sits in the tree.
func TestBreakAfterAvoidNoOrphanHeading(t *testing.T) {
	longList := buildListHTML("ul", 30)
	card := "<div class=\"card\">" + fillerParagraphs(40) + "</div>"

	cases := map[string]string{
		"paragraph":     "<h2>STICKY</h2><p>Follower paragraph.</p><p>And more text.</p>",
		"list":          "<h2>STICKY</h2>" + longList,
		"borderedCard":  "<h2>STICKY</h2>" + card,
		"fixedHeight":   "<h2>STICKY</h2><div class=\"fixed\">Block content</div>",
		"inWrapperDiv":  "<div><h2>STICKY</h2></div><p>Follower paragraph.</p><p>And more.</p>",
		"inWrapperSect": "<section><h2>STICKY</h2><p>Follower paragraph.</p><p>And more.</p></section>",
		// The wrapper hides the heading's break-after from the paginator
		// unless the value propagates from the last in-flow child, and the
		// unsplittable fixed-height block leaves no room to recover.
		"wrappedThenFixed": "<div><h2>STICKY</h2></div><div class=\"fixed\">Block content</div>",
	}

	for name, tail := range cases {
		t.Run(name, func(t *testing.T) {
			// Sweep the filler count so the heading lands at every possible
			// offset relative to the page bottom.
			for n := 18; n <= 30; n++ {
				html := fillerParagraphs(n) + tail
				pages := renderHTMLPages(t, breakAfterCSS, html)
				if headingIsOrphaned(pages, "STICKY") {
					t.Errorf("filler=%d: heading orphaned at the bottom of its page", n)
				}
			}
		})
	}
}
