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
	// column, left, right, …). Before the first block, it is that block's
	// forced break-before, "" without one.
	Next(brk string) (Region, error)
	// Filled hands back every region exactly once, the last one included,
	// before the Next that follows it. When FlowText returns an error, the
	// region it was filling is not handed back.
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
	// PageNum is the 1-based number of the caller's page the region lies
	// on: headings and anchors take their page from it, and inside/outside
	// floats their side, odd being right. 0, the zero value, counts as an
	// even page, so a region that leaves it unset lies on a left page.
	PageNum int
	// Left and Top are the region's top-left corner in PDF coordinates on
	// the caller's page, where the caller places Filled.Box. Heading
	// positions for the outline are computed from them.
	Left, Top bag.ScaledPoint
}

// Filled reports how a region was filled.
type Filled struct {
	// Box is the region's content, to be placed at the region's top-left
	// corner. Box.Height == Used.
	Box *node.VList
	// Used is the height from the region's top edge to the bottom edge of
	// the last box in it, or of a side float that reaches further. It
	// exceeds the region's Height when a box too tall for the empty region
	// is placed in it anyway.
	Used bag.ScaledPoint
	// MarginAfter is the margin below the last box, as far as it reaches
	// below Used: the last block's margin-bottom at the end of the flow,
	// the margin spent at the foot of the region at an automatic break.
	MarginAfter bag.ScaledPoint
	// Fragments lists the parts of the flow's children in the region, top
	// to bottom.
	Fragments []Fragment
}

// Fragment is the part of one flow child that landed in a region. The flow's
// children are the blocks of the body. Only they get an id per fragment: an
// id nested inside a child that splits is not on that region's boxes.
type Fragment struct {
	// ID is the child's id attribute, "" without one.
	ID string
	// Index is the child's position among the flow's children. Loose text
	// between blocks counts as an anonymous child of its own.
	Index int
	// Top is the fragment's top edge, measured from the region's top edge,
	// and Height its height, both without the child's margins.
	Top, Height bag.ScaledPoint
	// Continued is set when the child began in an earlier region, Continues
	// when it goes on in the next one.
	Continued, Continues bool
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
// page content the builder holds is kept as it was. As with
// OutputPagesFromText, that includes the widows and orphans HTMLToText
// recorded, so each Text goes to FlowText before the next HTMLToText.
//
// FlowText returns an error when it is called while it or
// OutputPagesFromText is running on the same builder, such as from a method
// of r.
func (cb *CSSBuilder) FlowText(te *frontend.Text, r Regions) error {
	if r == nil {
		return errors.New("htmlbag: FlowText needs regions")
	}
	done, err := cb.startFlow()
	if err != nil {
		return err
	}
	defer done()
	saved := cb.takePageState()
	defer func() {
		cb.fragLines = nil
		cb.reflowRebuild = false
		cb.restorePageState(saved)
	}()
	cb.dropPageLevelContent()
	fc := &flowCursor{regions: &callerRegions{cb: cb, r: r}, caller: true}
	cb.callerFlow = fc
	defer func() { cb.callerFlow = nil }()
	marginAfter, err := cb.flowText(te, fc)
	if err != nil {
		return err
	}
	return fc.regions.filled(filled{marginAfter: marginAfter})
}

// startFlow marks the builder as flowing until done is called, or fails
// when it already is: a flow keeps its state in the builder.
func (cb *CSSBuilder) startFlow() (done func(), err error) {
	if cb.flowing {
		return nil, errors.New("htmlbag: OutputPagesFromText or FlowText is already running on this builder")
	}
	cb.flowing = true
	return func() { cb.flowing = false }, nil
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
// Of the running elements, only those the last HTMLToText added go: a name
// an earlier Text captured first keeps serving that Text's pages.
func (cb *CSSBuilder) dropPageLevelContent() {
	if n := len(cb.positionedItems); n > 0 {
		bag.Logger.Warn("FlowText does not support position: absolute or fixed, dropping the elements", "count", n)
		cb.positionedItems = nil
	}
	if n := len(cb.textRunning); n > 0 {
		bag.Logger.Warn("FlowText does not support running elements, dropping them", "count", n)
		for name, added := range cb.textRunning {
			if added {
				delete(cb.runningElements, name)
			}
		}
		cb.textRunning = nil
	}
}

// region is one rectangle the paginator fills.
type region struct {
	// width and height are the size of the rectangle.
	width, height bag.ScaledPoint
	// left and top are its top-left corner in PDF coordinates on page.
	left, top bag.ScaledPoint
	// page is the page a page region paints on, nil for a caller's region.
	// pageNum is the 1-based page number headings, anchors and
	// inside/outside floats take.
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
	// next is the flow child the next region starts with, nil at the end
	// of the flow or when it is not known.
	next *flowChild
}

// regions hands out the rectangles the paginator fills and takes each back
// once it is filled.
type regions interface {
	// next returns the region to fill: once before the first block, then
	// whenever the content moves on. brk is "" for an automatic break, else
	// the forced break-before or break-after keyword that caused it, also
	// before the first block.
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
	// seen holds the flow children placed in an earlier region.
	seen map[*flowChild]bool
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
	if cr.seen == nil {
		cr.seen = map[*flowChild]bool{}
	}
	return cr.r.Filled(cr.cur.sink.filled(cr.cur.width, f, cr.seen))
}

// regionSink collects the boxes of a caller's region, each at its offset
// from the region's top edge.
type regionSink struct {
	entries []sinkEntry
}

type sinkEntry struct {
	off, height bag.ScaledPoint
	// floats is how far below off the side floats in box paint.
	floats bag.ScaledPoint
	box    *node.VList
	margin bool
	child  *flowChild
}

func (s *regionSink) empty() bool { return len(s.entries) == 0 }

func (s *regionSink) add(off bag.ScaledPoint, box *node.VList, h bag.ScaledPoint) {
	// The spacer only moves the body below the table rows placed before it.
	if o, _ := box.Attributes["origin"].(string); o == "table continuation spacer" {
		return
	}
	_, margin := marginKern(box.List)
	margin = margin && box.List.Next() == nil
	child := flowChildOf(box)
	if child == nil && box.List != nil {
		child = flowChildOf(box.List)
	}
	s.entries = append(s.entries, sinkEntry{off: off, height: h, floats: floatsBottom(box), box: box, margin: margin, child: child})
}

// floatsBottom is how far below the top of n the side floats in it paint: a
// float box reports no height of its own.
func floatsBottom(n node.Node) bag.ScaledPoint {
	if fh, ok := floatBoxHeight(n); ok {
		return fh
	}
	vl, ok := n.(*node.VList)
	if !ok {
		return 0
	}
	var y, bottom bag.ScaledPoint
	for c := vl.List; c != nil; c = c.Next() {
		if b := floatsBottom(c); b > 0 {
			bottom = max(bottom, y+b)
		}
		y += vlistNodeHeight(c)
	}
	return bottom
}

// filled assembles the boxes into the region's Box, the boxes of each
// fragment in a box of their own. Margins below the last box are left out and
// reported as MarginAfter, collapsed with the flow's. A side float that
// paints below the last box extends Used to its bottom. seen holds the flow
// children of earlier regions and takes this region's.
func (s *regionSink) filled(width bag.ScaledPoint, f filled, seen map[*flowChild]bool) Filled {
	n := len(s.entries)
	var trailing bag.ScaledPoint
	for n > 0 && s.entries[n-1].margin {
		trailing += s.entries[n-1].height
		n--
	}
	entries := s.entries[:n]
	s.entries = nil
	box := node.NewVList()
	box.Width = width
	var frags []Fragment
	var cursor, floats bag.ScaledPoint
	var tail node.Node
	for i := 0; i < len(entries); {
		j := i + 1
		child := entries[i].child
		for child != nil && j < len(entries) && entries[j].child == child {
			j++
		}
		run := entries[i:j]
		i = j
		top := run[0].off
		var nd *node.VList
		if child == nil {
			nd = run[0].box
		} else {
			nd = stackEntries(run, width)
			if child.id != "" && !carriesID(run, child.id) {
				nd.SetAttribute("id", child.id)
			}
			if fr, ok := fragmentOf(run, child, seen); ok {
				frags = append(frags, fr)
			}
		}
		tail = appendAt(box, tail, cursor, top, nd)
		cursor = top + vlistNodeHeight(nd)
		for _, e := range run {
			floats = max(floats, e.off+e.floats)
		}
	}
	margin := max(trailing, f.marginAfter)
	if floats > cursor {
		k := node.NewKern()
		k.Kern = floats - cursor
		appendAt(box, tail, 0, 0, k)
		margin = max(0, margin-k.Kern)
		cursor = floats
	}
	box.Height = cursor
	for _, e := range entries {
		if e.child != nil {
			seen[e.child] = true
		}
	}
	if k := len(frags); k > 0 && f.next != nil && entries[len(entries)-1].child == f.next {
		frags[k-1].Continues = true
	}
	return Filled{Box: box, Used: cursor, MarginAfter: margin, Fragments: frags}
}

// appendAt appends nd to the list of box after tail, with a kern from cursor
// down to top, and returns the new tail.
func appendAt(box *node.VList, tail node.Node, cursor, top bag.ScaledPoint, nd node.Node) node.Node {
	nd.SetPrev(nil)
	nd.SetNext(nil)
	if gap := top - cursor; gap != 0 {
		k := node.NewKern()
		k.Kern = gap
		tail = appendAt(box, tail, 0, 0, k)
	}
	if tail == nil {
		box.List = nd
	} else {
		tail.SetNext(nd)
		nd.SetPrev(tail)
	}
	return nd
}

// stackEntries stacks the boxes of one fragment in a box of its own that
// spans what they draw, from the leftmost edge (the rows of a table narrower
// than the region, an indented block), or the region's width without any.
func stackEntries(run []sinkEntry, width bag.ScaledPoint) *node.VList {
	vl := node.NewVList()
	top := run[0].off
	var tail node.Node
	var left, right bag.ScaledPoint
	found := false
	for _, e := range run {
		if !e.margin {
			l, w := drawnExtent(e.box)
			if !found || l < left {
				left = l
			}
			if !found || l+w > right {
				right = l + w
			}
			found = true
		}
		tail = appendAt(vl, tail, top+vl.Height, e.off, e.box)
		vl.Height = e.off + e.height - top
	}
	if right <= left {
		vl.Width = width
		return vl
	}
	vl.ShiftX = left
	vl.Width = right - left
	for _, e := range run {
		// The shift comes off the box, or off its only child that carries it.
		e.box.ShiftX -= left
		if e.box.ShiftX >= 0 || e.box.List == nil || e.box.List.Next() != nil {
			continue
		}
		switch c := e.box.List.(type) {
		case *node.VList:
			c.ShiftX += e.box.ShiftX
			e.box.ShiftX = 0
		case *node.HList:
			c.ShiftX += e.box.ShiftX
			e.box.ShiftX = 0
		}
	}
	return vl
}

// drawnExtent is the left edge and width of what box draws: box itself, or
// its only child (a table row placed on its own sits in a box of the region's
// width), shifted by both.
func drawnExtent(box *node.VList) (left, width bag.ScaledPoint) {
	if box.List != nil && box.List.Next() == nil {
		switch c := box.List.(type) {
		case *node.VList:
			return box.ShiftX + c.ShiftX, c.Width
		case *node.HList:
			return box.ShiftX + c.ShiftX, c.Width
		}
	}
	return box.ShiftX, box.Width
}

// carriesID reports whether a box of the run already carries id: an unsplit
// paragraph, or a fragment of a split one.
func carriesID(run []sinkEntry, id string) bool {
	for _, e := range run {
		if v, _ := e.box.GetAttribute("id"); v == id {
			return true
		}
		if e.box.List != nil {
			if v, _ := e.box.List.GetAttribute("id"); v == id {
				return true
			}
		}
	}
	return false
}

// fragmentOf is the Fragment of child that run holds. Margins take no part
// in its extent, side floats all they paint; a run of margins alone is no
// fragment.
func fragmentOf(run []sinkEntry, child *flowChild, seen map[*flowChild]bool) (Fragment, bool) {
	fr := Fragment{ID: child.id, Index: child.index, Continued: seen[child]}
	found := false
	for _, e := range run {
		if e.margin {
			continue
		}
		if !found {
			fr.Top, found = e.off, true
		}
		fr.Height = max(fr.Height, e.off+max(e.height, e.floats)-fr.Top)
	}
	return fr, found
}

// attrFlowChild marks a node with the flow child it belongs to.
const attrFlowChild = "_flowChild"

// flowChild is one child of a caller's flow, a block of the body.
type flowChild struct {
	index int
	id    string
}

// flowChildren gives every block of a body its flowChild, the same one for
// every build of it.
type flowChildren struct {
	items []any
	// ordinal is the position of items[i] among the blocks, -1 for text
	// between them.
	ordinal []int
	byItem  map[int]*flowChild
}

func newFlowChildren(body *frontend.Text) *flowChildren {
	fcs := &flowChildren{items: body.Items, ordinal: make([]int, len(body.Items)), byItem: map[int]*flowChild{}}
	n := 0
	for i, itm := range body.Items {
		fcs.ordinal[i] = -1
		t, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		// The whitespace the box branch skips (see stampItemIndices).
		if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
			continue
		}
		fcs.ordinal[i] = n
		n++
	}
	return fcs
}

// stamp marks the item boxes of vl, built from the body items from base on,
// with their flow child.
func (fcs *flowChildren) stamp(vl *node.VList, base int) {
	for n := vl.List; n != nil; n = n.Next() {
		child, ok := n.(*node.VList)
		if !ok || child.Attributes == nil {
			continue
		}
		k, ok := child.Attributes["_groupItemIdx"].(int)
		if !ok || base+k >= len(fcs.items) || fcs.ordinal[base+k] < 0 {
			continue
		}
		i := base + k
		fch := fcs.byItem[i]
		if fch == nil {
			fch = &flowChild{index: fcs.ordinal[i]}
			if t, ok := fcs.items[i].(*frontend.Text); ok {
				fch.id, _ = t.Settings[frontend.SettingElementID].(string)
			}
			fcs.byItem[i] = fch
		}
		child.Attributes[attrFlowChild] = fch
	}
}

// flowChildOf returns the flow child n belongs to, or nil.
func flowChildOf(n node.Node) *flowChild {
	if n == nil {
		return nil
	}
	v, _ := n.GetAttribute(attrFlowChild)
	fch, _ := v.(*flowChild)
	return fch
}

// propagateFlowChild marks the nodes of the list to that have no flow child
// with the one of from, whose parts they are.
func propagateFlowChild(from, to node.Node) {
	fch := flowChildOf(from)
	if fch == nil {
		return
	}
	for n := to; n != nil; n = n.Next() {
		if flowChildOf(n) == nil {
			n.SetAttribute(attrFlowChild, fch)
		}
	}
}

// stampFragment gives a fragment of the split block blockVL the block's id
// and flow child.
func stampFragment(frag, blockVL *node.VList) {
	if id, ok := blockVL.Attributes["id"]; ok {
		frag.SetAttribute("id", id)
	}
	propagateFlowChild(blockVL, frag)
}

// attrMarginTop is the margin-top of the block after a collapsed-margin kern
// between two siblings, floats between them included.
const attrMarginTop = "_marginTop"

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
	// serial numbers the regions from 1, as pages cannot: a caller's
	// regions may share one.
	serial int
	// rebuiltIn is the serial of the region the current group's items were
	// last rebuilt for, 0 before a rebuild.
	rebuiltIn int
}

// start takes the first region, brk being the first block's forced
// break-before.
func (fc *flowCursor) start(brk string) error {
	reg, err := fc.regions.next(brk)
	if err != nil {
		return err
	}
	fc.cur, fc.top, fc.serial = reg, topKept, 1
	return nil
}

// breakTo hands back the current region and moves on to the next one. n is
// the node the next region starts with, or a block it goes on with.
func (fc *flowCursor) breakTo(brk string, n node.Node) error {
	if err := fc.regions.filled(filled{next: flowChildOf(n)}); err != nil {
		return err
	}
	reg, err := fc.regions.next(brk)
	if err != nil {
		return err
	}
	fc.cur = reg
	fc.serial++
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
	if fc.top != topTruncated {
		return false
	}
	if !fc.regionEmpty(cb) {
		fc.top = topPlaced
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
	m := max(fc.cur.marginBefore, 0)
	if k, ok := marginKern(cur); ok {
		// Of a margin collapsed across a forced break-after, only the
		// margin-top after the break is kept.
		if mt, ok := k.Attributes[attrMarginTop].(bag.ScaledPoint); ok {
			k.Kern = mt
		}
		k.Kern = max(k.Kern, m)
		return cur
	}
	if m == 0 {
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
