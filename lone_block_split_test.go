package htmlbag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// A bordered block that is the body's only child splits across pages like
// any other (CSS Fragmentation 3, box-decoration-break: slice), issue #35.
func TestLoneBorderedBlockSplits(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "<p>Line %d</p>", i)
	}
	css := charCSS + ".box { border: 1pt solid black }"
	for _, c := range []struct{ name, before string }{
		{"only child", ""},
		{"after a paragraph", "<p>Before</p>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages := renderHTMLPages(t, css, c.before+`<div class="box">`+sb.String()+`</div>`)
			if len(pages) != 2 {
				t.Errorf("%d pages, want 2", len(pages))
			}
			bottom := charTop - bag.MustSP("160pt")
			next, onFirst := 1, 0
			for _, l := range boxedLines(pages) {
				if !strings.HasPrefix(l.text, "Line") {
					continue
				}
				if want := fmt.Sprintf("Line%d", next); l.text != want {
					t.Fatalf("line %q, want %q", l.text, want)
				}
				if l.bottom < bottom {
					t.Errorf("%s on page %d ends at %s, below the content area at %s", l.text, l.page, l.bottom, bottom)
				}
				if l.page == 1 {
					onFirst++
				}
				next++
			}
			if next != 21 {
				t.Errorf("placed %d lines, want 20", next-1)
			}
			if onFirst == 0 {
				t.Error("page 1 holds none of the box's lines")
			}
		})
	}
}

// boxedLines is placedLines that also looks inside the hlists a border wraps
// around its content.
func boxedLines(pages []*document.Page) []placedLine {
	var out []placedLine
	var walk func(n node.Node, y bag.ScaledPoint, pg int)
	walk = func(n node.Node, y bag.ScaledPoint, pg int) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.VList:
				walk(v.List, y, pg)
				y -= v.Height + v.Depth
			case *node.HList:
				nested := false
				for c := v.List; c != nil; c = c.Next() {
					if vl, ok := c.(*node.VList); ok {
						nested = true
						walk(vl.List, y, pg)
					}
				}
				if !nested {
					var sb strings.Builder
					collectGlyphs(v.List, &sb)
					if sb.Len() > 0 {
						out = append(out, placedLine{pg, y, y - v.Height - v.Depth, v.Width, sb.String()})
					}
				}
				y -= v.Height + v.Depth
			case *node.Kern:
				y -= v.Kern
			case *node.Glue:
				y -= v.Width
			}
		}
	}
	for i, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist, obj.Y, i+1)
			}
		}
	}
	return out
}
