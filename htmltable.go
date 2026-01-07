package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
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
				cb.buildTBody(t, tbl)
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
				cb.buildTD(t, tr)
			}
		}
	}
	tbl.Rows = append(tbl.Rows, tr)
}

func (cb *CSSBuilder) buildTD(te *frontend.Text, row *frontend.TableRow) {
	td := &frontend.TableCell{}

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

	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			// For box elements (ul, ol, div, etc.), create a FormatToVList function
			// that uses CreateVlist - this ensures the same code path as outside tables
			if isBox, ok := t.Settings[frontend.SettingBox]; ok && isBox.(bool) {
				textCopy := t
				ftv := func(wd bag.ScaledPoint) (*node.VList, error) {
					return cb.CreateVlist(textCopy, wd)
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
