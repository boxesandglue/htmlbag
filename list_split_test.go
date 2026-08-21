package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

const listSplitCSS = `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 11pt }`

// buildListHTML wraps n <li> items, each carrying a unique marker word, in a
// list of the given tag.
func buildListHTML(tag string, n int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<%s>", tag)
	for i := range n {
		fmt.Fprintf(&sb, "<li>ITEM%03d</li>", i)
	}
	fmt.Fprintf(&sb, "</%s>", tag)
	return sb.String()
}

// countItems reports how many of the ITEMnnn markers appear across all pages.
func countItems(pages []*document.Page, total int) int {
	var all strings.Builder
	for _, pg := range pages {
		all.WriteString(pageText(pg))
	}
	text := all.String()
	found := 0
	for i := range total {
		if strings.Contains(text, fmt.Sprintf("ITEM%03d", i)) {
			found++
		}
	}
	return found
}

// A list taller than one page must flow across pages instead of being shipped
// whole (which dropped every item past the first page's worth).
func TestListSplitsAcrossPages(t *testing.T) {
	for _, tag := range []string{"ul", "ol"} {
		t.Run(tag, func(t *testing.T) {
			const items = 120
			pages := renderHTMLPages(t, listSplitCSS, buildListHTML(tag, items))
			if len(pages) < 2 {
				t.Fatalf("<%s> with %d items produced %d page(s), want at least 2", tag, items, len(pages))
			}
			if got := countItems(pages, items); got != items {
				t.Errorf("only %d of %d list items reached the PDF", got, items)
			}
			// The list starts at the top of the document, so page 1 must
			// carry items rather than being left blank.
			if txt := pageText(pages[0]); !strings.Contains(txt, "ITEM000") {
				t.Errorf("first item is missing from page 1: %q", truncate(txt, 80))
			}
		})
	}
}

// A list preceded and followed by paragraphs keeps both siblings and every
// item.
func TestListSplitKeepsSiblings(t *testing.T) {
	const items = 120
	html := "<p>INTROPARA</p>" + buildListHTML("ul", items) + "<p>OUTROPARA</p>"
	pages := renderHTMLPages(t, listSplitCSS, html)
	if got := countItems(pages, items); got != items {
		t.Errorf("only %d of %d list items reached the PDF", got, items)
	}
	var all strings.Builder
	for _, pg := range pages {
		all.WriteString(pageText(pg))
	}
	for _, want := range []string{"INTROPARA", "OUTROPARA"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("%s missing from the output", want)
		}
	}
	if !strings.Contains(pageText(pages[0]), "INTROPARA") {
		t.Error("intro paragraph did not stay on page 1")
	}
}

// break-inside: avoid keeps a list monolithic: it moves to the next page
// rather than being fragmented.
func TestListBreakInsideAvoidNotSplit(t *testing.T) {
	css := listSplitCSS + "\nul { break-inside: avoid }"
	const items = 12
	html := "<p>FILLERPARA</p>" + buildListHTML("ul", items)
	pages := renderHTMLPages(t, css, html)
	// All items must sit on a single page.
	pagesWithItems := 0
	for _, pg := range pages {
		if strings.Contains(pageText(pg), "ITEM000") || strings.Contains(pageText(pg), "ITEM011") {
			pagesWithItems++
		}
	}
	if pagesWithItems != 1 {
		t.Errorf("break-inside: avoid list spans %d pages, want 1", pagesWithItems)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
