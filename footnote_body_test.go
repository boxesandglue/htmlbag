package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
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
