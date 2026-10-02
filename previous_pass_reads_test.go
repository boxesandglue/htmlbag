package htmlbag

import "testing"

func TestPreviousPassReads(t *testing.T) {
	css := `@page { size: 200pt 200pt; margin: 20pt; }`
	t.Run("nothing", func(t *testing.T) {
		_, cb := renderHTMLPagesCB(t, css, `<h1 id="a">Title</h1><p>Text with no reference.</p>`)
		if r := cb.PreviousPassReads(); r.Pages || len(r.Anchors) != 0 {
			t.Errorf("reads = %+v, want none", r)
		}
	})
	t.Run("target functions", func(t *testing.T) {
		_, cb := renderHTMLPagesCB(t, css+` a.p::after { content: " on page " target-counter(attr(href), page) }
			a.t::after { content: target-text(attr(href)) }`,
			`<h1 id="a">Title</h1><p><a class="p" href="#a">see</a> <a class="t" href="#missing">and</a></p>`)
		r := cb.PreviousPassReads()
		if r.Pages || !r.Anchors["a"] || !r.Anchors["missing"] || len(r.Anchors) != 2 {
			t.Errorf("reads = %+v, want anchors a and missing", r)
		}
	})
	t.Run("pages in a margin box", func(t *testing.T) {
		_, cb := renderHTMLPagesCB(t, `@page { size: 200pt 200pt; margin: 20pt; @bottom-center { content: counter(page) " of " counter(pages) } }`,
			`<p>Text.</p>`)
		if r := cb.PreviousPassReads(); !r.Pages || len(r.Anchors) != 0 {
			t.Errorf("reads = %+v, want pages", r)
		}
	})
}
