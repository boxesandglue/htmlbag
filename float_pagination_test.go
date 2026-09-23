package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// Floats at a page break. The page geometry is the one pageFiller describes:
// an A4 page with 20mm margins holds twenty three-line filler paragraphs,
// with a few points to spare.

const paginationCSS = `@page { size: a4; margin: 20mm; } p { margin: 0; }`

// pageFlow lists, in painting order, what a page holds of interest: "float"
// for every float box and "line" for every line of the flow beside it. The
// lines inside a float box are its own and are not listed.
func pageFlow(pg *document.Page) []string {
	var out []string
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.VList:
				if _, isFloat := floatBoxHeight(c); isFloat {
					out = append(out, "float")
					continue
				}
				walk(c.List)
			case *node.HList:
				if origin, _ := c.Attributes["origin"].(string); origin == "line" {
					out = append(out, "line")
					continue
				}
				walk(c.List)
			}
		}
	}
	for _, obj := range pg.Objects {
		if obj.Vlist != nil {
			walk(obj.Vlist)
		}
	}
	return out
}

// floatPage returns the number of the first page holding a float box.
func floatPage(pages []*document.Page) int {
	for i, pg := range pages {
		for _, tok := range pageFlow(pg) {
			if tok == "float" {
				return i + 1
			}
		}
	}
	return 0
}

// assertFloatsKeepTheirText fails when a page ends with a float box: the
// block beside a float goes with it to the next page.
func assertFloatsKeepTheirText(t *testing.T, pages []*document.Page) {
	t.Helper()
	for i, pg := range pages {
		flow := pageFlow(pg)
		for j, tok := range flow {
			if tok != "float" {
				continue
			}
			if j == len(flow)-1 || flow[j+1] != "line" {
				t.Errorf("page %d: a float box is the last thing on the page, parted from the text beside it", i+1)
			}
		}
	}
}

func countIndented(indents []bag.ScaledPoint) int {
	n := 0
	for _, in := range indents {
		if in > 0 {
			n++
		}
	}
	return n
}

// A margin note written before a paragraph the page has no room for goes to
// the next page with that paragraph rather than staying behind alone.
func TestAMarginNoteStaysWithItsParagraph(t *testing.T) {
	css := paginationCSS + ` .note { float: left; width: 40pt; margin-left: -50pt; margin-right: 10pt; }`
	for fillers := 18; fillers <= 21; fillers++ {
		html := `<!DOCTYPE html><html><body>` + pageFiller(fillers) +
			`<div class="note">note</div><p>` + floatProse + `</p>` + pageFiller(3) + `</body></html>`
		pages, _ := renderHTMLPagesCB(t, css, html)
		if floatPage(pages) == 0 {
			t.Fatalf("%d fillers: the note was not painted at all", fillers)
		}
		assertFloatsKeepTheirText(t, pages)
	}
}

// A float taller than what is left of the page moves to the next page, and
// the paragraph beside it moves with it: the lines give way to it there.
func TestAFloatTallerThanTheRestOfThePageMovesOn(t *testing.T) {
	css := paginationCSS + ` .fig { float: left; width: 40pt; height: 60pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="fig"></div><p>` + strings.Repeat(floatProse+" ", 2) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 2 {
		t.Fatalf("float painted on page %d, want 2 (it does not fit on page 1)", got)
	}
	assertFloatsKeepTheirText(t, pages)
	if countIndented(pageLineIndents(pages[1])) == 0 {
		t.Errorf("page 2: no line gives way to the float beside it")
	}
}

// A paragraph split at a page break keeps its narrowed lines beside the
// float on the first page; the lines on the next page are beside nothing and
// run at full width, even where the float's band would have covered them.
func TestASplitParagraphLeavesTheBandBehind(t *testing.T) {
	css := paginationCSS + ` .fig { float: left; width: 40pt; height: 40pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="fig"></div><p>` + strings.Repeat(floatProse+" ", 3) + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 1 {
		t.Fatalf("float painted on page %d, want 1 (it fits with three lines beside it)", got)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want the paragraph to continue on page 2", len(pages))
	}
	if countIndented(pageLineIndents(pages[0])) == 0 {
		t.Errorf("page 1: the lines beside the float are not narrowed")
	}
	if n := countIndented(pageLineIndents(pages[1])); n > 0 {
		t.Errorf("page 2: %d lines are narrowed beside a float that is on page 1", n)
	}
}

// The band of a float reaches into the paragraph after the one beside it.
// When the page break falls between the two, the second paragraph is rebuilt
// at full width on the next page.
func TestABandCutByThePageBreakEndsThere(t *testing.T) {
	css := paginationCSS + ` .fig { float: left; width: 40pt; height: 40pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(19) +
		`<div class="fig"></div>` + pageFiller(3) + `</body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 1 {
		t.Fatalf("float painted on page %d, want 1", got)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if countIndented(pageLineIndents(pages[0])) == 0 {
		t.Errorf("page 1: the lines beside the float are not narrowed")
	}
	if n := countIndented(pageLineIndents(pages[1])); n > 0 {
		t.Errorf("page 2: %d lines are narrowed beside a float that is on page 1", n)
	}
}

// The same two rules inside a block container that is split across pages: a
// float child stays with the child beside it, and children pushed to the
// next page are rebuilt at full width.
func TestAFloatInASplitContainerStaysWithItsText(t *testing.T) {
	css := paginationCSS + ` .note { float: left; width: 40pt; margin-left: -50pt; margin-right: 10pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(18) +
		`<div>` + pageFiller(2) + `<div class="note">note</div>` + pageFiller(2) + `</div></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if floatPage(pages) == 0 {
		t.Fatalf("the note was not painted at all")
	}
	assertFloatsKeepTheirText(t, pages)
}

func TestASplitContainerLeavesTheBandBehind(t *testing.T) {
	css := paginationCSS + ` .fig { float: left; width: 40pt; height: 76pt; }`
	html := `<!DOCTYPE html><html><body>` + pageFiller(18) +
		`<div><div class="fig"></div>` + pageFiller(4) + `</div></body></html>`
	pages, _ := renderHTMLPagesCB(t, css, html)
	if got := floatPage(pages); got != 1 {
		t.Fatalf("float painted on page %d, want 1", got)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if countIndented(pageLineIndents(pages[0])) == 0 {
		t.Errorf("page 1: the lines beside the float are not narrowed")
	}
	if n := countIndented(pageLineIndents(pages[1])); n > 0 {
		t.Errorf("page 2: %d lines are narrowed beside a float that is on page 1", n)
	}
}
