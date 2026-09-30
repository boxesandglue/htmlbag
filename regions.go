package htmlbag

import (
	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// region is one rectangle the paginator fills.
type region struct {
	// width and height are the size of the rectangle.
	width, height bag.ScaledPoint
	// left and top are its top-left corner in PDF coordinates on page.
	left, top bag.ScaledPoint
	// page is the page the region lies on, pageNum its 1-based number.
	// Headings, anchors and inside/outside floats take their page from here.
	page    *document.Page
	pageNum int
}

// bottom is the y coordinate of the region's bottom edge.
func (r region) bottom() bag.ScaledPoint { return r.top - r.height }

// isRight reports whether the region lies on a right (recto) page.
func (r region) isRight() bool { return r.pageNum%2 == 1 }

// filled reports how a region was filled. pageRegions paints from the
// builder's page buffer and needs nothing in it yet.
type filled struct{}

// regions hands out the rectangles the paginator fills and takes each back
// once it is filled.
type regions interface {
	// next returns the region to fill: once before the first block, then
	// whenever the content moves on. brk is "" for an automatic break, else
	// the forced break-before or break-after keyword that caused it.
	next(brk string) (region, error)
	// filled hands back every region exactly once, the last one included,
	// before the next call to next.
	filled(f filled) error
}

// pageRegions is the pagination of OutputPagesFromText: one region per
// page, its content area. Everything page-level stays here: the @page rules
// and margin boxes of NewPage, and the floats, footnotes and positioned
// boxes that flushInserts paints with the body.
type pageRegions struct {
	cb      *CSSBuilder
	cur     region
	started bool
}

// next starts a new page for every call but the first, which takes the
// current page. Every brk is a page break here.
func (pr *pageRegions) next(brk string) (region, error) {
	cb := pr.cb
	if pr.started {
		if err := cb.shipoutAndStartPage(); err != nil {
			return region{}, err
		}
	}
	reg, err := cb.pageRegion()
	if err != nil {
		return region{}, err
	}
	if !pr.started {
		// NewPage stores them on every later page.
		storePageDimensions(cb, cb.currentPageDimensions)
	}
	pr.started, pr.cur = true, reg
	return reg, nil
}

func (pr *pageRegions) filled(filled) error {
	if !pr.started {
		return pr.cb.flushInserts()
	}
	return pr.cb.flushInsertsIn(pr.cur)
}

// pageRegion is the content area of the current page as a region.
func (cb *CSSBuilder) pageRegion() (region, error) {
	pd, err := cb.PageSize()
	if err != nil {
		return region{}, err
	}
	return region{
		width:   pd.ContentWidth,
		height:  pd.ContentHeight,
		left:    pd.PageAreaLeft,
		top:     pd.Height - pd.PageAreaTop,
		page:    cb.frontend.Doc.CurrentPage,
		pageNum: len(cb.frontend.Doc.Pages),
	}, nil
}

// flowCursor is the paginator's place in its regions.
type flowCursor struct {
	regions regions
	cur     region
}

// breakTo hands back the current region and moves on to the next one.
func (fc *flowCursor) breakTo(brk string) error {
	if err := fc.regions.filled(filled{}); err != nil {
		return err
	}
	reg, err := fc.regions.next(brk)
	if err != nil {
		return err
	}
	fc.cur = reg
	return nil
}

// breakKeyword is the forced break keyword of a break-before or break-after
// value, or "" when it is not one.
func breakKeyword(v any) string {
	if !isForcedBreakValue(v) {
		return ""
	}
	s, _ := v.(string)
	return s
}

// breakAfterKeyword is the forced break-after keyword of n, or "".
func breakAfterKeyword(n node.Node) string {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		return breakKeyword(vl.Attributes["pageBreakAfter"])
	}
	return ""
}
