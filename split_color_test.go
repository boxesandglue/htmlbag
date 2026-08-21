package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
)

// pageEmitsColor reports whether any StartStop node on the page emits a
// non-black non-stroking fill colour. A fragment that inherits the page's
// default graphics state without re-stating the colour renders black, which
// is the bug this guards.
func pageEmitsColor(objects []node.Node) bool {
	var walk func(n node.Node) bool
	walk = func(n node.Node) bool {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.StartStop:
				if v.ShipoutCallback == nil {
					continue
				}
				out := v.ShipoutCallback(v)
				// Ignore the reset instruction, which is black by design.
				if strings.Contains(out, "rg") && !strings.HasPrefix(strings.TrimSpace(out), "0 0 0 RG") {
					return true
				}
			case *node.HList:
				if walk(v.List) {
					return true
				}
			case *node.VList:
				if walk(v.List) {
					return true
				}
			}
		}
		return false
	}
	for _, o := range objects {
		if walk(o) {
			return true
		}
	}
	return false
}

// A coloured paragraph that breaks across pages must keep its colour on the
// continuation pages: the colour instruction lives inside the paragraph's
// first line, and a page's content stream starts out black.
func TestSplitParagraphKeepsColorOnLaterPages(t *testing.T) {
	css := `@page { size: a5; margin: 1cm }
body { font-family: serif; font-size: 11pt; color: red }`
	long := strings.Repeat("Farbiger Fließtext, der über mehrere Seiten laufen muss. ", 120)
	pages := renderHTMLPages(t, css, "<p>"+long+"</p>")
	if len(pages) < 2 {
		t.Fatalf("paragraph produced %d page(s), want at least 2", len(pages))
	}
	for i, pg := range pages {
		var objs []node.Node
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				objs = append(objs, obj.Vlist)
			}
		}
		if !pageEmitsColor(objs) {
			t.Errorf("page %d of the split paragraph emits no colour instruction (renders black)", i+1)
		}
	}
}
