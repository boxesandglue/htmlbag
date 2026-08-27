package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// Regression test for the widow-protection pullback in outputBlockSplit: the
// pullback loop counted only HLists when advancing remainingLines, but the
// children of a transparent box container (a <ul>) are VLists. remainingLines
// therefore never advanced and the loop drained the batch down to the orphan
// minimum of two items, leaving the page half empty (seen as a half-blank
// page inside a long <ul> in a real document). A correct pullback moves the
// minimum number of items, so a page may end up with two items only when the
// following page does not carry more than that.
func TestListSplitWidowPullbackFillsPage(t *testing.T) {
	css := `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 11pt }
li { margin-bottom: 0.5em }`
	filler := strings.Repeat("lorem ipsum dolor sit amet consetetur sadipscing elitr sed diam nonumy eirmod tempor ", 3)

	// Both counts end one split step with a single leftover item, which is
	// what arms the pullback: 8 items trigger it on the page the list
	// starts on, 17 items on a follow-up (middle) page.
	for _, items := range []int{8, 17} {
		t.Run(fmt.Sprintf("%ditems", items), func(t *testing.T) {
			var sb strings.Builder
			sb.WriteString("<p>INTROPARA " + filler + "</p><ul>")
			for i := range items {
				fmt.Fprintf(&sb, "<li>ITEM%03d %s</li>", i, filler)
			}
			sb.WriteString("</ul>")

			pages := renderHTMLPages(t, css, sb.String())
			if got := countItems(pages, items); got != items {
				t.Fatalf("only %d of %d list items reached the PDF", got, items)
			}

			perPage := make([]int, len(pages))
			lastItemPage := 0
			for pi, pg := range pages {
				txt := pageText(pg)
				for i := range items {
					if strings.Contains(txt, fmt.Sprintf("ITEM%03d", i)) {
						perPage[pi]++
						lastItemPage = pi
					}
				}
			}
			for pi := 0; pi < lastItemPage; pi++ {
				if perPage[pi] > 0 && perPage[pi] <= 2 && perPage[pi+1] > perPage[pi] {
					t.Errorf("page %d carries only %d items while page %d carries %d: the widow pullback drained the batch (items per page: %v)",
						pi+1, perPage[pi], pi+2, perPage[pi+1], perPage)
				}
			}
		})
	}
}
