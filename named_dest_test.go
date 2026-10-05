package htmlbag

import (
	"regexp"
	"strconv"
	"testing"
)

// namedDests reads the page number and the y position of each named
// destination in an uncompressed PDF.
func namedDests(t *testing.T, pdf []byte) map[string][2]float64 {
	t.Helper()
	kids := regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`).FindSubmatch(pdf)
	if kids == nil {
		t.Fatal("no /Kids in the PDF")
	}
	pageNum := map[string]int{}
	for i, ref := range regexp.MustCompile(`(\d+) 0 R`).FindAllSubmatch(kids[1], -1) {
		pageNum[string(ref[1])] = i + 1
	}
	names := regexp.MustCompile(`/Names\s*\[([^\]]*)\]`).FindSubmatch(pdf)
	dests := map[string][2]float64{}
	if names == nil {
		return dests
	}
	for _, m := range regexp.MustCompile(`\(([^)]*)\)\s*(\d+) 0 R`).FindAllSubmatch(names[1], -1) {
		obj := regexp.MustCompile(`(?s)\n` + string(m[2]) + ` 0 obj\s*<<\s*/D\s*\[\s*(\d+) 0 R\s*/XYZ\s*([\d.]+)\s*([\d.]+)`).FindSubmatch(pdf)
		if obj == nil {
			t.Fatalf("destination %s: no object %s", m[1], m[2])
		}
		y, _ := strconv.ParseFloat(string(obj[3]), 64)
		dests[string(m[1])] = [2]float64{float64(pageNum[string(obj[1])]), y}
	}
	return dests
}

// Every element with an id is a link target: a div and a span as well as a
// paragraph and a heading (#69), at the top of the element's first fragment.
// The page is 200pt high with 20pt margins, the lines 12pt.
func TestNamedDestinationForEveryID(t *testing.T) {
	compress := func(cb *CSSBuilder) { cb.frontend.Doc.CompressLevel = 0 }
	t.Run("div, heading, paragraph, span", func(t *testing.T) {
		pdf := renderLineModelPDF(t, charCSS, `<div id="d"><p>In a div.</p></div>
<h1 id="h">A heading</h1>
<p id="p">A paragraph with a <span id="s">span</span>.</p>`, compress)
		dests := namedDests(t, pdf)
		d, h, p, sp := dests["d"], dests["h"], dests["p"], dests["s"]
		if d != [2]float64{1, 180} {
			t.Errorf("destination d at %v, want page 1, y 180, the top of the div", d)
		}
		if h[0] != 1 || p[0] != 1 || !(h[1] < 180 && p[1] < h[1]) {
			t.Errorf("destinations h at %v and p at %v, want both on page 1, p below h below the div", h, p)
		}
		if sp != p {
			t.Errorf("destination s at %v, want %v, in the paragraph's first line", sp, p)
		}
	})
	t.Run("pre-rendered box among blocks", func(t *testing.T) {
		pdf := renderLineModelPDF(t, charCSS, `<p>A</p><div id="b1" data-vlist-id="b"></div>`, func(cb *CSSBuilder) {
			compress(cb)
			pendingBox(t, cb, "b")
		})
		if got := namedDests(t, pdf)["b1"]; got != [2]float64{1, 168} {
			t.Errorf("destination b1 at %v, want page 1, y 168, below the paragraph", got)
		}
	})
	t.Run("div that moves on to page 2", func(t *testing.T) {
		pdf := renderLineModelPDF(t, charCSS, `<p>A</p><div id="d" style="break-before: page"><p>B</p></div>`, compress)
		if got := namedDests(t, pdf)["d"]; got[0] != 2 || got[1] != 180 {
			t.Errorf("destination d at page %v, y %v; want page 2, y 180", got[0], got[1])
		}
	})
	t.Run("div split across pages", func(t *testing.T) {
		var lines string
		for i := 0; i < 12; i++ {
			lines += "<p>line</p>"
		}
		pdf := renderLineModelPDF(t, charCSS, lines+`<div id="d"><p>first</p><p>second</p><p>third</p></div>`, compress)
		// Twelve lines leave room for the div's first line on page 1.
		if got := namedDests(t, pdf)["d"]; got != [2]float64{1, 36} {
			t.Errorf("destination d at %v, want page 1, y 36, where the div starts", got)
		}
	})
}
