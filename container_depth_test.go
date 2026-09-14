package htmlbag

import (
	"bytes"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestContainerHeightIncludesChildDepth guards the vpack invariant of the
// box-container branch in buildVlistInternal: a container VList must report
// Height+Depth equal to the summed vertical extent of its children, with only
// the last child's depth left in Depth. Under the half-leading model every
// paragraph carries L/2 plus the font depth in its Depth; dropping the depth
// of interior children made everything following a nested container
// (blockquote > ul > li) start one depth per nesting level too high, so the
// next block overlapped the container's last line.
func TestContainerHeightIncludesChildDepth(t *testing.T) {
	fe, err := frontend.NewForWriter(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadIncludedFonts(fe); err != nil {
		t.Fatal(err)
	}
	cb, err := New(fe, NewCSSParserWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if err = cb.InitPage(); err != nil {
		t.Fatal(err)
	}
	te, err := cb.HTMLToText(`<!DOCTYPE html><html><body>
<blockquote>
<p><strong>Important:</strong></p>
<ul>
<li>a short first item</li>
<li>a considerably longer second item that certainly wraps onto several lines at the narrow width used by this test</li>
</ul>
</blockquote>
<p>the following paragraph must not overlap the blockquote</p>
</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := cb.CreateVlist(te, bag.MustSP("150pt"))
	if err != nil {
		t.Fatal(err)
	}

	containers := 0
	var walk func(v *node.VList)
	walk = func(v *node.VList) {
		if origin, _ := v.Attributes["origin"].(string); origin == "buildVListInternal" {
			containers++
			var sum, lastDepth bag.ScaledPoint
			for e := v.List; e != nil; e = e.Next() {
				sum += vlistNodeHeight(e)
				lastDepth = 0
				switch c := e.(type) {
				case *node.VList:
					lastDepth = c.Depth
				case *node.HList:
					lastDepth = c.Depth
				}
			}
			if got := v.Height + v.Depth; got != sum {
				t.Errorf("container %q: Height+Depth = %s, children sum to %s", v.Attributes["origin"], got, sum)
			}
			if v.Depth != lastDepth {
				t.Errorf("container depth is %s, want last child's depth %s", v.Depth, lastDepth)
			}
		}
		for e := v.List; e != nil; e = e.Next() {
			if c, ok := e.(*node.VList); ok {
				walk(c)
			}
		}
	}
	walk(vl)
	if containers < 3 {
		t.Fatalf("expected at least body, blockquote and ul containers, walked %d", containers)
	}
}
