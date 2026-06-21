package htmlbag

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestAvoidBreakAfterKeepsHeadingWithSplittableCard guards the orphan-heading
// fix: a `page-break-after: avoid` heading near the bottom of a page must travel
// with its following block, even when that block is a bordered card whose
// children are block-level VLists (paragraphs/divs) rather than HList text
// lines. Before the fix, outputBlockSplit's orphan protection counted only
// HList children, saw zero "lines" in such a card, and shunted the whole card
// to the next page — leaving the heading orphaned at the bottom of the previous
// page. We assert the heading anchor and the card anchor land on the same page.
func TestAvoidBreakAfterKeepsHeadingWithSplittableCard(t *testing.T) {
	const css = `
@page { size: A4; margin: 2cm; }
body { font-family: sans; font-size: 11pt; }
.group {
  margin: 18pt 0 6pt 0;
  padding-bottom: 2pt;
  border-bottom: 0.5pt solid #e5e5e5;
  page-break-after: avoid;
}
.check { margin: 8pt 0 0 0; padding: 0 0 8pt 0; border-bottom: 0.5pt solid #f0f0f0; }
.description { font-size: 9pt; margin: 3pt 0 0 0; }
.finding { margin: 6pt 0 0 12pt; padding: 2pt 0 2pt 8pt; border-left: 2pt solid #1664c0; }
`

	// Build enough short bordered filler cards to push the kept heading near
	// the bottom of page 1, then the tall card with seven bordered findings.
	var b strings.Builder
	for i := 1; i <= 13; i++ {
		fmt.Fprintf(&b, `<article class="check"><p>Passing check %d</p>`+
			`<p class="description">One-line description for filler check %d.</p></article>`, i, i)
	}
	b.WriteString(`<p class="group" id="heading">Suggestions (1)</p>`)
	b.WriteString(`<article class="check" id="card"><p>Link annotations have /Contents</p>` +
		`<p class="description">Every interactive Link annotation should expose a /Contents value ` +
		`describing where the link goes; without it assistive technology reads the raw URI.</p>`)
	for i := 1; i <= 7; i++ {
		b.WriteString(`<div class="finding"><p>Link annotation has no /Contents</p>` +
			`<p class="description">Add /Contents with a short description of the link target.</p></div>`)
	}
	b.WriteString(`</article>`)
	html := `<!DOCTYPE html><html><body>` + b.String() + `</body></html>`

	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("frontend.NewForWriter: %v", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatalf("LoadIncludedFonts: %v", err)
	}
	cb, err := New(fe, csshtml.NewCSSParserWithDefaults())
	if err != nil {
		t.Fatalf("htmlbag.New: %v", err)
	}
	if err := cb.ParseCSSString(css); err != nil {
		t.Fatalf("ParseCSSString: %v", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatalf("InitPage: %v", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatalf("HTMLToText: %v", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatalf("OutputPagesFromText: %v", err)
	}

	pageOf := func(id string) int {
		for _, a := range cb.Anchors {
			if a.ID == id {
				return a.Page
			}
		}
		t.Fatalf("anchor %q not collected", id)
		return 0
	}
	headingPage := pageOf("heading")
	cardPage := pageOf("card")
	if headingPage == 0 || cardPage == 0 {
		t.Fatalf("anchors not page-assigned: heading=%d card=%d", headingPage, cardPage)
	}
	if headingPage != cardPage {
		t.Errorf("page-break-after:avoid heading orphaned: heading on page %d, card starts on page %d; want same page",
			headingPage, cardPage)
	}
}
