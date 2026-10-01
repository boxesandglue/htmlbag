package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/node"
)

// hasOrigin reports whether a node with the given origin attribute is in list.
func hasOrigin(list node.Node, origin string) bool {
	for n := list; n != nil; n = n.Next() {
		if o, _ := n.GetAttribute("origin"); o == origin {
			return true
		}
		switch t := n.(type) {
		case *node.VList:
			if hasOrigin(t.List, origin) {
				return true
			}
		case *node.HList:
			if hasOrigin(t.List, origin) {
				return true
			}
		}
	}
	return false
}

// A paragraph in a table cell draws its own background and borders, as it
// does on the page. The cell set a plain paragraph as text, which draws
// neither.
func TestCellParagraphDrawsItsBox(t *testing.T) {
	cell := func(p string) string {
		return `<table style="width:200pt"><tr><td>` + p + `</td></tr></table>`
	}
	for _, tc := range []struct{ name, p, origin string }{
		{"background", `<p style="background-color:yellow">Shaded</p>`, "html background color"},
		{"border", `<p style="border:1pt solid black">Boxed</p>`, "html border + clipping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if vl := buildHTML(t, floatBuilder(t), cell(tc.p)); !hasOrigin(vl.List, tc.origin) {
				t.Errorf("no %q in the cell", tc.origin)
			}
		})
	}
	// The cell draws its own background; its bare text does not again.
	vl := buildHTML(t, floatBuilder(t), `<table><tr><td style="background-color:yellow">Bare</td></tr></table>`)
	if hasOrigin(vl.List, "html background color") {
		t.Error("the cell's background is drawn a second time behind its text")
	}
}
