package htmlbag

import (
	"fmt"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// CreateVlist builds a vlist (a vertical list) from the Text object.
func (cb *CSSBuilder) CreateVlist(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	vl, err := cb.buildVlistInternal(te, wd)
	if err != nil {
		return nil, err
	}
	return vl, nil
}

func (cb *CSSBuilder) buildVlistInternal(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	// Get padding-left from this element to pass to children (for ul/ol lists)
	var paddingLeft bag.ScaledPoint
	if pl, ok := settings[frontend.SettingPaddingLeft]; ok {
		paddingLeft = pl.(bag.ScaledPoint)
	}

	if isBox, ok := settings[frontend.SettingBox]; ok && isBox.(bool) {
		vls := node.NewVList()
		vls.Attributes = node.H{"origin": "buildVListInternal"}

		// Track previous element's margin-bottom for margin collapsing
		var prevMarginBottom bag.ScaledPoint

		for i, itm := range te.Items {
			switch t := itm.(type) {
			case *frontend.Text:
				if dbg, ok := t.Settings[frontend.SettingDebug].(string); ok && dbg == "table" {
					return cb.buildTable(t, wd)
				}

				// Get margin-top of current element
				var curMarginTop bag.ScaledPoint
				if mt, ok := t.Settings[frontend.SettingMarginTop]; ok {
					curMarginTop = mt.(bag.ScaledPoint)
				}

				// Calculate collapsed margin (CSS margin collapsing)
				var marginGlue bag.ScaledPoint
				if i == 0 {
					// First element: use margin-top only
					marginGlue = curMarginTop
				} else {
					// Collapsed margin: max of previous bottom and current top
					marginGlue = bag.Max(prevMarginBottom, curMarginTop)
				}

				// Insert margin kern if needed
				if marginGlue > 0 {
					k := node.NewKern()
					k.Kern = marginGlue
					k.Attributes = node.H{"origin": "margin"}
					vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
					vls.Height += marginGlue
				}

				// Apply padding-left: reduce width for children and shift content
				childWidth := wd
				if paddingLeft > 0 {
					childWidth = wd - paddingLeft
				}
				vl, err := cb.buildVlistInternal(t, childWidth)
				if err != nil {
					return nil, err
				}

				// Shift content right by padding-left
				if paddingLeft > 0 {
					// Add kern at the beginning of each HList
					for cur := vl.List; cur != nil; cur = cur.Next() {
						if hl, ok := cur.(*node.HList); ok {
							k := node.NewKern()
							k.Kern = paddingLeft
							k.Attributes = node.H{"origin": "padding-left"}
							hl.List = node.InsertBefore(hl.List, hl.List, k)
							hl.Width += paddingLeft
						}
					}
				}
				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), vl)
				if vl.Width > vls.Width {
					vls.Width = vl.Width
				}
				vls.Height += vl.Height
				vls.Depth = vl.Depth

				// Store margin-bottom for next iteration
				if mb, ok := t.Settings[frontend.SettingMarginBottom]; ok {
					prevMarginBottom = mb.(bag.ScaledPoint)
				} else {
					prevMarginBottom = 0
				}
			case string:
				fmt.Println("~~> string")
			default:
				fmt.Println("~~> bvi unknown", t)
			}
		}

		// Add final margin-bottom after last element
		if prevMarginBottom > 0 {
			k := node.NewKern()
			k.Kern = prevMarginBottom
			k.Attributes = node.H{"origin": "margin-bottom"}
			vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
			vls.Height += prevMarginBottom
		}

		return vls, nil
	}

	// FormatParagraph -> Mknodes handles SettingPrepend (e.g., bullet points)
	vl, _, err := cb.frontend.FormatParagraph(te, wd)
	if err != nil {
		return nil, err
	}

	return vl, nil
}
