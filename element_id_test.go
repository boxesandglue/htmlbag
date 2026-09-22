package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// elementIDs counts the id attributes of the lists on the pages.
func elementIDs(pages []*document.Page) map[string]int {
	ids := map[string]int{}
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for ; n != nil; n = n.Next() {
			switch v := n.(type) {
			case *node.HList:
				if id, ok := v.Attributes["id"].(string); ok {
					ids[id]++
				}
				walk(v.List)
			case *node.VList:
				if id, ok := v.Attributes["id"].(string); ok {
					ids[id]++
				}
				walk(v.List)
			}
		}
	}
	for _, pg := range pages {
		for _, obj := range pg.Objects {
			if obj.Vlist != nil {
				walk(obj.Vlist)
			}
		}
	}
	return ids
}

// TestElementIDReachesTheBox checks that an element's id is carried onto the
// box it builds, and onto nothing inside it, so the id can be found again in
// the laid-out page.
func TestElementIDReachesTheBox(t *testing.T) {
	html := `<p id="para">before</p>
<div id="outer"><p>a child of the div</p><p id="inner">its own id</p></div>
<table><tr id="row"><td id="cell"><p>in the cell</p></td><td>two</td></tr></table>`
	ids := elementIDs(renderHTMLPages(t, borderModelCSS, html))
	// A div's own box is not what this is about, but its id must not
	// reach the paragraphs inside it.
	if ids["outer"] > 1 {
		t.Errorf(`id "outer" is on %d boxes, want at most the div's own`, ids["outer"])
	}
	delete(ids, "outer")
	want := []string{"para", "inner", "row", "cell"}
	if len(ids) != len(want) {
		t.Errorf("ids = %v, want each of %v once", ids, want)
	}
	for _, w := range want {
		if ids[w] != 1 {
			t.Errorf("id %q is on %d boxes, want 1 (ids %v)", w, ids[w], ids)
		}
	}
}
