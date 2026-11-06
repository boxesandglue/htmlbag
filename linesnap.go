package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// Line snapping (line-snap: baseline): the lines of an element stand on the
// line grid of the page, the one the trace linegrid draws. The grid starts at
// the top of the page area, so a line of one column stands level with a line
// of the next one, and with the columns below a spanner.
//
// A line is snapped where it is placed, as only there its height on the page
// is known. The piece between two snapped lines has to be a multiple of the
// grid, and the rest goes into the margins of that piece, in proportion to
// them. They may shrink down to zero: the line goes to the nearest grid line
// unless that takes more than the margins have. A piece without margins takes
// the rest as a kern above the line.
//
// The lines of a paragraph move apart to the next multiple of the grid, which
// does not depend on where they stand. Everything else is rigid: a box with a
// border or background has its first line snapped and keeps its inside, a
// table or a block image is not snapped at all, and the line after them finds
// the grid again. The zone of the top floats is a multiple of the grid, so a
// float that arrives below lines already placed moves them from line to line.
//
// To reach the margins inside a transparent container, the container is taken
// apart at placement, so that only one level of the tree has to be handled.
// Its children then no longer are whole items: a page of another width does
// not rebuild them.

// lineSnapValues are the values of line-snap, true for a snapped line.
var lineSnapValues = map[string]bool{"none": false, "baseline": true}

// attrLineSnap marks the box of an element with line-snap: baseline.
const attrLineSnap = "_lineSnap"

// lineSnapTolerance is the difference from the grid that counts as none. A
// line-height computed from a factor misses the grid by rounding errors.
const lineSnapTolerance = bag.Factor / 20

// setLineSnap records that the lines of an element's block Text are snapped.
func (cb *CSSBuilder) setLineSnap(te *frontend.Text, snap bool) {
	if cb == nil || !snap {
		return
	}
	if cb.lineSnaps == nil {
		cb.lineSnaps = map[*frontend.Text]bool{}
	}
	cb.lineSnaps[te] = true
}

// stampLineSnap puts the mark of t on the box built from it.
func (cb *CSSBuilder) stampLineSnap(vl *node.VList, t *frontend.Text) {
	if cb.lineSnaps[t] {
		vl.SetAttribute(attrLineSnap, true)
	}
}

// snapsLines reports whether te's tree holds an element with line-snap:
// baseline.
func (cb *CSSBuilder) snapsLines(te *frontend.Text) bool {
	if len(cb.lineSnaps) == 0 {
		return false
	}
	return findText(te, func(t *frontend.Text) bool { return cb.lineSnaps[t] }) != nil
}

// startLineSnap sets up the grid the lines of te snap to and reports whether
// there is one.
func (cb *CSSBuilder) startLineSnap(te *frontend.Text) bool {
	cb.snapGrids, cb.snapSource = nil, nil
	if !cb.snapsLines(te) {
		return false
	}
	src := lineGridSource(te)
	if src == nil {
		return false
	}
	cb.snapSource = src
	if cb.snapGridAt(cb.snapStep()) == nil {
		cb.snapSource = nil
		return false
	}
	return true
}

// snapStep is the distance of the lines of the grid on the current page, 0
// when the flow does not snap.
func (cb *CSSBuilder) snapStep() bag.ScaledPoint {
	if cb.snapSource == nil {
		return 0
	}
	if cb.pageLineStep > 0 {
		return cb.pageLineStep
	}
	return paragraphLeading(cb.snapSource.Settings)
}

// snapGridAt returns the grid of step.
func (cb *CSSBuilder) snapGridAt(step bag.ScaledPoint) *lineGrid {
	if g, ok := cb.snapGrids[step]; ok {
		return g
	}
	g := cb.lineGridAt(cb.snapSource, step)
	if cb.snapGrids == nil {
		cb.snapGrids = map[bag.ScaledPoint]*lineGrid{}
	}
	cb.snapGrids[step] = g
	return g
}

// snapGrid returns the grid of the current page, nil when the flow does not
// snap.
func (fc *flowCursor) snapGrid(cb *CSSBuilder) *lineGrid {
	if !fc.snapping {
		return nil
	}
	return cb.snapGridAt(cb.snapStep())
}

// snapped reports whether n is a block whose lines are snapped.
func (fc *flowCursor) snapped(n node.Node) bool {
	if !fc.snapping {
		return false
	}
	vl, ok := n.(*node.VList)
	if !ok || vl.Attributes == nil || isPlaceholderBox(vl) {
		return false
	}
	if _, isFloat := floatBoxHeight(n); isFloat {
		return false
	}
	if o, _ := vl.Attributes["origin"].(string); o == "table" {
		return false
	}
	on, _ := vl.Attributes[attrLineSnap].(bool)
	return on
}

// blockFirstLine returns the distance from the top of n to the baseline of its
// first line, false when it has none.
func blockFirstLine(n *node.VList) (bag.ScaledPoint, bool) {
	// A box with a border holds its lines in a frame, and only the snapshot
	// of the split shows them.
	if inner, ok := n.Attributes["_splittableInner"].([]node.Node); ok {
		hv, _ := n.Attributes["_splittableHv"].(HTMLValues)
		d, found := firstLineIn(inner)
		return hv.BorderTopWidth + hv.PaddingTop + d, found
	}
	var items []node.Node
	for c := n.List; c != nil; c = c.Next() {
		items = append(items, c)
	}
	return firstLineIn(items)
}

// firstLineIn is blockFirstLine for a stack of items.
func firstLineIn(items []node.Node) (bag.ScaledPoint, bool) {
	var y bag.ScaledPoint
	for _, c := range items {
		switch t := c.(type) {
		case *node.HList:
			return y + t.Height, true
		case *node.VList:
			if _, isFloat := floatBoxHeight(t); isFloat {
				continue
			}
			if d, ok := blockFirstLine(t); ok {
				return y + d, true
			}
		}
		y += vlistNodeHeight(c)
	}
	return 0, false
}

// snapPieceStart is the first entry of the page buffer that belongs to the
// piece of the line being snapped. A table placed directly empties the
// buffer.
func (fc *flowCursor) snapPieceStart(cb *CSSBuilder) int {
	return min(fc.snapFrom, len(cb.pageBuf))
}

// snapMargins is the sum of the margins of the current piece.
func (fc *flowCursor) snapMargins(cb *CSSBuilder) bag.ScaledPoint {
	var sum bag.ScaledPoint
	for _, e := range cb.pageBuf[fc.snapPieceStart(cb):] {
		if k, ok := marginKern(e.box.List); ok && e.box.List.Next() == nil && k.Kern > 0 {
			sum += k.Kern
		}
	}
	return sum
}

// gridOffset is how far the current place lies below the origin of the grid,
// topFloats being the height of the top floats above the body.
func (fc *flowCursor) gridOffset(cb *CSSBuilder, topFloats bag.ScaledPoint) bag.ScaledPoint {
	return fc.cur.gridTop - fc.cur.top + topFloats + cb.pageBufHeight
}

// snapRest returns how far a line must move to stand on the grid, line being
// the distance of its baseline from the current place.
func (fc *flowCursor) snapRest(cb *CSSBuilder, line, topFloats bag.ScaledPoint) bag.ScaledPoint {
	g := fc.snapGrid(cb)
	if g == nil {
		return 0
	}
	return nearestGridLine(fc.gridOffset(cb, topFloats)+line, g, fc.snapMargins(cb))
}

// nearestGridLine returns the distance from the baseline y to the nearest
// line of g that lies below the first baseline, and the next one when the
// nearest would take more than margins from what is above y.
func nearestGridLine(y bag.ScaledPoint, g *lineGrid, margins bag.ScaledPoint) bag.ScaledPoint {
	n := (y - g.first + g.step/2) / g.step
	if y < g.first {
		n = 0
	}
	rest := g.first + n*g.step - y
	switch {
	case max(rest, -rest) <= lineSnapTolerance:
		return 0
	case rest < 0 && margins+rest < 0:
		return rest + g.step
	}
	return rest
}

// blockSnapRest is the rest of the first line of n, a block about to be
// placed with the inserts incoming, 0 when n is not snapped.
func (fc *flowCursor) blockSnapRest(cb *CSSBuilder, n node.Node, incoming []*Insert) bag.ScaledPoint {
	if !fc.snapped(n) {
		return 0
	}
	line, ok := blockFirstLine(n.(*node.VList))
	if !ok {
		return 0
	}
	floats := append(append([]*Insert{}, cb.pageInserts[InsertFloatTop]...), filterInserts(incoming, InsertFloatTop)...)
	return fc.snapRest(cb, line, cb.totalFloatTopHeight(floats))
}

// snapBlock snaps the first line of n, about to be buffered, and reports
// whether it has one. Its inserts are committed.
func (fc *flowCursor) snapBlock(cb *CSSBuilder, n node.Node) bool {
	if !fc.snapped(n) {
		return false
	}
	line, ok := blockFirstLine(n.(*node.VList))
	if !ok {
		return false
	}
	fc.applySnap(cb, fc.snapRest(cb, line, cb.pageInsertHeight[InsertFloatTop]), line)
	return true
}

// snapSlack is what a line may need at most to reach the grid.
func (fc *flowCursor) snapSlack(cb *CSSBuilder) bag.ScaledPoint {
	if g := fc.snapGrid(cb); g != nil {
		return g.step
	}
	return 0
}

// applySnap spends rest on the current piece and ends it. The buffered
// margins are replaced and not changed: a trial shares them with the real
// run.
func (fc *flowCursor) applySnap(cb *CSSBuilder, rest bag.ScaledPoint, line bag.ScaledPoint) {
	if rest != 0 {
		fc.traceSnap(cb, rest, line)
	}
	if margins := fc.snapMargins(cb); rest != 0 && margins > 0 {
		left, spare := rest, margins
		for i := fc.snapPieceStart(cb); i < len(cb.pageBuf); i++ {
			k, ok := marginKern(cb.pageBuf[i].box.List)
			if !ok || cb.pageBuf[i].box.List.Next() != nil || k.Kern <= 0 {
				continue
			}
			share := bag.ScaledPoint(int64(left) * int64(k.Kern) / int64(spare))
			left, spare = left-share, spare-k.Kern
			cb.pageBuf[i] = snappedMargin(k, share, cb.pageBuf[i])
			cb.pageBufHeight += share
		}
	} else if rest != 0 {
		k := node.NewKern()
		k.Kern = rest
		k.Attributes = node.H{"origin": "line snap"}
		cb.bufferBody(node.Vpack(k), rest)
	}
}

// snappedMargin is the page buffer entry of the margin k, grown by share.
func snappedMargin(k *node.Kern, share bag.ScaledPoint, e pageBufEntry) pageBufEntry {
	nk := node.NewKern()
	nk.Kern = k.Kern + share
	nk.Attributes = k.Attributes
	box := node.NewVList()
	box.List = nk
	box.Width = e.box.Width
	box.Height = e.height + share
	box.Attributes = e.box.Attributes
	return pageBufEntry{box: box, height: e.height + share}
}

// endPiece starts a new piece behind what the page buffer holds.
func (fc *flowCursor) endPiece(cb *CSSBuilder) {
	fc.snapFrom = len(cb.pageBuf)
}

// transparentForSnap reports whether vl, the node about to be placed, is a
// container to take apart: nothing draws around its children, and there is a
// snapped line among them.
func (fc *flowCursor) transparentForSnap(cb *CSSBuilder, vl *node.VList, room bag.ScaledPoint) bool {
	if !fc.snapping || vl.Attributes == nil || vl.List == nil {
		return false
	}
	if isPlaceholderBox(vl) {
		return false
	}
	if _, isFloat := floatBoxHeight(vl); isFloat {
		return false
	}
	if o, _ := vl.Attributes["origin"].(string); o == "table" {
		return false
	}
	if _, leaf := vl.Attributes["_splittableTe"]; leaf {
		return false
	}
	if hv, ok := vl.Attributes["_splittableHv"].(HTMLValues); ok && (hv.hasBorder() || hv.BackgroundColor != nil) {
		return false
	}
	if _, _, clone := decorationClone(vl); clone || cb.fragmentTrim(vl) != (breakTrim{}) {
		return false
	}
	if avoidBreakInside(vl) && vlistNodeHeight(vl) <= room {
		return false
	}
	return fc.holdsSnapped(vl)
}

// holdsSnapped reports whether a block in vl has snapped lines.
func (fc *flowCursor) holdsSnapped(vl *node.VList) bool {
	for n := vl.List; n != nil; n = n.Next() {
		c, ok := n.(*node.VList)
		if !ok {
			continue
		}
		if fc.snapped(c) || fc.holdsSnapped(c) {
			return true
		}
	}
	return false
}

// spliceChildren puts the children of wrap in its place in the chain, next
// being the node after it, and returns the first one. What wrap carries for
// the paginator goes with them.
func spliceChildren(wrap *node.VList, next node.Node) node.Node {
	propagateInsertsAttr(wrap, wrap.List)
	propagateFlowChild(wrap, wrap.List)
	first := wrap.List
	last := node.Tail(first)
	last.SetNext(next)
	if next != nil {
		next.SetPrev(last)
	}
	first.SetPrev(nil)
	return first
}

// takeApartForSnap is spliceChildren for a container the line snapping takes
// apart, whose own shift, marks and break-after are its children's from then
// on.
func takeApartForSnap(wrap *node.VList, next node.Node) node.Node {
	shiftChildren(wrap)
	carryBoxMarks(wrap, wrap.List)
	propagateAnchorIndices(wrap, wrap.List)
	if v, ok := wrap.Attributes["pageBreakAfter"]; ok {
		for n := node.Tail(wrap.List); n != nil; n = n.Prev() {
			if isContentNode(n) {
				n.SetAttribute("pageBreakAfter", v)
				break
			}
		}
	}
	return spliceChildren(wrap, next)
}

// snapLines moves the lines of the paragraph vl apart to multiples of the
// grid. It leaves alone a box that draws around them, whose frame is made
// already.
func (fc *flowCursor) snapLines(cb *CSSBuilder, vl *node.VList) {
	g := fc.snapGrid(cb)
	if g == nil {
		return
	}
	if hv, _ := vl.Attributes["_splittableHv"].(HTMLValues); hv.hasBorder() || hv.BackgroundColor != nil {
		return
	}
	if _, _, clone := decorationClone(vl); clone {
		return
	}
	var prev *node.HList
	var gap bag.ScaledPoint
	changed := false
	for n := vl.List; n != nil; n = n.Next() {
		hl, ok := n.(*node.HList)
		if !ok {
			gap += vlistNodeHeight(n)
			continue
		}
		if prev != nil {
			d := prev.Depth + gap + hl.Height
			if extra := gridMultiple(d, g.step) - d; extra > lineSnapTolerance {
				k := node.NewKern()
				k.Kern = extra
				k.Attributes = node.H{"origin": "line snap"}
				vl.List = node.InsertBefore(vl.List, hl, k)
				vl.Height += extra
				changed = true
			}
		}
		prev, gap = hl, 0
	}
	if _, ok := vl.Attributes["_splittableInner"]; ok && changed {
		var inner []node.Node
		for n := vl.List; n != nil; n = n.Next() {
			inner = append(inner, n)
		}
		vl.Attributes["_splittableInner"] = inner
	}
}

// gridMultiple is the multiple of step that d does not exceed by more than
// the tolerance, at least one.
func gridMultiple(d, step bag.ScaledPoint) bag.ScaledPoint {
	return max((d-lineSnapTolerance+step-1)/step, 1) * step
}

// traceSnap notes rest for the trace, at the line it moves.
func (fc *flowCursor) traceSnap(cb *CSSBuilder, rest, line bag.ScaledPoint) {
	if cb.pageLineGrid == nil || fc.cur.trial {
		return
	}
	cb.snapPending = append(cb.snapPending, snapMark{left: fc.cur.left, y: cb.pageBufHeight + line + rest, rest: rest})
}

// snapMark is a rest in the trace: rest moved the line whose baseline is at
// y, in a region that begins at left. While the region is filled, y counts
// from the top of its body.
type snapMark struct {
	left, y, rest bag.ScaledPoint
}

// settleSnapMarks gives the marks of a region whose body is painted from top
// on their place on the page. Only now it is known: a top float moves it.
func (cb *CSSBuilder) settleSnapMarks(top bag.ScaledPoint) {
	for _, m := range cb.snapPending {
		m.y = top - m.y
		cb.snapMarks = append(cb.snapMarks, m)
	}
	cb.snapPending = nil
}

// traceSnapMarks paints the rests of the page, each left of the line it
// moved, and forgets them.
func (cb *CSSBuilder) traceSnapMarks(g *lineGrid, page *document.Page) {
	for _, m := range cb.snapMarks {
		hl, err := g.label(formatRest(m.rest), 0)
		if err != nil {
			bag.Logger.Warn("The line grid trace cannot set a rest, leaving it out", "error", err)
			break
		}
		vl := node.Vpack(hl)
		vl.Attributes = node.H{"origin": "linegrid trace", "artifact": document.ArtifactLayout}
		page.OutputAt(m.left-textWidth(hl)-lineGridOverhang-lineGridNumberGap, m.y+hl.Height, vl)
	}
	cb.snapMarks = nil
}

// textWidth is the width of the glyphs of a line, which is as wide as the
// paragraph it was set in.
func textWidth(hl *node.HList) bag.ScaledPoint {
	var w bag.ScaledPoint
	for n := hl.List; n != nil; n = n.Next() {
		switch t := n.(type) {
		case *node.Glyph:
			w += t.Width
		case *node.HList:
			w += textWidth(t)
		}
	}
	return w
}

// formatRest writes a rest as +5pt or -1.5pt.
func formatRest(rest bag.ScaledPoint) string {
	s := strconv.FormatFloat(rest.ToPT(), 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if rest > 0 {
		s = "+" + s
	}
	return s + "pt"
}
