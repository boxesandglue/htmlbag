package htmlbag

import (
	"strings"
	"testing"
)

// A <caption> is set as a block above its table; it was dropped before.
func TestTableCaption(t *testing.T) {
	html := `<html><body><table><caption>Thecaption</caption><tr><td>Cell</td></tr></table></body></html>`
	pages, _ := renderHTMLPagesCB(t, `@page { size: a6; margin: 1cm }`, html)
	texts := pageTexts(pages[0])
	if len(texts) < 2 {
		t.Fatalf("texts on the page: %v, want the caption and the cell", texts)
	}
	if texts[0].text != "Thecaption" || texts[0].y <= texts[1].y {
		t.Errorf("first text %q at %s, then %q at %s; want the caption above the table", texts[0].text, texts[0].y, texts[1].text, texts[1].y)
	}
}

// A caption keeps with its table: at the foot of a page it moves on with it.
func TestTableCaptionKeepsWithTable(t *testing.T) {
	filler := strings.Repeat("<p>Filler text for the page.</p>", 28)
	html := `<html><body>` + filler + `<table><caption>Thecaption</caption><tr><td>Cell</td></tr><tr><td>Cell</td></tr></table></body></html>`
	pages, _ := renderHTMLPagesCB(t, `@page { size: a6; margin: 1cm } p { margin: 0 }`, html)
	caption, cell := pageOf(pages, "Thecaption"), pageOf(pages, "Cell")
	if caption == 0 || caption != cell {
		t.Errorf("the caption is on page %d, the table starts on page %d", caption, cell)
	}
}
