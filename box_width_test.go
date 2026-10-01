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
