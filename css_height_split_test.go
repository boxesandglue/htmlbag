package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
)

// flowDepth is how far below top, the content area's top edge, the lowest
// box on pg reaches.
func flowDepth(pg *document.Page, top bag.ScaledPoint) bag.ScaledPoint {
	var d bag.ScaledPoint
	for _, obj := range pg.Objects {
		if obj.Vlist != nil && obj.Vlist.List != nil {
			d = max(d, top-(obj.Y-obj.Vlist.Height-obj.Vlist.Depth))
		}
	}
	return d
}

// A block whose height is taller than the room left is split at the page
// end like the space it stands for: the page is filled, the rest of the
// height goes on at the top of the next page, and what follows the block
// comes right below it there (#79).
func TestACSSHeightSplitsAtThePageEnd(t *testing.T) {
	const css = `@page { size: 240pt 200pt; margin: 16pt; }
body { font-size: 8pt; line-height: 1.3; }
p { margin: 0; }
.tall { height: 200pt; }
.bg { background-color: #d24; }
.border { border: 1pt solid black; }
.card { border: 1pt solid black; padding: 4pt; }`
	full, top := bag.MustSP("168pt"), bag.MustSP("184pt")
	for _, c := range []struct{ name, block string }{
		{"plain", `<div class="tall">TALL</div>`},
		{"background", `<div class="tall bg">TALL</div>`},
		{"border", `<div class="tall border">TALL</div>`},
		{"paragraph", `<p class="tall bg">TALL</p>`},
		{"in a card", `<div class="card"><p>Before.</p><div class="tall bg">TALL</div></div>`},
		{"after a paragraph", `<p>Before.</p><div class="tall bg">TALL</div>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages, _ := renderHTMLPagesCB(t, css, `<html><body>`+c.block+`<p>After the tall block.</p></body></html>`)
			var texts []string
			for _, pg := range pages {
				texts = append(texts, pageText(pg))
			}
			if len(pages) != 2 || !strings.Contains(texts[1], "Afterthetallblock.") {
				t.Fatalf("pages %q, want two with the paragraph after the block on the second", texts)
			}
			if h := flowDepth(pages[0], top); h < full-bag.MustSP("0.5pt") || h > full {
				t.Errorf("page 1 reaches %s down, want the content area, %s", h, full)
			}
			if h := flowDepth(pages[1], top); h > full {
				t.Errorf("page 2 reaches %s down, below the content area", h)
			}
		})
	}
}

// The same in FlowText regions: the first region is filled, the rest of the
// height is at the top of the next, with the paragraph after it below.
func TestFlowTextCSSHeightSplitsAtTheRegionEnd(t *testing.T) {
	for _, c := range []struct{ name, block string }{
		{"plain", `<div style="height: 100pt">T</div>`},
		{"background", `<div style="height: 100pt; background-color: #d24">T</div>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			cb, _ := newFlowBuilder(t, "")
			tr := flow(t, cb, c.block+`<p>After</p>`, wide("60pt"), wide("1000pt"))
			if len(tr.filled) != 2 {
				t.Fatalf("filled %d regions, want 2", len(tr.filled))
			}
			if got := tr.filled[0].Used; got != sp("60pt") {
				t.Errorf("region 1 used %s, want 60pt", got)
			}
			lines := boxLines(tr.filled[1])
			if len(lines) != 1 || !strings.Contains(lines[0].text, "After") || lines[0].top != sp("40pt") {
				t.Errorf("region 2 lines %+v, want After at 40pt, below the rest of the height", lines)
			}
		})
	}
}
