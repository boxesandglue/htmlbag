package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
)

// A block that is the only child of the body or of a FlowText flow keeps the
// box its heading, anchor and element id are recorded on: the outline entry
// and target-counter(…, page) take the page from it, and the id stays
// findable on the laid-out page.
func TestLoneBlockKeepsItsHeadingAndID(t *testing.T) {
	for _, c := range []struct{ name, before string }{
		{"only block", ""},
		{"after a block", "<p>Before</p>"},
	} {
		t.Run(c.name+", page", func(t *testing.T) {
			pages, cb := renderHTMLPagesCB(t, charCSS, c.before+`<h1 id="head">Heading</h1>`)
			if len(cb.Headings) != 1 || cb.Headings[0].Page != 1 {
				t.Errorf("headings %+v, want one on page 1", cb.Headings)
			}
			if len(cb.Anchors) != 1 || cb.Anchors[0].Page != 1 {
				t.Errorf("anchors %+v, want one on page 1", cb.Anchors)
			}
			if n := elementIDs(pages)["head"]; n != 1 {
				t.Errorf(`id "head" is on %d boxes, want 1`, n)
			}
		})
		t.Run(c.name+", FlowText", func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			r := wide("1000pt")
			r.PageNum = 3
			tr := flow(t, cb, c.before+`<p id="para">Paragraph</p>`, r)
			if len(cb.Anchors) != 1 || cb.Anchors[0].Page != 3 {
				t.Errorf("anchors %+v, want one on page 3", cb.Anchors)
			}
			pages := []*document.Page{{Objects: []document.Object{{Vlist: tr.filled[0].Box}}}}
			if n := elementIDs(pages)["para"]; n != 1 {
				t.Errorf(`id "para" is on %d boxes, want 1`, n)
			}
		})
	}
}
