package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// footnoteBodies returns the bodies of the footnote markers in te, in
// document order.
func footnoteBodies(te *frontend.Text) []*frontend.Text {
	var out []*frontend.Text
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case insertMarker:
			if t.Class == InsertFootnote {
				out = append(out, t.Body)
			}
		case *frontend.Text:
			out = append(out, footnoteBodies(t)...)
		}
	}
	return out
}

// The body of a footnote takes the footnote element's own styles, not the
// paragraph's: the number in front of the note is set with them, and came
// out at the paragraph's size next to a note made smaller in CSS.
func TestFootnoteBodyTakesTheElementsStyles(t *testing.T) {
	for name, html := range map[string]string{
		"in the paragraph": `<p>Text.<span class="footnote" style="font-size: 6pt">Note.</span></p>`,
		"in an inline":     `<p>Text <em>with<span class="footnote" style="font-size: 6pt">Note.</span></em>.</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, te := runStylePassWithBuilder(t, `<!DOCTYPE html><html><body>`+html+`</body></html>`)
			bodies := footnoteBodies(te)
			if len(bodies) != 1 {
				t.Fatalf("%d footnotes, want 1", len(bodies))
			}
			if got := bodies[0].Settings[frontend.SettingSize]; got != bag.MustSP("6pt") {
				t.Errorf("footnote body size %v, want 6pt", got)
			}
		})
	}
}

// pageOf returns the 1-based number of the first page whose text contains
// s, 0 when none does.
func pageOf(pages []*document.Page, s string) int {
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist == nil {
				continue
			}
			var sb strings.Builder
			collectGlyphs(obj.Vlist.List, &sb)
			if strings.Contains(sb.String(), s) {
				return i + 1
			}
		}
	}
	return 0
}

// The footnote of a paragraph split across pages goes to the page that
// holds its call, not to the one the paragraph starts on.
func TestFootnoteGoesWithItsLine(t *testing.T) {
	words := strings.Repeat("Justified text over its interword spaces. ", 6)
	html := `<html><body>` + strings.Repeat("<p>"+words+"</p>", 4) +
		`<p>Splitstarts ` + strings.Repeat(words, 3) + ` The call is here.<span class="footnote">Thelatenote.</span> ` + words + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, `@page { size: a6; margin: 1cm } body { font-size: 9pt }`, html)
	start, call, note := pageOf(pages, "Splitstarts"), pageOf(pages, "Thecallishere."), pageOf(pages, "Thelatenote.")
	if start == 0 || call <= start {
		t.Fatalf("the paragraph starts on page %d, the call is on page %d; the test needs the call on a later page", start, call)
	}
	if note != call {
		t.Errorf("the note is on page %d, its call on page %d", note, call)
	}
}

// A line whose footnote does not fit on the page any more moves on with it.
func TestFootnoteMovesOnWithItsLine(t *testing.T) {
	long := strings.Repeat("A long footnote that takes up room at the foot of the page. ", 8)
	words := strings.Repeat("Text of the paragraph. ", 120)
	html := `<html><body><p>` + words + ` Callhere.<span class="footnote">` + long + `</span> ` + words + `</p></body></html>`
	pages, _ := renderHTMLPagesCB(t, `@page { size: a6; margin: 1cm } body { font-size: 9pt }`, html)
	call, note := pageOf(pages, "Callhere."), pageOf(pages, "Alongfootnote")
	if call == 0 || note != call {
		t.Errorf("the note is on page %d, its call on page %d", note, call)
	}
}
