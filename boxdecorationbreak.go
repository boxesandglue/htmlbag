package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// attrDecorationClone marks a splittable block with box-decoration-break:
// clone (CSS Fragmentation 3 §5.4): each fragment keeps the padding and border
// on both of its sides. Its value is a clonePadding.
const attrDecorationClone = "_decorationClone"

// clonePadding is the vertical padding of a block without a border or
// background, which every fragment repeats as kerns around its children. A
// block with a border or background repeats the sides of its _splittableHv.
type clonePadding struct{ top, bottom bag.ScaledPoint }

// cloneDecoration marks vl, a splittable block built from te, when te has
// box-decoration-break: clone, and returns the children its fragments are
// cut from. A block without a border or background (bare) has its vertical
// padding as kerns among the children; they are left out, as buildFragment
// puts them around every fragment.
func (cb *CSSBuilder) cloneDecoration(vl *node.VList, te *frontend.Text, children []node.Node, bare bool, hv HTMLValues) []node.Node {
	if !cb.clones[te] {
		return children
	}
	vl.SetAttribute(attrDecorationClone, clonePadding{})
	if !bare {
		return children
	}
	inner := children
	if len(inner) > 0 && hasOriginKern(inner[0], "padding-top") {
		inner = inner[1:]
	}
	if len(inner) > 0 && hasOriginKern(inner[len(inner)-1], "padding-bottom") {
		inner = inner[:len(inner)-1]
	}
	if len(inner) == 0 {
		delete(vl.Attributes, attrDecorationClone)
		return children
	}
	vl.SetAttribute(attrDecorationClone, clonePadding{hv.PaddingTop, hv.PaddingBottom})
	return inner
}

// hasOriginKern reports whether n is a kern of the given origin.
func hasOriginKern(n node.Node, origin string) bool {
	k, ok := n.(*node.Kern)
	if !ok || k.Attributes == nil {
		return false
	}
	o, _ := k.Attributes["origin"].(string)
	return o == origin
}

// decorationClone returns the heights a fragment of the split block vl adds
// above and below its children when it has box-decoration-break: clone, and
// whether it has.
func decorationClone(vl *node.VList) (top, bottom bag.ScaledPoint, ok bool) {
	if vl.Attributes == nil {
		return 0, 0, false
	}
	pad, ok := vl.Attributes[attrDecorationClone].(clonePadding)
	if !ok {
		return 0, 0, false
	}
	if hv, _ := vl.Attributes["_splittableHv"].(HTMLValues); hv.hasBorder() || hv.BackgroundColor != nil {
		return hv.PaddingTop + hv.BorderTopWidth, hv.PaddingBottom + hv.BorderBottomWidth, true
	}
	return pad.top, pad.bottom, true
}

// padFragment puts the padding kerns of a bare block with box-decoration-break:
// clone around the items of one of its fragments.
func padFragment(items []node.Node, pad clonePadding) []node.Node {
	if pad.top > 0 {
		k := node.NewKern()
		k.Kern = pad.top
		k.Attributes = node.H{"origin": "padding-top"}
		items = append([]node.Node{k}, items...)
	}
	if pad.bottom > 0 {
		k := node.NewKern()
		k.Kern = pad.bottom
		k.Attributes = node.H{"origin": "padding-bottom"}
		items = append(items, k)
	}
	return items
}
