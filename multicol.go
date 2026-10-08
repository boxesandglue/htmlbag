package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// multicol holds the CSS Multi-column Layout properties of an element. None
// of them is inherited. Only the body is laid out in columns yet, by
// columnRegions; column-count elsewhere is set in one column, with a
// warning.
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

// columns returns the column count of the body te and the gap between the
// columns, or 0 when te is set in one column.
func (cb *CSSBuilder) columns(te *frontend.Text) (count int, gap bag.ScaledPoint) {
	m, ok := cb.multicols[te]
	if !ok || m.count < 2 {
		return 0, 0
	}
	return m.count, m.gap
}

// columnRegions is the pagination of OutputPagesFromText for a body with
// column-count: count regions side by side in the content area of every
// page, filled from the first to the last. The floats and footnotes of a
// column are placed in the column, at its width. Everything else page-level
// stays as in pageRegions.
type columnRegions struct {
	cb    *CSSBuilder
	count int
	gap   bag.ScaledPoint
	// page is the content area of the current page, col the column being
	// filled in it, from 0, and cur that column.
	page    region
	col     int
	cur     region
	started bool
}

// next moves on to the next column, or to the first column of a new page
// after the last column or for a forced break other than column.
func (cr *columnRegions) next(brk string) (region, error) {
	cb := cr.cb
	switch {
	case !cr.started:
		pg, err := cb.pageRegion()
		if err != nil {
			return region{}, err
		}
		// NewPage stores them on every later page.
		storePageDimensions(cb, cb.currentPageDimensions)
		cr.page, cr.col = pg, 0
	case (brk == "" || brk == "column") && cr.col+1 < cr.count:
		cr.col++
	default:
		if err := cb.shipoutAndStartPage(); err != nil {
			return region{}, err
		}
		pg, err := cb.pageRegion()
		if err != nil {
			return region{}, err
		}
		cr.page, cr.col = pg, 0
	}
	cr.started = true
	cr.cur = cr.column(cr.col)
	return cr.cur, nil
}

// column is column i of the current page. The columns share the page's
// height; the inserts of a column keep the page's vertical extent.
func (cr *columnRegions) column(i int) region {
	reg := cr.page
	n := bag.ScaledPoint(cr.count)
	reg.width = (cr.page.width - (n-1)*cr.gap) / n
	reg.left = cr.page.left + bag.ScaledPoint(i)*(reg.width+cr.gap)
	reg.inserts.left, reg.inserts.width = reg.left, reg.width
	return reg
}

func (cr *columnRegions) filled(filled) error {
	if !cr.started {
		return cr.cb.flushInserts()
	}
	return cr.cb.flushInsertsIn(cr.cur)
}
