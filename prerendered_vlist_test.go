package htmlbag

import (
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// cellSequence returns a cell's glyph text in reading order, with each
// pre-rendered VList written as its origin in brackets.
func cellSequence(n node.Node, out *strings.Builder) {
	for ; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.Glyph:
			out.WriteString(t.Components)
		case *node.HList:
			cellSequence(t.List, out)
		case *node.VList:
			if o, ok := t.Attributes["origin"].(string); ok && strings.HasPrefix(o, "pre-") {
				out.WriteString("[" + o + "]")
				continue
			}
			cellSequence(t.List, out)
		}
	}
}

func prerendered(origin string) *node.VList {
	r := node.NewRule()
	r.Width, r.Height = bag.MustSP("40pt"), bag.MustSP("10pt")
	vl := node.Vpack(r)
	vl.Attributes = node.H{"origin": origin}
	return vl
}

// A block with data-vlist-id among a cell's contents is set as that
// pre-rendered VList where it stands, empty or not; the td's own
// data-vlist-id is set above them. Every one is kept, in order.
func TestPrerenderedVListsKeepTheirPlaceInACell(t *testing.T) {
	for _, tc := range []struct {
		name, cell, want string
	}{
		{"on the cell", `<td data-vlist-id="pre-1">A</td>`, "[pre-1]A"},
		{"between paragraphs", `<td><p>A</p><div data-vlist-id="pre-1">&#8203;</div><p>B</p></td>`, "A[pre-1]B"},
		{"two", `<td><p>A</p><div data-vlist-id="pre-1">&#8203;</div><p>B</p><div data-vlist-id="pre-2">&#8203;</div></td>`, "A[pre-1]B[pre-2]"},
		{"empty", `<td><p>A</p><div data-vlist-id="pre-1"></div><p>B</p></td>`, "A[pre-1]B"},
		{"in a th", `<th><div data-vlist-id="pre-1">&#8203;</div><p>A</p></th>`, "[pre-1]A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cb := floatBuilder(t)
			cb.PendingVLists["pre-1"] = prerendered("pre-1")
			cb.PendingVLists["pre-2"] = prerendered("pre-2")
			vl := buildHTML(t, cb, `<table><tr>`+tc.cell+`</tr></table>`)
			var sb strings.Builder
			cellSequence(vl.List, &sb)
			if got := sb.String(); got != tc.want {
				t.Errorf("cell reads %q, want %q", got, tc.want)
			}
		})
	}
}
