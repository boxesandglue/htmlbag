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

	// Push a fresh insert-collection scope; restore on exit so nested
	// tables don't leak their inserts into the enclosing table.
	savedInserts := cb.tableInserts
	savedWidth := cb.tableInsertWidth
	cb.tableInserts = nil
	cb.tableInsertWidth = tbl.MaxWidth
	defer func() {
		cb.tableInserts = savedInserts
		cb.tableInsertWidth = savedWidth
	}()

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
	// Third pass: process tfoot. HTML5 allows <tfoot> in any source
	// order within <table>, but it always renders at the bottom of
	// the table (after all body rows). Appending to tbl.Rows last
	// matches that visual order; tagTable later marks the trailing
	// FooterRows as TFoot in the structure tree.
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "tfoot" {
				rowsBefore := len(tbl.Rows)
				cb.buildTBody(t, tbl)
				tbl.FooterRows += len(tbl.Rows) - rowsBefore
			}
		}
	}
	// Collect source tr Texts in the same order BuildTable will emit row
	// HLists (thead first, then tbody). Capture the page-break-inside
	// sentinel off each tr's Settings before BuildTable runs, and delete
	// the sentinel from Settings so it cannot leak into FormatParagraph
	// (whose setting-type switch has a strict "unknown setting" default).
	var trPageBreakInside []any
	collectTRs := func(section *frontend.Text) {
		for _, itm := range section.Items {
			tr, ok := itm.(*frontend.Text)
			if !ok {
				continue
			}
			if elt, _ := tr.Settings[frontend.SettingDebug].(string); elt != "tr" {
				continue
			}
			v := tr.Settings[settingPageBreakInside]
			trPageBreakInside = append(trPageBreakInside, v)
			delete(tr.Settings, settingPageBreakInside)
		}
	}
	for _, itm := range te.Items {
		if t, ok := itm.(*frontend.Text); ok {
			if elt, _ := t.Settings[frontend.SettingDebug].(string); elt == "thead" {
				collectTRs(t)
			}
		}
	}
	for _, itm := range te.Items {
		if t, ok := itm.(*frontend.Text); ok {
			if elt, _ := t.Settings[frontend.SettingDebug].(string); elt == "tbody" {
				collectTRs(t)
			}
		}
	}

	vls, err := cb.frontend.BuildTable(tbl)
	if err != nil {
		return nil, err
	}

	vl := vls[0]

	// Propagate page-break-inside from source tr Texts onto row HList
	// Attributes. Rows are emitted in the same order we collected above,
	// so walk vl.List in parallel with trPageBreakInside.
	if len(trPageBreakInside) > 0 {
		i := 0
		for n := vl.List; n != nil && i < len(trPageBreakInside); n = n.Next() {
			hl, ok := n.(*node.HList)
			if !ok {
				continue
			}
			if v := trPageBreakInside[i]; v != nil {
				if hl.Attributes == nil {
					hl.Attributes = node.H{}
				}
				hl.Attributes["pageBreakInside"] = v
			}
			i++
		}

		// Restore the captured sentinels onto the source tr Texts so a
		// rebuild of this table at another width (page width reflow in
		// outputTableRows) propagates them again.
		i = 0
		restoreTRs := func(section *frontend.Text) {
			for _, itm := range section.Items {
				tr, ok := itm.(*frontend.Text)
				if !ok {
					continue
				}
				if elt, _ := tr.Settings[frontend.SettingDebug].(string); elt != "tr" {
					continue
				}
				if i < len(trPageBreakInside) && trPageBreakInside[i] != nil {
					tr.Settings[settingPageBreakInside] = trPageBreakInside[i]
				}
				i++
			}
		}
		for _, itm := range te.Items {
			if t, ok := itm.(*frontend.Text); ok {
				if elt, _ := t.Settings[frontend.SettingDebug].(string); elt == "thead" {
					restoreTRs(t)
				}
			}
		}
		for _, itm := range te.Items {
			if t, ok := itm.(*frontend.Text); ok {
				if elt, _ := t.Settings[frontend.SettingDebug].(string); elt == "tbody" {
					restoreTRs(t)
				}
			}
		}
	}

	// Attach all inserts collected from this table's cells. The page
	// builder will reserve space at the bottom of the page where the
	// table lands. Multi-page tables (outputTableRows path) are a known
	// limitation: all inserts attach to the first row's VList.
	if len(cb.tableInserts) > 0 {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["inserts"] = cb.tableInserts
	}

	// PDF/UA: tag the table structure.
	// Repeated headers on continuation pages are left untagged
	// (the backend will wrap them as artifacts in PDF/UA mode).
	if cb.enableTagging {
		cb.tagTable(vl, tbl)
	}

	// Source Text and its formatting width: lets outputTableRows rebuild
	// the remaining rows when an automatic page break switches to a page
	// with a different content width.
	if vl.Attributes == nil {
		vl.Attributes = node.H{}
	}
	vl.Attributes["_tableTe"] = te
	vl.Attributes["_tableTeWidth"] = wd

	return vl, nil
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
	// CSS vertical-align (top/middle/bottom) maps to TableCell.VAlign.
	// Without this propagation cells default to middle, which mid-aligns
	// short labels next to multi-line bodies in a hanging-indent layout.
	if v, ok := settings[frontend.SettingVAlign]; ok && v != nil {
		if va, ok := v.(frontend.VerticalAlignment); ok {
			td.VAlign = va
		}
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
			// Pull any insertMarkers out of this cell's text tree before
			// it reaches FormatParagraph / BuildTable. Footnote markers
			// become in-text superscript calls; float markers become
			// empty placeholders. Bodies are collected on the CSSBuilder
			// for attachment to the table VList; the page builder then
			// dispatches each by class.
			fns, err := cb.extractFootnotes(t, cb.tableInsertWidth)
			if err == nil && len(fns) > 0 {
				cb.tableInserts = append(cb.tableInserts, fns...)
			}
			topFls, err := cb.extractFloats(t, cb.tableInsertWidth, InsertFloatTop)
			if err == nil && len(topFls) > 0 {
				cb.tableInserts = append(cb.tableInserts, topFls...)
			}
			bottomFls, err := cb.extractFloats(t, cb.tableInsertWidth, InsertFloatBottom)
			if err == nil && len(bottomFls) > 0 {
				cb.tableInserts = append(cb.tableInserts, bottomFls...)
			}
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
			} else if vl := singleDeferredFormattedVListInText(t); vl != nil {
				// The Text contains exactly one VList carrying a
				// deferred FormatToVList closure and no other content.
				// FormatParagraph in the cell pipeline mishandles
				// inline-VList-only paragraphs (the linebreaker
				// produces empty lines and drops the VList), so
				// short-circuit: hand the closure straight to the
				// cell builder. The cell builder will call it at the
				// final paraWidth.
				td.Contents = append(td.Contents, getDeferredFormatter(vl))
			} else if hasDeferredFormatterInTextTree(t) {
				// Mixed inline content with a deferred closure somewhere
				// inside — e.g. paragraph text + inline SVG. Resolve
				// the closures against the real cell width, then run
				// FormatParagraph.
				tCaptured := t
				td.Contents = append(td.Contents, frontend.FormatToVList(func(wd bag.ScaledPoint) (*node.VList, error) {
					resolveDeferredSizing(tCaptured.Items, wd)
					vl, _, err := cb.frontend.FormatParagraph(tCaptured, wd)
					return vl, err
				}))
			} else {
				td.Contents = append(td.Contents, itm)
			}
		default:
			// frontend.BuildTable's TableCell pipeline only recognises
			// *Text and FormatToVList in cell Contents — a raw
			// *node.VList is silently dropped from the cell during
			// build. Wrap any pre-built VList (typical carriers:
			// inline-svg wrappers from collectHorizontalNodes, raster
			// <img width=…%> wrappers from the deferred-sizing path)
			// in a FormatToVList closure so the cell can include them.
			// The closure is also the canonical materialization site
			// for any DeferredSizer attached to the VList: it fires at
			// the exact cell content width, after frontend.BuildTable
			// has settled column widths.
			if vl, ok := itm.(*node.VList); ok {
				if ftv := getDeferredFormatter(vl); ftv != nil {
					td.Contents = append(td.Contents, ftv)
				} else {
					// Bare *node.VList without deferred-sizing marker —
					// wrap in a passthrough closure so the cell
					// builder doesn't drop it.
					vlCaptured := vl
					td.Contents = append(td.Contents, frontend.FormatToVList(func(_ bag.ScaledPoint) (*node.VList, error) {
						return vlCaptured, nil
					}))
				}
			} else {
				td.Contents = append(td.Contents, itm)
			}
		}
	}
	row.Cells = append(row.Cells, td)
}

// tagTable walks the table VList and creates Table/TR/TH/TD structure elements.
func (cb *CSSBuilder) tagTable(tableVL *node.VList, tbl *frontend.Table) {
	format := cb.frontend.Doc.Format
	tableSE := newSE("Table", format)
	cb.structureCurrent.AddChild(tableSE)

	// Create THead/TBody/TFoot grouping SEs. PDF/UA-1 §7.5 maps these
	// directly to the HTML element names; TFoot is added in source
	// order after TBody, matching both the painted order on the page
	// and the reading order expected by AT.
	var theadSE, tbodySE, tfootSE *document.StructureElement
	if tbl.HeaderRows > 0 {
		theadSE = newSE("THead", format)
		tableSE.AddChild(theadSE)
	}
	tbodySE = newSE("TBody", format)
	tableSE.AddChild(tbodySE)
	if tbl.FooterRows > 0 {
		tfootSE = newSE("TFoot", format)
		tableSE.AddChild(tfootSE)
	}
	footerStart := len(tbl.Rows) - tbl.FooterRows

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

		// Determine parent: THead for header rows, TFoot for trailing
		// footer rows, TBody for everything in between.
		rowParent := tbodySE
		if theadSE != nil && rowIdx < tbl.HeaderRows {
			rowParent = theadSE
		} else if tfootSE != nil && rowIdx >= footerStart {
			rowParent = tfootSE
		}

		trSE := newSE("TR", format)
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
			canonical := "TD"
			if cell.IsHeader {
				canonical = "TH"
			}
			cellSE := newSE(canonical, format)
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
