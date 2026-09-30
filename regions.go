package htmlbag

import (
	"errors"
	"fmt"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// Regions hands out the rectangles FlowText fills and takes each back once
// it is filled.
type Regions interface {
	// Next returns the region to fill: once before the first block, then
	// only when the content moves on. brk is "" for an automatic break, else
	// the forced break-before or break-after keyword that caused it (page,
	// column, left, right, …).
	Next(brk string) (Region, error)
	// Filled hands back every region exactly once, the last one included,
	// before the Next that follows it.
	Filled(f Filled) error
}

// Region is one rectangle to fill.
type Region struct {
	// Width and Height are the size of the rectangle.
	Width, Height bag.ScaledPoint
	// MarginBefore is the margin still open above the region, such as the
	// MarginAfter of a flow this one continues. It collapses with the first
	// block's margin-top in the first region and in a region after a forced
	// break. After an automatic break the margin at a region's top is
	// truncated (CSS Fragmentation 3 §5.2), so it is not used there.
	MarginBefore bag.ScaledPoint
	// Page is the page the region lies on and PageNum its 1-based number.
	// Headings, anchors and inside/outside floats take their page from here.
	Page    *document.Page
	PageNum int
	// Left and Top are the region's top-left corner in PDF coordinates on
	// Page, where the caller places Filled.Box. Heading positions for the
	// outline are computed from them.
	Left, Top bag.ScaledPoint
}

// Filled reports how a region was filled.
type Filled struct {
	// Box is the region's content, to be placed at the region's top-left
	// corner. Box.Height == Used.
	Box *node.VList
	// Used is the height from the region's top edge to the bottom edge of
	// the last box in it.
	Used bag.ScaledPoint
	// MarginAfter is the margin below the last box, not included in Used:
	// the last block's margin-bottom at the end of the flow, the margin
	// spent at the foot of the region at an automatic break.
	MarginAfter bag.ScaledPoint
}

// FlowText pours the blocks of te into the regions r hands out. It is the
// region-by-region counterpart of OutputPagesFromText and leaves the pages
// to the caller: nothing is painted, and no page is started or shipped out.
// Headings and anchors take their page and position from the regions.
//
// Side floats are laid out inside the regions. Footnotes, float: top and
// float: bottom, position: absolute and fixed, and running elements are
// page-level and not supported here: they are dropped with a warning.
//
// A region of another width than the one before it rebuilds the remaining
// whole blocks at its width, and re-breaks the rest of a paragraph or a table
// split across the two, as a page of another @page width does. What that
// cannot rebuild keeps the width it was built at: the lines of a paragraph
// that is the flow's only block, the children of a split box with a border
// or background, and a paragraph whose rest fails to re-break.
//
// Nothing of the flow is left in the builder when FlowText returns, and the
// page OutputPagesFromText may have in progress is kept as it was. As with
// OutputPagesFromText, that includes the widows and orphans HTMLToText
// recorded, so each Text goes to FlowText before the next HTMLToText.
func (cb *CSSBuilder) FlowText(te *frontend.Text, r Regions) error {
	if r == nil {
		return errors.New("htmlbag: FlowText needs regions")
	}
	saved := cb.takePageState()
	defer func() {
		cb.fragLines = nil
		cb.reflowRebuild = false
		cb.restorePageState(saved)
	}()
	cb.dropPageLevelContent()
	fc := &flowCursor{regions: &callerRegions{cb: cb, r: r}, caller: true}
	marginAfter, err := cb.flowText(te, fc)
	if err != nil {
		return err
	}
	return fc.regions.filled(filled{marginAfter: marginAfter})
}

// pageState is the part of the builder that holds the page being filled.
type pageState struct {
	buf        []pageBufEntry
	bufHeight  bag.ScaledPoint
	inserts    map[InsertClass][]*Insert
	insertsHgt map[InsertClass]bag.ScaledPoint
}

// takePageState sets the page being filled aside and leaves the builder with
// an empty one.
func (cb *CSSBuilder) takePageState() pageState {
	s := pageState{cb.pageBuf, cb.pageBufHeight, cb.pageInserts, cb.pageInsertHeight}
	cb.pageBuf, cb.pageBufHeight = nil, 0
	cb.pageInserts = map[InsertClass][]*Insert{}
	cb.pageInsertHeight = map[InsertClass]bag.ScaledPoint{}
	return s
}

func (cb *CSSBuilder) restorePageState(s pageState) {
	cb.pageBuf, cb.pageBufHeight = s.buf, s.bufHeight
	cb.pageInserts, cb.pageInsertHeight = s.inserts, s.insertsHgt
}

// dropPageLevelContent drops the positioned and running elements HTMLToText
// took out of the flow: they belong to a page, which FlowText does not make.
func (cb *CSSBuilder) dropPageLevelContent() {
	if n := len(cb.positionedItems); n > 0 {
		bag.Logger.Warn("FlowText does not support position: absolute or fixed, dropping the elements", "count", n)
		cb.positionedItems = nil
	}
	if n := len(cb.runningElements); n > 0 {
		bag.Logger.Warn("FlowText does not support running elements, dropping them", "count", n)
		clear(cb.runningElements)
	}
}

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
	// marginBefore is Region.MarginBefore.
	marginBefore bag.ScaledPoint
	// sink collects the boxes of a caller's region; nil for a page region,
	// whose boxes are painted onto the page.
	sink *regionSink
}

// bottom is the y coordinate of the region's bottom edge.
func (r region) bottom() bag.ScaledPoint { return r.top - r.height }

// isRight reports whether the region lies on a right (recto) page.
func (r region) isRight() bool { return r.pageNum%2 == 1 }

// output places box of height h with its top edge at y.
func (r region) output(y bag.ScaledPoint, box *node.VList, h bag.ScaledPoint) {
	if r.sink != nil {
		r.sink.add(r.top-y, box, h)
		return
	}
	r.page.OutputAt(r.left, y, box)
}

// filled reports how a region was filled.
type filled struct {
	// marginAfter is the margin-bottom that ends the flow, set for the last
	// region only.
	marginAfter bag.ScaledPoint
}

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

// callerRegions takes the regions of FlowText from the caller and hands each
// one back with the boxes collected in it.
type callerRegions struct {
	cb      *CSSBuilder
	r       Regions
	cur     region
	started bool
	count   int
}

func (cr *callerRegions) next(brk string) (region, error) {
	rg, err := cr.r.Next(brk)
	if err != nil {
		return region{}, err
	}
	cr.count++
	if rg.Width <= 0 || rg.Height <= 0 {
		return region{}, fmt.Errorf("htmlbag: region %d is %s × %s, it needs a width and a height", cr.count, rg.Width, rg.Height)
	}
	cr.cur = region{
		width:        rg.Width,
		height:       rg.Height,
		left:         rg.Left,
		top:          rg.Top,
		page:         rg.Page,
		pageNum:      rg.PageNum,
		marginBefore: rg.MarginBefore,
		sink:         &regionSink{},
	}
	cr.started = true
	return cr.cur, nil
}

func (cr *callerRegions) filled(f filled) error {
	if !cr.started {
		return nil
	}
	if err := cr.cb.flushInsertsIn(cr.cur); err != nil {
		return err
	}
	return cr.r.Filled(cr.cur.sink.filled(cr.cur.width, f.marginAfter))
}

// regionSink collects the boxes of a caller's region, each at its offset
// from the region's top edge.
type regionSink struct {
	entries []sinkEntry
}

type sinkEntry struct {
	off, height bag.ScaledPoint
	box         *node.VList
	margin      bool
}

func (s *regionSink) empty() bool { return len(s.entries) == 0 }

func (s *regionSink) add(off bag.ScaledPoint, box *node.VList, h bag.ScaledPoint) {
	// The spacer only moves the body below the table rows placed before it.
	if o, _ := box.Attributes["origin"].(string); o == "table continuation spacer" {
		return
	}
	_, margin := marginKern(box.List)
	margin = margin && box.List.Next() == nil
	s.entries = append(s.entries, sinkEntry{off: off, height: h, box: box, margin: margin})
}

// filled assembles the boxes into the region's Box. Margins below the last
// box are left out and reported as MarginAfter, collapsed with flowMargin.
func (s *regionSink) filled(width, flowMargin bag.ScaledPoint) Filled {
	n := len(s.entries)
	var trailing bag.ScaledPoint
	for n > 0 && s.entries[n-1].margin {
		trailing += s.entries[n-1].height
		n--
	}
	box := node.NewVList()
	box.Width = width
	var cursor bag.ScaledPoint
	var tail node.Node
	appendNode := func(nd node.Node) {
		if tail == nil {
			box.List = nd
		} else {
			tail.SetNext(nd)
			nd.SetPrev(tail)
		}
		tail = nd
	}
	for _, e := range s.entries[:n] {
		if gap := e.off - cursor; gap != 0 {
			k := node.NewKern()
			k.Kern = gap
			appendNode(k)
		}
		e.box.SetPrev(nil)
		e.box.SetNext(nil)
		appendNode(e.box)
		cursor = e.off + e.height
	}
	box.Height = cursor
	s.entries = nil
	return Filled{Box: box, Used: cursor, MarginAfter: max(trailing, flowMargin)}
}

// marginKern returns n as a collapsed-margin kern.
func marginKern(n node.Node) (*node.Kern, bool) {
	k, ok := n.(*node.Kern)
	if !ok || k.Attributes == nil {
		return nil, false
	}
	o, _ := k.Attributes["origin"].(string)
	return k, o == "margin"
}

// regionTop is how the top of the current region came about, which decides
// what happens to a margin there.
type regionTop int

const (
	// topPlaced: something is in the region already.
	topPlaced regionTop = iota
	// topKept: the first region or one after a forced break, where margins
	// are kept and MarginBefore collapses with the first margin.
	topKept
	// topTruncated: a region after an automatic break, where a margin that
	// would sit at its top is truncated.
	topTruncated
)

// flowCursor is the paginator's place in its regions.
type flowCursor struct {
	regions regions
	cur     region
	// caller is set for FlowText's regions. Only there are margins at a
	// region top truncated after an automatic break, column breaks forced,
	// and page-level inserts dropped; OutputPagesFromText paginates as it
	// always has.
	caller bool
	top    regionTop
	warned map[InsertClass]bool
}

// start takes the first region.
func (fc *flowCursor) start() error {
	reg, err := fc.regions.next("")
	if err != nil {
		return err
	}
	fc.cur, fc.top = reg, topKept
	return nil
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
	fc.top = topKept
	if brk == "" {
		fc.top = topTruncated
	}
	return nil
}

// regionEmpty reports whether nothing is placed in a caller's region yet.
func (fc *flowCursor) regionEmpty(cb *CSSBuilder) bool {
	return fc.caller && cb.pageBufHeight == 0 && fc.cur.sink.empty()
}

// truncated reports whether cur, the next node to place, is a margin at the
// top of a caller's region after an automatic break, which is dropped.
func (fc *flowCursor) truncated(cb *CSSBuilder, cur node.Node) bool {
	if fc.top != topTruncated || !fc.regionEmpty(cb) {
		return false
	}
	_, isMargin := marginKern(cur)
	return isMargin
}

// marginBefore collapses MarginBefore with the margin at the top of a
// caller's region in the first region and after a forced break, cur being
// the next node to place. The returned node takes the place of cur.
func (fc *flowCursor) marginBefore(cb *CSSBuilder, cur node.Node) node.Node {
	if fc.top != topKept || !fc.regionEmpty(cb) {
		return cur
	}
	fc.top = topPlaced
	m := fc.cur.marginBefore
	if m <= 0 {
		return cur
	}
	if k, ok := marginKern(cur); ok {
		k.Kern = max(k.Kern, m)
		return cur
	}
	k := node.NewKern()
	k.Kern = m
	k.Attributes = node.H{"origin": "margin"}
	k.SetNext(cur)
	cur.SetPrev(k)
	return k
}

// insertsOn is insertsOnNode, less the page-level inserts a caller's region
// does not take.
func (fc *flowCursor) insertsOn(n node.Node) []*Insert {
	ins := insertsOnNode(n)
	if !fc.caller || len(ins) == 0 {
		return ins
	}
	for _, in := range ins {
		if fc.warned[in.Class] {
			continue
		}
		if fc.warned == nil {
			fc.warned = map[InsertClass]bool{}
		}
		fc.warned[in.Class] = true
		what := "footnotes"
		switch in.Class {
		case InsertFloatTop:
			what = "float: top"
		case InsertFloatBottom:
			what = "float: bottom"
		}
		bag.Logger.Warn("FlowText does not support " + what + ", dropping it")
	}
	return nil
}

// forcedKeyword is the forced break keyword of a break-before or break-after
// value, or "" when it is not one. A caller's regions also take column.
func (fc *flowCursor) forcedKeyword(v any) string {
	if s, _ := v.(string); fc.caller && s == "column" {
		return s
	}
	return breakKeyword(v)
}

// breakAfter is the forced break-after keyword of n, or "".
func (fc *flowCursor) breakAfter(n node.Node) string {
	if vl, ok := n.(*node.VList); ok && vl.Attributes != nil {
		return fc.forcedKeyword(vl.Attributes["pageBreakAfter"])
	}
	return ""
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
