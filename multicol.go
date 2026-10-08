package htmlbag

import (
	"errors"
	"maps"
	"slices"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/boxesandglue/frontend/pdfdraw"
)

// multicol holds the CSS Multi-column Layout properties of an element. None
// of them is inherited. The body is laid out in columns by columnRegions, and
// so is a direct child of the body without a border, a background or
// padding, whose siblings then span the columns; column-count elsewhere is
// set in one column, with a warning.
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
	// boxed is set for an element with a border, a background or padding,
	// which cannot hand its children to the columns of the page.
	boxed bool
}

// multicolBody returns the Text whose items flow into columns for body, and
// the column properties, or nil when body is set in one column. That is body
// itself when it has column-count, or else a Text made of the items of a
// direct child with column-count, between the child's siblings, which span
// the columns.
func (cb *CSSBuilder) multicolBody(body *frontend.Text) (*frontend.Text, multicol) {
	if m, ok := cb.multicols[body]; ok {
		return body, m
	}
	for i, itm := range body.Items {
		child, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		m, ok := cb.multicols[child]
		if !ok {
			continue
		}
		if m.boxed {
			bag.Logger.Warn("column-count on an element with a border, a background or padding is not laid out, the content is set in one column")
			return nil, multicol{}
		}
		flow := &frontend.Text{Settings: maps.Clone(body.Settings)}
		for _, sib := range body.Items[:i] {
			cb.markSpanner(sib)
			flow.Items = append(flow.Items, sib)
		}
		flow.Items = append(flow.Items, child.Items...)
		for _, sib := range body.Items[i+1:] {
			cb.markSpanner(sib)
			flow.Items = append(flow.Items, sib)
		}
		return flow, m
	}
	return nil, multicol{}
}

// markSpanner makes itm span the columns, if it is a block.
func (cb *CSSBuilder) markSpanner(itm any) {
	if t, ok := itm.(*frontend.Text); ok {
		if cb.spanners == nil {
			cb.spanners = map[*frontend.Text]bool{}
		}
		cb.spanners[t] = true
	}
}

// splitAtSpanners splits the groups of a multi-column flow before and after
// every item that spans the columns, and reports for every group whether it
// is such a spanner. Loose items go with the item before them.
func (cb *CSSBuilder) splitAtSpanners(groups [][]any) ([][]any, []bool) {
	var out [][]any
	var spans []bool
	for _, g := range groups {
		start := 0
		span := cb.isSpanner(g[0])
		for k := 1; k < len(g); k++ {
			t, ok := g[k].(*frontend.Text)
			if !ok || cb.spanners[t] == span {
				continue
			}
			out, spans = append(out, g[start:k]), append(spans, span)
			start, span = k, !span
		}
		out, spans = append(out, g[start:]), append(spans, span)
	}
	return out, spans
}

func (cb *CSSBuilder) isSpanner(itm any) bool {
	t, ok := itm.(*frontend.Text)
	return ok && cb.spanners[t]
}

// columnRegions is the pagination of OutputPagesFromText for a multi-column
// body: rows of count columns side by side, each column filled after the one
// before it, and between the rows bands as wide as the page for the elements
// that span the columns. A row ends at the foot of the page or below its
// tallest column, where a spanner follows. The floats and footnotes of a
// column are placed in the column, at its width. Everything else page-level
// stays as in pageRegions.
type columnRegions struct {
	cb    *CSSBuilder
	count int
	gap   bag.ScaledPoint
	// fillAuto is column-fill: auto, which balances only the rows before a
	// spanner, not the last one.
	fillAuto bool
	// body is the Text the flow takes its items from.
	body *frontend.Text
	// rule is the column-rule drawn between two columns of a row that
	// both hold content.
	rule multicol

	// page is the content area of the current page.
	page region
	// rowTop is the top edge of the current row, rowUsed the height of its
	// tallest column so far, col the column being filled, from 0.
	rowTop, rowUsed bag.ScaledPoint
	col             int
	// filledCols marks the columns of the row that hold content.
	filledCols []bool
	// span is set while a spanner band is filled, spanTop is its top edge
	// and spanUsed its height once filled. wantSpan tells next what comes:
	// flowText sets it before every group.
	span, wantSpan    bool
	spanTop, spanUsed bag.ScaledPoint
	cur               region
	started           bool

	// row counts the rows begun since balanceColumns reset it. While
	// balanced is set, the columns of row balanceRow are that high.
	row, balanceRow int
	balanced        bag.ScaledPoint
	// trial is set for the copy balanceColumns runs a group through: it
	// starts no page and paints nothing, and next fails with
	// errColumnsOverflow when the content moves on from the balanced row.
	// tooTall is then set when a column of that row holds more than fits.
	trial, tooTall bool
}

// errColumnsOverflow stops a trial run whose content does not fit into the
// balanced row.
var errColumnsOverflow = errors.New("htmlbag: the content does not fit into the balanced row")

func (cr *columnRegions) pageBottom() bag.ScaledPoint { return cr.page.top - cr.page.height }

// next hands out the next column of the row, the first column of a new row on
// the next page, a spanner band below the row, or a new row below the
// spanner. A forced break other than column starts a new page.
func (cr *columnRegions) next(brk string) (region, error) {
	forcedPage := brk != "" && brk != "column"
	switch {
	case !cr.started:
		pg, err := cr.cb.pageRegion()
		if err != nil {
			return region{}, err
		}
		// NewPage stores them on every later page.
		storePageDimensions(cr.cb, cr.cb.currentPageDimensions)
		cr.page = pg
		cr.startRow(pg.top)
		cr.span, cr.spanTop = cr.wantSpan, pg.top
	case cr.wantSpan && !cr.span:
		cr.endRow()
		cr.span = true
		cr.spanTop = cr.rowTop - cr.rowUsed
		if forcedPage {
			if err := cr.newPage(); err != nil {
				return region{}, err
			}
			cr.spanTop = cr.page.top
		}
	case !cr.wantSpan && cr.span:
		cr.span = false
		if forcedPage {
			if err := cr.newPage(); err != nil {
				return region{}, err
			}
		} else {
			cr.startRow(cr.spanTop - cr.spanUsed)
		}
	case cr.span:
		if err := cr.newPage(); err != nil {
			return region{}, err
		}
		cr.spanTop = cr.page.top
	case !forcedPage && cr.col+1 < cr.count:
		cr.col++
	default:
		cr.endRow()
		if err := cr.newPage(); err != nil {
			return region{}, err
		}
	}
	cr.started = true
	cr.cur = cr.current()
	return cr.cur, nil
}

// newPage starts a new page with a new row at its top. A trial keeps the
// page it has, and fails when it leaves the balanced row.
func (cr *columnRegions) newPage() error {
	if cr.trial {
		if cr.balanced > 0 && cr.row >= cr.balanceRow {
			return errColumnsOverflow
		}
	} else {
		if err := cr.cb.shipoutAndStartPage(); err != nil {
			return err
		}
		pg, err := cr.cb.pageRegion()
		if err != nil {
			return err
		}
		cr.page = pg
	}
	cr.startRow(cr.page.top)
	return nil
}

func (cr *columnRegions) startRow(top bag.ScaledPoint) {
	if cr.started {
		cr.row++
	}
	cr.rowTop, cr.rowUsed, cr.col = top, 0, 0
	cr.filledCols = make([]bool, cr.count)
}

// endRow draws the column rule of the row that ends: in the middle of the
// gap between two columns that both hold content, as high as the tallest
// column. A trial draws nothing.
func (cr *columnRegions) endRow() {
	r := cr.rule
	if cr.trial || cr.span || !r.ruleSolid || r.ruleWidth <= 0 || cr.rowUsed <= 0 {
		return
	}
	col := r.ruleColor
	if col == nil {
		col = cr.cb.frontend.GetColor("black")
	}
	n := bag.ScaledPoint(cr.count)
	width := (cr.page.width - (n-1)*cr.gap) / n
	for i := 0; i+1 < cr.count; i++ {
		if !cr.filledCols[i] || !cr.filledCols[i+1] {
			continue
		}
		rule := node.NewRule()
		rule.Width, rule.Height = r.ruleWidth, cr.rowUsed
		rule.Pre = pdfdraw.NewStandalone().
			ColorNonstroking(*col).
			Rect(0, 0, r.ruleWidth, -cr.rowUsed).
			Fill().
			String()
		vl := node.Vpack(rule)
		vl.Attributes = node.H{"origin": "column rule", "artifact": document.ArtifactLayout}
		x := cr.page.left + bag.ScaledPoint(i+1)*width + bag.ScaledPoint(i)*cr.gap + (cr.gap-r.ruleWidth)/2
		cr.cb.frontend.Doc.CurrentPage.OutputAt(x, cr.rowTop, vl)
	}
}

// finish draws the rule of the last row once the flow has ended.
func (cr *columnRegions) finish() {
	if cr.started && !cr.span {
		cr.endRow()
	}
}

// current is the region to fill: column col of the current row, or the
// spanner band.
func (cr *columnRegions) current() region {
	reg := cr.page
	top, left, width := cr.rowTop, cr.page.left, cr.page.width
	if cr.span {
		top = cr.spanTop
	} else {
		n := bag.ScaledPoint(cr.count)
		width = (cr.page.width - (n-1)*cr.gap) / n
		left = cr.page.left + bag.ScaledPoint(cr.col)*(width+cr.gap)
	}
	height := top - cr.pageBottom()
	if !cr.span && cr.balanced > 0 && cr.row == cr.balanceRow {
		height = min(height, cr.balanced)
	}
	reg.top, reg.height, reg.left, reg.width = top, height, left, width
	// Below content on the page, a block that does not fit moves on; from
	// the second column on, a margin at the top is truncated all the same.
	reg.occupied = top < cr.page.top
	reg.truncates = reg.occupied && !cr.span && cr.col > 0
	// The inserts keep the page's margin edges where the region reaches
	// them, and the region's edges elsewhere.
	reg.inserts.left, reg.inserts.width = left, width
	if top < cr.page.top {
		reg.inserts.top = top
	}
	if height < top-cr.pageBottom() {
		reg.inserts.bottom = top - height
	}
	if cr.trial {
		reg.sink, reg.trial = &regionSink{}, true
	}
	return reg
}

// filled notes how high the region is filled, then paints it, or discards
// it in a trial.
func (cr *columnRegions) filled(filled) error {
	cb := cr.cb
	if !cr.started {
		return cb.flushInserts()
	}
	used := cb.pageInsertHeight[InsertFloatTop] + cb.pageBufHeight
	if cb.pageInsertHeight[InsertFloatBottom]+cb.pageInsertHeight[InsertFootnote] > 0 {
		used = cr.cur.height
	}
	if cr.span {
		cr.spanUsed = used
	} else {
		cr.rowUsed = max(cr.rowUsed, used)
		if used > 0 {
			cr.filledCols[cr.col] = true
		}
		if cr.balanced > 0 && cr.row == cr.balanceRow && used > cr.cur.height {
			cr.tooTall = true
		}
	}
	return cb.flushInsertsIn(cr.cur)
}

// balances reports whether group i of the flow, not a spanner, is balanced:
// before a spanner always, at the end unless column-fill is auto.
func (cr *columnRegions) balances(i int, spans []bool) bool {
	if i+1 < len(spans) {
		return spans[i+1]
	}
	return !cr.fillAuto
}

// balanceColumns makes the last row that the group vl fills, starting in the
// current column, only as high as it takes to hold the rest of the group in
// its columns: rows before it are full. It runs the group through trial
// copies of the regions, first to find that row, then to search the
// smallest height that holds it. When a trial cannot run, or the group ends
// in the column it starts in, the last of its row, the row stays full. The
// floats and footnotes of a balanced row are placed in it, at the foot of
// its columns.
func (cb *CSSBuilder) balanceColumns(vl *node.VList, fc *flowCursor, cr *columnRegions) {
	cr.row, cr.balanced = 0, 0
	end, ok := cb.columnTrial(vl, fc, cr, -1, 0)
	// With only the last column of the row left, there is nothing to
	// spread the content over.
	if !ok || end.row == 0 && cr.col == cr.count-1 {
		return
	}
	lo, hi := bag.ScaledPoint(0), end.rowTop-end.pageBottom()
	for hi-lo > bag.MustSP("0.5pt") {
		mid := lo + (hi-lo)/2
		if _, fits := cb.columnTrial(vl, fc, cr, end.row, mid); fits {
			hi = mid
		} else {
			lo = mid
		}
	}
	cr.balanceRow, cr.balanced = end.row, hi
	cr.cur = cr.current()
	fc.cur = cr.cur
}

// columnTrial runs a copy of vl through a trial copy of cr from fc's place,
// with the columns of row balanceRow balanced at height h (none for a
// negative row). It returns the regions as the trial leaves them, and
// whether the group fits: it ran to its end, in the balanced row when there
// is one, without a column there holding more than h. The builder's page
// is as it was afterwards.
func (cb *CSSBuilder) columnTrial(vl *node.VList, fc *flowCursor, cr *columnRegions, balanceRow int, h bag.ScaledPoint) (*columnRegions, bool) {
	t := *cr
	t.filledCols = slices.Clone(cr.filledCols)
	t.trial, t.tooTall, t.row, t.balanceRow, t.balanced = true, false, 0, balanceRow, h
	if balanceRow < 0 {
		t.balanced = 0
	}
	tfc := *fc
	tfc.regions = &t
	t.cur = t.current()
	tfc.cur = t.cur

	saved := cb.takePageState()
	cb.pageBuf = append([]pageBufEntry(nil), saved.buf...)
	cb.pageBufHeight = saved.bufHeight
	for k, v := range saved.inserts {
		cb.pageInserts[k] = append([]*Insert(nil), v...)
	}
	maps.Copy(cb.pageInsertHeight, saved.insertsHgt)
	defer cb.restorePageState(saved)

	restart, _, err := cb.outputGroupNodes(trialCopy(vl), &tfc)
	if err != nil || restart >= 0 {
		return &t, false
	}
	if err := t.filled(filled{}); err != nil {
		return &t, false
	}
	return &t, !t.tooTall
}

// trialCopy copies vl deeply for a trial run. Copy clones the attributes
// only shallowly, so the lines a splittable block lists in _splittableInner
// would still be the original ones, and splitting the block in the trial
// would cut them up for the real run: they are mapped to their copies. A
// listed node that is not in the tree is copied on its own.
func trialCopy(vl *node.VList) *node.VList {
	cp := vl.Copy().(*node.VList)
	copies := map[node.Node]node.Node{vl: cp}
	var pair func(a, b node.Node)
	pair = func(a, b node.Node) {
		for ; a != nil && b != nil; a, b = a.Next(), b.Next() {
			copies[a] = b
			switch t := a.(type) {
			case *node.VList:
				pair(t.List, b.(*node.VList).List)
			case *node.HList:
				pair(t.List, b.(*node.HList).List)
			}
		}
	}
	pair(vl.List, cp.List)
	for _, c := range copies {
		var attrs node.H
		switch t := c.(type) {
		case *node.VList:
			attrs = t.Attributes
		case *node.HList:
			attrs = t.Attributes
		}
		inner, ok := attrs["_splittableInner"].([]node.Node)
		if !ok {
			continue
		}
		mapped := make([]node.Node, len(inner))
		for i, n := range inner {
			switch m, ok := copies[n]; {
			case ok:
				mapped[i] = m
			case isVList(n):
				mapped[i] = trialCopy(n.(*node.VList))
			default:
				mapped[i] = n.Copy()
			}
		}
		attrs["_splittableInner"] = mapped
	}
	return cp
}

func isVList(n node.Node) bool {
	_, ok := n.(*node.VList)
	return ok
}
