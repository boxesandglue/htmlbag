package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func (cb *CSSBuilder) buildTable(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	tbl := &frontend.Table{}
	tbl.MaxWidth = wd
	if sWd, ok := te.Settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok && strings.HasSuffix(wdStr, "%") {
			tbl.MaxWidth = ParseRelativeSize(wdStr, wd, wd)
			tbl.Stretch = true
		}
	}
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt := t.Settings[frontend.SettingDebug].(string)
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

func (cb *CSSBuilder) buildTBody(te *frontend.Text, tbl *frontend.Table) {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt := t.Settings[frontend.SettingDebug].(string)
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
			elt := t.Settings[frontend.SettingDebug].(string)
			if elt == "td" {
				cb.buildTD(t, tr)
			}
		}
	}
	tbl.Rows = append(tbl.Rows, tr)
}

func (cb *CSSBuilder) buildTD(te *frontend.Text, row *frontend.TableRow) {
	td := &frontend.TableCell{}
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
