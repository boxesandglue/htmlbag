package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// A block with an explicit width is that wide (CSS 2.1 §10.3.3), however
// narrow its children are. Without a border or a background it took its
// widest child's width, which leaves out the child's margins.
func TestExplicitWidthIsTheBoxWidth(t *testing.T) {
	for _, p := range []string{
		`<p style="margin:0 60pt 0 40pt">Indented</p>`,
		`<p>Plain</p>`,
	} {
		vl := buildHTML(t, floatBuilder(t), `<div style="width:300pt">`+p+`</div>`)
		var box *node.VList
		for n := vl.List; n != nil && box == nil; n = n.Next() {
			box, _ = n.(*node.VList)
		}
		if box == nil {
			t.Fatalf("%s: no box for the div", p)
		}
		if want := bag.MustSP("300pt"); box.Width != want {
			t.Errorf("%s: the div is %s wide, want %s", p, box.Width, want)
		}
	}
}

// width: auto is the initial value, so declaring it keeps the width a box has
// without it: a float as wide as its widest child, a block as wide as before.
func TestWidthAutoChangesNothing(t *testing.T) {
	var float func(n node.Node) *node.VList
	float = func(n node.Node) *node.VList {
		for e := n; e != nil; e = e.Next() {
			switch c := e.(type) {
			case *node.VList:
				if origin, _ := c.Attributes["origin"].(string); origin == "float" {
					return c
				}
				if f := float(c.List); f != nil {
					return f
				}
			case *node.HList:
				if f := float(c.List); f != nil {
					return f
				}
			}
		}
		return nil
	}
	floatWidth := func(style string) bag.ScaledPoint {
		vl := buildHTML(t, floatBuilder(t), `<div style="float:left`+style+`"><p style="margin:0 10pt">ab</p></div><p>beside the float</p>`)
		f := float(vl.List)
		if f == nil {
			t.Fatalf("%q: no float", style)
		}
		return f.Width
	}
	if a, b := floatWidth(";width:auto"), floatWidth(""); a != b {
		t.Errorf("a float with width:auto is %s wide, without width %s", a, b)
	}

	blockWidth := func(style string) bag.ScaledPoint {
		vl := buildHTML(t, floatBuilder(t), `<div style="`+style+`"><p style="margin:0 60pt 0 40pt">Indented</p></div>`)
		for n := vl.List; n != nil; n = n.Next() {
			if box, ok := n.(*node.VList); ok {
				return box.Width
			}
		}
		t.Fatalf("%q: no box for the div", style)
		return 0
	}
	if a, b := blockWidth("width:auto"), blockWidth(""); a != b {
		t.Errorf("a block with width:auto is %s wide, without width %s", a, b)
	}
}
