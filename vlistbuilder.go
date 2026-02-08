package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
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

// isWhitespaceOnly returns true if the Text element contains only whitespace strings.
func isWhitespaceOnly(te *frontend.Text) bool {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return false
			}
		case *frontend.Text:
			if !isWhitespaceOnly(t) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (cb *CSSBuilder) buildVlistInternal(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	// Get padding-left from this element to pass to children (for ul/ol lists)
	var paddingLeft bag.ScaledPoint
	if pl, ok := settings[frontend.SettingPaddingLeft]; ok {
		paddingLeft = pl.(bag.ScaledPoint)
	}

	if isBox, ok := settings[frontend.SettingBox]; ok && isBox.(bool) {
		// If this box container has a prepend (e.g., list bullet), pass it
		// to the first child Text element so FormatParagraph can render it.
		if prep, ok := settings[frontend.SettingPrepend]; ok {
			for _, itm := range te.Items {
				if t, ok := itm.(*frontend.Text); ok {
					t.Settings[frontend.SettingPrepend] = prep
					break
				}
			}
		}

		// Extract border/padding values for this container
		hv := settingsToHTMLValues(settings)
		hasBorderOrBg := hv.hasBorder() || hv.BackgroundColor != nil

		// Calculate effective width for children
		childBaseWidth := wd
		if hasBorderOrBg {
			// HTMLBorder will handle all padding and borders visually
			childBaseWidth = wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight
		}

		vls := node.NewVList()
		vls.Attributes = node.H{"origin": "buildVListInternal"}

		// Track previous element's margin-bottom for margin collapsing
		var prevMarginBottom bag.ScaledPoint

		for i, itm := range te.Items {
			switch t := itm.(type) {
			case *frontend.Text:
				// Skip whitespace-only text elements (e.g. whitespace
				// between </ul> and </li> in the HTML tree).
				if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
					continue
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

				var vl *node.VList
				if dbg, ok := t.Settings[frontend.SettingDebug].(string); ok && dbg == "table" {
					var err error
					vl, err = cb.buildTable(t, wd)
					if err != nil {
						return nil, err
					}
				} else {
					// Reduce width for children by padding-left (for lists).
					// When the container has borders/background, HTMLBorder
					// handles all padding, so skip the per-child shift.
					childWidth := childBaseWidth
					if !hasBorderOrBg && paddingLeft > 0 {
						childWidth = childBaseWidth - paddingLeft
					}
					var err error
					vl, err = cb.buildVlistInternal(t, childWidth)
					if err != nil {
						return nil, err
					}

					// Shift content right by padding-left (only for lists
					// without borders — HTMLBorder handles all other cases)
					if !hasBorderOrBg && paddingLeft > 0 {
						for cur := vl.List; cur != nil; cur = cur.Next() {
							switch n := cur.(type) {
							case *node.HList:
								k := node.NewKern()
								k.Kern = paddingLeft
								k.Attributes = node.H{"origin": "padding-left"}
								n.List = node.InsertBefore(n.List, n.List, k)
								n.Width += paddingLeft
							case *node.VList:
								// Box container child (e.g. li with nested ul):
								// shift the entire VList right.
								n.ShiftX += paddingLeft
							}
						}
					}
				}
				// Propagate page-break-after to node attributes
				if pba, ok := t.Settings[frontend.SettingPageBreakAfter]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakAfter"] = pba
				}
				if pbb, ok := t.Settings[frontend.SettingPageBreakBefore]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakBefore"] = pbb
				}

				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), vl)
				if vl.Width > vls.Width {
					vls.Width = vl.Width
				}
				vls.Height += vl.Height
				vls.Depth = vl.Depth

				if cb.ElementCallback != nil {
					if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
						cb.ElementCallback(ElementEvent{
							TagName:     tag,
							TextContent: extractTextContent(t),
							VList:       vl,
						})
					}
				}

				// Annotate heading VLists so OutputPages can assign page numbers.
				if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
					switch tag {
					case "h1", "h2", "h3", "h4", "h5", "h6":
						if vl.Attributes == nil {
							vl.Attributes = node.H{}
						}
						vl.Attributes["_heading_idx"] = cb.headingCount
						cb.Headings = append(cb.Headings, HeadingEntry{Level: tag, Text: extractTextContent(t)})
						cb.headingCount++
					}
				}

				// Store margin-bottom for next iteration
				if mb, ok := t.Settings[frontend.SettingMarginBottom]; ok {
					prevMarginBottom = mb.(bag.ScaledPoint)
				} else {
					prevMarginBottom = 0
				}
			}
		}

		// Handle final margin-bottom after last element.
		if prevMarginBottom > 0 {
			if hasBorderOrBg {
				// Border/padding blocks margin collapsing: add kern.
				k := node.NewKern()
				k.Kern = prevMarginBottom
				k.Attributes = node.H{"origin": "margin-bottom"}
				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
				vls.Height += prevMarginBottom
			} else {
				// No border/padding: the last child's margin-bottom
				// collapses through the parent boundary (CSS margin
				// collapsing). Propagate the maximum to the parent.
				if mb, ok := te.Settings[frontend.SettingMarginBottom]; ok {
					parentMB := mb.(bag.ScaledPoint)
					if prevMarginBottom > parentMB {
						te.Settings[frontend.SettingMarginBottom] = prevMarginBottom
					}
				} else {
					te.Settings[frontend.SettingMarginBottom] = prevMarginBottom
				}
			}
		}

		// Apply borders/background to this block container
		if hasBorderOrBg {
			vls.Width = childBaseWidth
			vls = cb.HTMLBorder(vls, hv)
		}

		return vls, nil
	}

	// Extract border/padding values first to calculate content width
	hv := settingsToHTMLValues(settings)

	// Reduce width by border and padding (CSS box-sizing: border-box behavior)
	contentWidth := wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight

	// FormatParagraph -> Mknodes handles SettingPrepend (e.g., bullet points)
	vl, _, err := cb.frontend.FormatParagraph(te, contentWidth)
	if err != nil {
		return nil, err
	}

	// Apply borders if any are defined
	if hv.hasBorder() || hv.BackgroundColor != nil {
		vl = cb.HTMLBorder(vl, hv)
	}

	return vl, nil
}

// extractTextContent recursively collects string content from a Text tree.
func extractTextContent(te *frontend.Text) string {
	var b strings.Builder
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			b.WriteString(t)
		case *frontend.Text:
			b.WriteString(extractTextContent(t))
		}
	}
	return b.String()
}

// settingsToHTMLValues extracts border/padding/background settings into HTMLValues.
func settingsToHTMLValues(settings frontend.TypesettingSettings) HTMLValues {
	hv := HTMLValues{}

	if v, ok := settings[frontend.SettingBackgroundColor]; ok && v != nil {
		hv.BackgroundColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderTopWidth]; ok && v != nil {
		hv.BorderTopWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderRightWidth]; ok && v != nil {
		hv.BorderRightWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomWidth]; ok && v != nil {
		hv.BorderBottomWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderLeftWidth]; ok && v != nil {
		hv.BorderLeftWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderTopColor]; ok && v != nil {
		hv.BorderTopColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderRightColor]; ok && v != nil {
		hv.BorderRightColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderBottomColor]; ok && v != nil {
		hv.BorderBottomColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderLeftColor]; ok && v != nil {
		hv.BorderLeftColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderTopStyle]; ok && v != nil {
		hv.BorderTopStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderRightStyle]; ok && v != nil {
		hv.BorderRightStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderBottomStyle]; ok && v != nil {
		hv.BorderBottomStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderLeftStyle]; ok && v != nil {
		hv.BorderLeftStyle = v.(frontend.BorderStyle)
	}
	if v, ok := settings[frontend.SettingBorderTopLeftRadius]; ok && v != nil {
		hv.BorderTopLeftRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderTopRightRadius]; ok && v != nil {
		hv.BorderTopRightRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomLeftRadius]; ok && v != nil {
		hv.BorderBottomLeftRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomRightRadius]; ok && v != nil {
		hv.BorderBottomRightRadius = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingTop]; ok && v != nil {
		hv.PaddingTop = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingRight]; ok && v != nil {
		hv.PaddingRight = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingBottom]; ok && v != nil {
		hv.PaddingBottom = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingLeft]; ok && v != nil {
		hv.PaddingLeft = v.(bag.ScaledPoint)
	}

	return hv
}
