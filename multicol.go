package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
)

// multicol holds the CSS Multi-column Layout properties of an element. None
// of them is inherited. They are read, but not laid out yet: a document with
// column-count set is set in one column, with a warning.
type multicol struct {
	// count is column-count, 0 for auto.
	count int
	// gap is column-gap where gapSet, 1em of the element's font otherwise
	// (normal).
	gap    bag.ScaledPoint
	gapSet bool
	// spanAll is column-span: all.
	spanAll bool
	// fillAuto is column-fill: auto; the initial balance is false.
	fillAuto bool
	// ruleWidth, ruleSolid and ruleColor are the column-rule longhands. As
	// with borders, solid is the only style drawn.
	ruleWidth bag.ScaledPoint
	ruleSolid bool
	ruleColor *color.Color
}
