package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// evaluateContent turns parsed CSS content tokens into a displayable string.
// counters maps counter names (e.g. "page", "pages") to their current values.
func evaluateContent(tokens []csshtml.ContentToken, counters map[string]int) string {
	var sb strings.Builder
	for _, tok := range tokens {
		switch tok.Type {
		case csshtml.ContentString:
			sb.WriteString(tok.Value)
		case csshtml.ContentCounter:
			if v, ok := counters[tok.Value]; ok {
				sb.WriteString(strconv.Itoa(v))
			}
		}
	}
	return sb.String()
}

// BeforeShipout should be called when placing a CSS page in the PDF. It adds
// page margin boxes to the current page.
func (cb *CSSBuilder) BeforeShipout() error {
	var err error
	df := cb.frontend
	dimensions := cb.currentPageDimensions
	mp := dimensions.masterpage
	if mp != nil {
		pageMarginBoxes := make(map[string]*pageMarginBox)
		for areaName, attr := range mp.PageArea {
			pmb := &pageMarginBox{
				widthAuto: true,
			}
			pmb.hasContents = hasContents(attr, mp.PageAreaContent[areaName])
			if wd, ok := attr["width"]; ok {
				if wd != "auto" {
					pmb.areaWidth = ParseRelativeSize(wd, dimensions.Width, dimensions.Width)
				}
			}

			pageMarginBoxes[areaName] = pmb
		}
		for areaName := range mp.PageArea {
			pmb := pageMarginBoxes[areaName]
			switch areaName {
			case "top-left-corner":
				pmb.x = 0
				pmb.y = df.Doc.DefaultPageHeight
				pmb.wd = dimensions.MarginLeft
				pmb.ht = dimensions.MarginTop
			case "top-right-corner":
				pmb.x = dimensions.Width - dimensions.MarginRight
				pmb.y = df.Doc.DefaultPageHeight
				pmb.wd = dimensions.MarginRight
				pmb.ht = dimensions.MarginTop
			case "bottom-left-corner":
				pmb.x = 0
				pmb.y = dimensions.MarginBottom
				pmb.wd = dimensions.MarginLeft
				pmb.ht = dimensions.MarginBottom
			case "bottom-right-corner":
				pmb.x = dimensions.Width - dimensions.MarginRight
				pmb.y = dimensions.MarginBottom
				pmb.wd = dimensions.MarginRight
				pmb.ht = dimensions.MarginBottom
			case "top-left", "top-center", "top-right":
				pmb.x = dimensions.MarginLeft
				pmb.y = df.Doc.DefaultPageHeight
				pmb.wd = dimensions.Width - dimensions.MarginLeft - dimensions.MarginRight
				pmb.ht = dimensions.MarginTop
				switch areaName {
				case "top-left":
					pmb.halign = frontend.HAlignLeft
				case "top-center":
					pmb.halign = frontend.HAlignCenter
				case "top-right":
					pmb.halign = frontend.HAlignRight
				}
			case "bottom-left", "bottom-center", "bottom-right":
				pmb.x = dimensions.MarginLeft
				pmb.y = dimensions.MarginBottom
				pmb.wd = dimensions.Width - dimensions.MarginLeft - dimensions.MarginRight
				pmb.ht = dimensions.MarginBottom
				switch areaName {
				case "bottom-left":
					pmb.halign = frontend.HAlignLeft
				case "bottom-center":
					pmb.halign = frontend.HAlignCenter
				case "bottom-right":
					pmb.halign = frontend.HAlignRight
				}
			}
		}
		// todo: calculate the area size
		for _, areaName := range []string{"top-left-corner", "top-left", "top-center", "top-right", "top-right-corner", "right-top", "right-middle", "right-bottom", "bottom-right-corner", "bottom-right", "bottom-center", "bottom-left", "bottom-left-corner", "left-bottom", "left-middle", "left-top"} {
			if area, ok := mp.PageArea[areaName]; ok {
				contentTokens := mp.PageAreaContent[areaName]
				if !hasContents(area, contentTokens) {
					continue
				}
				styles := cb.stylesStack.PushStyles()

				if err = StylesToStyles(styles, area, cb.frontend, cb.stylesStack.CurrentStyle().Fontsize); err != nil {
					return err
				}
				pmb := pageMarginBoxes[areaName]

				// Apply margin to shrink and offset the margin box area.
				pmb.x += styles.marginLeft
				pmb.wd -= styles.marginLeft + styles.marginRight
				pmb.ht -= styles.marginTop + styles.marginBottom
				// Adjust vertical position: for top areas margin-top pushes
				// content down, for bottom areas it also pushes content down
				// (away from the content area boundary).
				if strings.HasPrefix(areaName, "top") {
					pmb.y -= styles.marginTop
				} else if strings.HasPrefix(areaName, "bottom") {
					pmb.y -= styles.marginTop
				}

				vl := node.NewVList()
				var err error
				cb.Counters["page"] = len(cb.frontend.Doc.Pages)
				c := evaluateContent(contentTokens, cb.Counters)
				if c != "" {
					txt := frontend.NewText()
					ApplySettings(txt.Settings, styles)
					if styles.Fontsize > 0 {
						txt.Settings[frontend.SettingSize] = styles.Fontsize
					} else if styles.DefaultFontSize > 0 {
						txt.Settings[frontend.SettingSize] = styles.DefaultFontSize
					}
					txt.Settings[frontend.SettingHeight] = pmb.ht - styles.BorderTopWidth - styles.BorderBottomWidth
					txt.Settings[frontend.SettingVAlign] = styles.Valign

					txt.Items = append(txt.Items, c)
					defaultFontFamily := styles.DefaultFontFamily
					if defaultFontFamily == nil {
						defaultFontFamily = styles.fontfamily
					}
					if defaultFontFamily == nil {
						defaultFontFamily = df.FindFontFamily("serif")
					}
					vl, _, err = df.FormatParagraph(txt, pmb.wd-styles.BorderLeftWidth-styles.BorderRightWidth, frontend.Family(defaultFontFamily), frontend.HorizontalAlign(pmb.halign))
					if err != nil {
						return err
					}

				} else {
					vl = node.NewVList()
					vl.Width = pmb.wd - styles.BorderLeftWidth - styles.BorderRightWidth
					vl.Height = pmb.ht - styles.BorderTopWidth - styles.BorderBottomWidth
				}
				hv := HTMLValues{
					BorderLeftWidth:         styles.BorderLeftWidth,
					BorderRightWidth:        styles.BorderRightWidth,
					BorderTopWidth:          styles.BorderTopWidth,
					BorderBottomWidth:       styles.BorderBottomWidth,
					BorderTopStyle:          styles.BorderTopStyle,
					BorderLeftStyle:         styles.BorderLeftStyle,
					BorderRightStyle:        styles.BorderRightStyle,
					BorderBottomStyle:       styles.BorderBottomStyle,
					BorderTopColor:          styles.BorderTopColor,
					BorderLeftColor:         styles.BorderLeftColor,
					BorderRightColor:        styles.BorderRightColor,
					BorderBottomColor:       styles.BorderBottomColor,
					PaddingLeft:             styles.PaddingLeft,
					PaddingRight:            styles.PaddingRight,
					PaddingBottom:           styles.PaddingBottom,
					PaddingTop:              styles.PaddingTop,
					BorderTopLeftRadius:     styles.BorderTopLeftRadius,
					BorderTopRightRadius:    styles.BorderTopRightRadius,
					BorderBottomLeftRadius:  styles.BorderBottomLeftRadius,
					BorderBottomRightRadius: styles.BorderBottomRightRadius,
					BackgroundColor:         styles.BackgroundColor,
				}
				vl = cb.HTMLBorder(vl, hv)
				df.Doc.CurrentPage.OutputAt(pmb.x, pmb.y, vl)
				cb.stylesStack.PopStyles()
			}
		}
	}
	return nil
}

// buildPages takes the internal pagebox slice and outputs each item with page
// breaks in between.
func (cb *CSSBuilder) buildPages() error {
	/*
		The pagebox is a slice of nodes that are either a StartStop node or a VList
		node.
		The start node (a StartStop node that has an empty Start field) denotes the
		start of a box (such as a div or a p).
		The VList node is actually something to typeset.
	*/
	pd, err := cb.PageSize()
	if err != nil {
		return err
	}
	y := pd.Height - pd.MarginTop
	var height, shiftDown bag.ScaledPoint
	for _, n := range cb.pagebox {
		switch t := n.(type) {
		case *node.StartStop:
			// start node
			tAttribs := t.Attributes
			if _, ok := tAttribs["pagebreak"]; ok {
				if err := cb.NewPage(); err != nil {
					return err
				}
			}
			var hv HTMLValues
			var ok bool
			shiftDown = tAttribs["shiftDown"].(bag.ScaledPoint)
			y -= shiftDown

			if hv, ok = tAttribs["hv"].(HTMLValues); ok {
				if t.StartNode == nil {
					// top start node -> draw border
					x := t.Attributes["x"].(bag.ScaledPoint)
					vl := node.NewVList()
					vl.Width = tAttribs["hsize"].(bag.ScaledPoint)
					vl.Height = tAttribs["height"].(bag.ScaledPoint)
					vl = cb.HTMLBorder(vl, hv)
					cb.frontend.Doc.CurrentPage.OutputAt(x, y, vl)
					y -= hv.PaddingTop + hv.BorderTopWidth
				} else {
					// bottom start node -> just move cursor
					y -= hv.PaddingBottom + hv.BorderBottomWidth
				}
			}

		case *node.VList:
			tAttribs := t.Attributes
			height = tAttribs["height"].(bag.ScaledPoint)
			x := tAttribs["x"].(bag.ScaledPoint)
			cb.frontend.Doc.CurrentPage.OutputAt(x, y, t)
			y -= height
		}
	}
	return nil
}
