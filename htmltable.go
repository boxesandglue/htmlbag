package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// parseColumnWidth parses a column width specification and returns a Glue node.
// Supports:
//   - fixed widths: "3cm", "50mm", "2in", "100pt"
//   - flexible widths: "*" (1 share), "2*" (2 shares), "3*" (3 shares)
func parseColumnWidth(width string) *node.Glue {
	g := node.NewGlue()
	width = strings.TrimSpace(width)

	if width == "" {
		// No width specified - auto
		g.Stretch = bag.Factor
		g.StretchOrder = 1
		return g
	}

	if strings.HasSuffix(width, "*") {
		// Flexible width: "*", "2*", "3*", etc.
		multiplier := 1.0
		prefix := strings.TrimSuffix(width, "*")
		if prefix != "" {
			if m, err := strconv.ParseFloat(prefix, 64); err == nil {
				multiplier = m
			}
		}
		g.Stretch = bag.ScaledPoint(multiplier * float64(bag.Factor))
		g.StretchOrder = 1
		return g
	}

	// Fixed width
	if sp, err := bag.SP(width); err == nil {
		g.Width = sp
	}
	return g
}

func (cb *CSSBuilder) buildTable(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	tbl := &frontend.Table{}
	tbl.MaxWidth = wd
	if sWd, ok := te.Settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok && strings.HasSuffix(wdStr, "%") {
			tbl.MaxWidth = ParseRelativeSize(wdStr, wd, wd)
			tbl.Stretch = true
		}
	}

	// Process colgroup for column specifications
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "colgroup" {
				cb.buildColgroup(t, tbl)
			}
		}
	}

	// First pass: process thead (header rows come first)
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "thead" {
				rowsBefore := len(tbl.Rows)
				cb.buildTBody(t, tbl)
				tbl.HeaderRows = len(tbl.Rows) - rowsBefore
			}
		}
	}
	// Second pass: process tbody (body rows come after header)
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "tbody" {
				cb.buildTBody(t, tbl)
			}
		}
	}
	vl, err := cb.frontend.BuildTable(tbl)
	if err != nil {
		return nil, err
	}

	// PDF/UA: tag the table structure
	if cb.enableTagging {
		cb.tagTable(vl[0], tbl)
	}

	return vl[0], nil
}

func (cb *CSSBuilder) buildColgroup(te *frontend.Text, tbl *frontend.Table) {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "col" {
				width := ""
				if w, ok := t.Settings[frontend.SettingColumnWidth].(string); ok {
					width = w
				}
				colSpec := frontend.ColSpec{
					ColumnWidth: parseColumnWidth(width),
				}
				tbl.ColSpec = append(tbl.ColSpec, colSpec)
			}
		}
	}
}

func (cb *CSSBuilder) buildTBody(te *frontend.Text, tbl *frontend.Table) {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "tr" {
				cb.buildTR(t, tbl)
			}
		}
	}
}

func (cb *CSSBuilder) buildTR(te *frontend.Text, tbl *frontend.Table) {
	tr := &frontend.TableRow{}
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "td" || elt == "th" {
				cb.buildTD(t, tr, elt == "th")
			}
		}
	}
	tbl.Rows = append(tbl.Rows, tr)
}

func (cb *CSSBuilder) buildTD(te *frontend.Text, row *frontend.TableRow, isHeader bool) {
	td := &frontend.TableCell{}
	td.IsHeader = isHeader

	// Extract colspan and rowspan
	settings := te.Settings
	if v, ok := settings[frontend.SettingColspan]; ok && v != nil {
		if colspan, ok := v.(int); ok && colspan > 1 {
			td.ExtraColspan = colspan - 1
		}
	}
	if v, ok := settings[frontend.SettingRowspan]; ok && v != nil {
		if rowspan, ok := v.(int); ok && rowspan > 1 {
			td.ExtraRowspan = rowspan - 1
		}
	}

	// Extract border settings from CSS
	if v, ok := settings[frontend.SettingBorderTopWidth]; ok && v != nil {
		td.BorderTopWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderBottomWidth]; ok && v != nil {
		td.BorderBottomWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderLeftWidth]; ok && v != nil {
		td.BorderLeftWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderRightWidth]; ok && v != nil {
		td.BorderRightWidth = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingBorderTopColor]; ok && v != nil {
		td.BorderTopColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderBottomColor]; ok && v != nil {
		td.BorderBottomColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderLeftColor]; ok && v != nil {
		td.BorderLeftColor = v.(*color.Color)
	}
	if v, ok := settings[frontend.SettingBorderRightColor]; ok && v != nil {
		td.BorderRightColor = v.(*color.Color)
	}
	// Extract padding settings
	if v, ok := settings[frontend.SettingPaddingTop]; ok && v != nil {
		td.PaddingTop = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingBottom]; ok && v != nil {
		td.PaddingBottom = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingLeft]; ok && v != nil {
		td.PaddingLeft = v.(bag.ScaledPoint)
	}
	if v, ok := settings[frontend.SettingPaddingRight]; ok && v != nil {
		td.PaddingRight = v.(bag.ScaledPoint)
	}
	// Extract background color
	if v, ok := settings[frontend.SettingBackgroundColor]; ok && v != nil {
		td.BackgroundColor = v.(*color.Color)
	}

	// If this cell references a pre-rendered VList, use it directly as content.
	if vlid, ok := settings[frontend.SettingPrerenderedVListID].(string); ok {
		if vl, vlOK := cb.PendingVLists[vlid]; vlOK {
			td.Contents = append(td.Contents, frontend.FormatToVList(func(wd bag.ScaledPoint) (*node.VList, error) {
				return vl, nil
			}))
		}
	}

	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			// For box elements (ul, ol, div, etc.), create a FormatToVList function
			// that uses CreateVlist - this ensures the same code path as outside tables
			if isBox, ok := t.Settings[frontend.SettingBox]; ok && isBox.(bool) {
				textCopy := t
				ftv := func(wd bag.ScaledPoint) (*node.VList, error) {
					vl, err := cb.CreateVlist(textCopy, wd)
					if err != nil {
						return nil, err
					}
					// Margin-bottom may have been propagated from a child
					// through a borderless parent (CSS margin collapsing).
					// In a table cell, materialize it as a kern.
					if mb, ok := textCopy.Settings[frontend.SettingMarginBottom]; ok {
						if mbSP, ok := mb.(bag.ScaledPoint); ok && mbSP > 0 {
							k := node.NewKern()
							k.Kern = mbSP
							k.Attributes = node.H{"origin": "margin-bottom"}
							vl.List = node.InsertAfter(vl.List, node.Tail(vl.List), k)
							vl.Height += mbSP
						}
					}
					return vl, nil
				}
				td.Contents = append(td.Contents, frontend.FormatToVList(ftv))
			} else {
				td.Contents = append(td.Contents, itm)
			}
		default:
			td.Contents = append(td.Contents, itm)
		}
	}
	row.Cells = append(row.Cells, td)
}

// tagTable walks the table VList and creates Table/TR/TH/TD structure elements.
func (cb *CSSBuilder) tagTable(tableVL *node.VList, tbl *frontend.Table) {
	tableSE := &document.StructureElement{Role: "Table"}
	cb.structureCurrent.AddChild(tableSE)

	// Create THead/TBody grouping SEs
	var theadSE, tbodySE *document.StructureElement
	if tbl.HeaderRows > 0 {
		theadSE = &document.StructureElement{Role: "THead"}
		tableSE.AddChild(theadSE)
	}
	tbodySE = &document.StructureElement{Role: "TBody"}
	tableSE.AddChild(tbodySE)

	// Walk rows: each child of the table VList is an HList (row)
	rowIdx := 0
	for cur := tableVL.List; cur != nil; cur = cur.Next() {
		rowHL, ok := cur.(*node.HList)
		if !ok {
			continue
		}
		if rowIdx >= len(tbl.Rows) {
			break
		}

		// Determine parent: THead for header rows, TBody otherwise
		rowParent := tbodySE
		if theadSE != nil && rowIdx < tbl.HeaderRows {
			rowParent = theadSE
		}

		trSE := &document.StructureElement{Role: "TR"}
		rowParent.AddChild(trSE)

		// Walk cells in this row
		row := tbl.Rows[rowIdx]
		cellIdx := 0
		for cellCur := rowHL.List; cellCur != nil; cellCur = cellCur.Next() {
			cellVL, ok := cellCur.(*node.VList)
			if !ok {
				continue
			}
			if cellIdx >= len(row.Cells) {
				break
			}

			cell := row.Cells[cellIdx]
			role := "TD"
			if cell.IsHeader {
				role = "TH"
			}
			cellSE := &document.StructureElement{Role: role}
			// Set Scope for TH cells
			if cell.IsHeader {
				if rowIdx < tbl.HeaderRows {
					cellSE.Scope = "Column"
				} else {
					cellSE.Scope = "Row"
				}
			}
			cellSE.ActualText = extractCellText(cell)
			trSE.AddChild(cellSE)
			tagVList(cellVL, cellSE)
			cellIdx++
		}
		rowIdx++
	}
}

// extractCellText extracts text content from a table cell's contents.
func extractCellText(cell *frontend.TableCell) string {
	var b strings.Builder
	for _, cc := range cell.Contents {
		switch t := cc.(type) {
		case *frontend.Text:
			b.WriteString(extractTextContent(t))
		}
	}
	return b.String()
}
