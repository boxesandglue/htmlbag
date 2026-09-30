package htmlbag

import (
	"fmt"
	"strings"
	"testing"
)

// linesPerPage renders a spacer that leaves room for `room` 20pt lines at the
// foot of the first page, then the body, and returns which page each line
// marker Lnn landed on.
func linesPerPage(t *testing.T, css string, room int, body string) map[string]int {
	t.Helper()
	base := `@page { size: 200pt 200pt; margin: 0 }
body { margin: 0; font-family: serif; font-size: 10pt; line-height: 20pt }
p, h2, div { margin: 0 }
h2 { font-size: 10pt; line-height: 20pt }
.spacer { height: %dpt }
`
	html := fmt.Sprintf(`<div class="spacer"></div>%s`, body)
	pages := renderHTMLPages(t, fmt.Sprintf(base, 200-20*room)+css, html)
	got := map[string]int{}
	for pi, pg := range pages {
		txt := pageText(pg)
		for i := 1; i <= 20; i++ {
			for _, prefix := range []string{"L", "H"} {
				m := fmt.Sprintf("%s%02d", prefix, i)
				if strings.Contains(txt, m) {
					got[m] = pi + 1
				}
			}
		}
	}
	return got
}

func lines(n int) string {
	var parts []string
	for i := 1; i <= n; i++ {
		parts = append(parts, fmt.Sprintf("L%02d", i))
	}
	return strings.Join(parts, "<br>")
}

// split reports how many of the first n lines landed on page 1 and page 2.
func split(got map[string]int, n int) (int, int) {
	var p1, p2 int
	for i := 1; i <= n; i++ {
		switch got[fmt.Sprintf("L%02d", i)] {
		case 1:
			p1++
		case 2:
			p2++
		}
	}
	return p1, p2
}

func TestWidowsOrphansFromCSS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		css    string
		room   int
		n      int
		p1, p2 int
	}{
		{"default", "", 3, 5, 3, 2},
		{"widows 3", "p { widows: 3 }", 3, 5, 2, 3},
		{"widows inherited", "body { widows: 3 }", 3, 5, 2, 3},
		{"widows initial", "body { widows: 3 } p { widows: initial }", 3, 5, 3, 2},
		{"widows 1 off", "p { widows: 1 }", 3, 4, 3, 1},
		{"default pulls back", "", 3, 4, 2, 2},
		{"orphans 4", "p { orphans: 4 }", 3, 5, 0, 5},
		{"orphans 1 off", "p { orphans: 1 }", 1, 4, 1, 3},
		{"default orphan moves", "", 1, 4, 0, 4},
		{"invalid ignored", "p { widows: 0; orphans: -1 }", 3, 5, 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := linesPerPage(t, tc.css, tc.room, "<p>"+lines(tc.n)+"</p>")
			if p1, p2 := split(got, tc.n); p1 != tc.p1 || p2 != tc.p2 {
				t.Errorf("lines on pages 1/2 = %d/%d, want %d/%d (%v)", p1, p2, tc.p1, tc.p2, got)
			}
		})
	}
}

// A break-after: avoid heading keeps the foothold the paragraph's orphans ask
// for. splittablePeekHeight and outputBlockSplit must agree on it, or the
// heading stays behind while the paragraph moves on.
func TestOrphansKeepAvoidHeading(t *testing.T) {
	css := `h2 { break-after: avoid; margin-bottom: 20pt } p { orphans: 3 }`
	got := linesPerPage(t, css, 4, "<h2>H01</h2><p>"+lines(6)+"</p>")
	if got["H01"] != 2 || got["L01"] != 2 {
		t.Errorf("heading on page %d, first line on page %d, want both on page 2", got["H01"], got["L01"])
	}
}

// A container counts its child blocks, and takes widows from its own style.
func TestWidowsOnList(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<ul style="widows: 3">`)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&sb, "<li>L%02d</li>", i)
	}
	sb.WriteString("</ul>")
	got := linesPerPage(t, "ul, li { margin: 0; padding: 0 }", 3, sb.String())
	if p1, p2 := split(got, 5); p1 != 2 || p2 != 3 {
		t.Errorf("items on pages 1/2 = %d/%d, want 2/3 (%v)", p1, p2, got)
	}
}

// Inherited widows and orphans reach every block, including ones formatted
// outside the paginator; none may break their formatting.
func TestWidowsOrphansEverywhere(t *testing.T) {
	css := `html { widows: 3; orphans: 3; font-family: serif }
.fl { float: left; width: 50pt }`
	html := `<p>intro<fn>note</fn></p>
<table><tr><td><p>cell</p></td><td>plain</td></tr></table>
<div class="fl"><p>float</p></div><p>` + lines(12) + `</p>
<ol><li>one</li><li>two</li></ol>`
	pages := renderHTMLPages(t, css, html)
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
}

// Text beside a block sits in an anonymous block, which inherits widows from
// its parent.
func TestWidowsOnAnonymousBlock(t *testing.T) {
	got := linesPerPage(t, "body { widows: 3 }", 3, lines(5))
	if p1, p2 := split(got, 5); p1 != 2 || p2 != 3 {
		t.Errorf("lines on pages 1/2 = %d/%d, want 2/3 (%v)", p1, p2, got)
	}
}

// The widows and orphans map must not keep a document's Texts reachable once
// OutputPagesFromText is done with them.
func TestFragLinesClearedAfterOutput(t *testing.T) {
	_, cb := renderHTMLPagesCB(t, "body { widows: 3; orphans: 3 }", "<p>"+lines(5)+"</p>")
	if n := len(cb.fragLines); n != 0 {
		t.Errorf("fragLines holds %d entries after OutputPagesFromText, want 0", n)
	}
}
