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

// pagesOf maps each heading's text and each anchor's id to its page.
func pagesOf(cb *CSSBuilder) (headings, anchors map[string]int) {
	headings, anchors = map[string]int{}, map[string]int{}
	for _, h := range cb.Headings {
		headings[h.Text] = h.Page
	}
	for _, a := range cb.Anchors {
		anchors[a.ID] = a.Page
	}
	return headings, anchors
}

// A lone block taller than the region still splits across pages, and its
// heading and anchor take the page it starts on. A box with one child is not
// _splittable, so kept whole it would run off the page.
func TestTallLoneBlockKeepsItsHeadingAndAnchor(t *testing.T) {
	for _, c := range []struct{ name, html string }{
		{"div with an id", `<div id="main"><p>` + lines(67) + `</p></div>`},
		{"nested", `<section id="main"><div><p>` + lines(67) + `</p></div></section>`},
		{"heading", `<h1 id="main">` + lines(67) + `</h1>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages, cb := renderHTMLPagesCB(t, charCSS, c.html)
			if len(pages) != 6 {
				t.Errorf("%d pages, want 6", len(pages))
			}
			headings, anchors := pagesOf(cb)
			if anchors["main"] != 1 || len(anchors) != 1 {
				t.Errorf("anchors %v, want main on page 1", anchors)
			}
			if c.name == "heading" && (len(headings) != 1 || cb.Headings[0].Page != 1) {
				t.Errorf("headings %v, want one on page 1", headings)
			}
		})
	}
}

// A tall lone container with headings is split between and inside its
// children, and each heading and anchor keeps its own page.
func TestTallLoneContainerKeepsItsChildrensPages(t *testing.T) {
	html := `<div id="main"><h1 id="a">A</h1><p>` + lines(30) + `</p><h1 id="b">B</h1><p>` +
		lines(40) + `</p><h1 id="c">C</h1><p>x</p></div>`
	pages, cb := renderHTMLPagesCB(t, charCSS, html)
	if len(pages) != 6 {
		t.Errorf("%d pages, want 6", len(pages))
	}
	headings, anchors := pagesOf(cb)
	for k, want := range map[string]int{"A": 1, "B": 3, "C": 6} {
		if headings[k] != want {
			t.Errorf("heading %s on page %d, want %d (%v)", k, headings[k], want, headings)
		}
	}
	for k, want := range map[string]int{"main": 1, "a": 1, "b": 3, "c": 6} {
		if anchors[k] != want {
			t.Errorf("anchor %s on page %d, want %d (%v)", k, anchors[k], want, anchors)
		}
	}
}
