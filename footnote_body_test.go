package htmlbag

import (
	"slices"
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

// textInFootnoteArea returns the first page on which body text reaches into
// the room reserved for the footnotes, the skip above the separator
// included, and by how much; 0 when the text keeps clear everywhere.
func textInFootnoteArea(pages []*document.Page, skip bag.ScaledPoint) (int, bag.ScaledPoint) {
	for i, pg := range pages {
		sep := slices.IndexFunc(pg.Objects, func(obj document.Object) bool {
			o, _ := obj.Vlist.Attributes["origin"].(string)
			return o == "footnote separator vlist"
		})
		if sep < 0 {
			continue
		}
		top := pg.Objects[sep].Y + skip
		// The body is painted before the footnotes.
		for _, obj := range pg.Objects[:sep] {
			var sb strings.Builder
			collectGlyphs(obj.Vlist.List, &sb)
			if sb.Len() == 0 {
				continue
			}
			if bottom := obj.Y - obj.Vlist.Height - obj.Vlist.Depth; bottom < top {
				return i + 1, top - bottom
			}
		}
	}
	return 0, 0
}

// A block split across pages keeps the room of the footnotes in its first
// part free. The split of a container went by the height of its children
// alone, and the lines of a paragraph after the call ran into the footnote
// (#99). The paragraph before the container keeps the container from being
// unwrapped to its sections.
func TestFootnoteRoomInSplitContainer(t *testing.T) {
	note := strings.Repeat("A long footnote that takes up room at the foot of the page. ", 4)
	words := strings.Repeat("Text of the paragraph. ", 70)
	html := `<html><body><p>Intro.</p><div><section><p>Short. Callhere.<span class="footnote">` + note + `</span></p></section>` +
		`<section><p>` + words + ` Laststands.</p></section></div></body></html>`
	pages, cb := renderHTMLPagesCB(t, `@page { size: a6; margin: 1cm } body { font-size: 9pt }`, html)
	if call, fn := pageOf(pages, "Callhere."), pageOf(pages, "Alongfootnote"); call != 1 || fn != 1 {
		t.Fatalf("the call is on page %d, the note on page %d; the test needs both on page 1", call, fn)
	}
	if pg, by := textInFootnoteArea(pages, cb.FootnoteSeparatorSkip); pg != 0 {
		t.Errorf("the text on page %d runs %s into the footnotes", pg, by)
	}
	if last := pageOf(pages, "Laststands."); last != 2 {
		t.Errorf("the paragraph ends on page %d, want 2", last)
	}
}
