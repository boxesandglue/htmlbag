package htmlbag

import (
	"errors"
	"slices"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
)

// RemainingRegions is a Regions that can name the regions still free on the
// current page, so that FlowText can balance the end of a flow over them.
type RemainingRegions interface {
	Regions
	// Remaining returns, in order, the regions Next("") would hand out after
	// the current one, up to the end of its page. It changes nothing. A
	// supplier with nothing to balance against, such as a single region,
	// returns nil.
	Remaining() []Region
	// BalanceEnd reports whether FlowText balances the end of the flow. The
	// part of the flow after its last forced break that ends on a page is
	// then set in the regions of that page as high as it takes to hold it:
	// each region ends on one line across the page, or at its own foot
	// above that line. Regions before are filled as without it.
	BalanceEnd() bool
}

// flowBalance balances the end of a caller's flow. When the last group of
// the flow is built, it keeps a pristine copy of the group and the state of
// the flow at that moment. At the start of each page after it, a trial runs
// a fresh copy of the group from its start through the regions handed out
// since, then through the regions Remaining names, capped at a line; a
// bisection finds the highest line at which the group ends on the page, and
// the real run goes on with its regions capped there.
type flowBalance struct {
	rr RemainingRegions
	// vl is a copy of the last group as built, nil before it is built and
	// once the flow is balanced or the balance given up. height is its
	// height.
	vl     *node.VList
	height bag.ScaledPoint
	// fc, sink and page are the cursor, the boxes already in its region and
	// the page buffer when vl was built.
	fc   flowCursor
	sink []sinkEntry
	page pageState
	// handed are the regions Next handed out since vl was built, consumed
	// the height used in the regions filled since.
	handed   []region
	consumed bag.ScaledPoint
	// Once balanced, the regions expect names are capped at line, as long
	// as Next hands them out in turn.
	capped bool
	line   bag.ScaledPoint
	expect []Region
}

// take keeps the last group vl, just built for fc's region, for the trials.
func (b *flowBalance) take(cb *CSSBuilder, vl *node.VList, fc *flowCursor) {
	b.vl, b.height = trialCopy(vl), vl.Height+vl.Depth
	b.fc = *fc
	b.sink = slices.Clone(fc.cur.sink.entries)
	s := cb.pageState()
	b.page = pageState{buf: slices.Clone(s.buf), bufHeight: s.bufHeight, inserts: map[InsertClass][]*Insert{}, insertsHgt: map[InsertClass]bag.ScaledPoint{}}
	for k, v := range s.inserts {
		b.page.inserts[k] = slices.Clone(v)
	}
	for k, v := range s.insertsHgt {
		b.page.insertsHgt[k] = v
	}
	b.handed, b.consumed = nil, 0
}

// entered is called for every region rg that Next hands out, cur being the
// region the paginator fills for it. It returns the region capped when the flow is
// balanced; at the start of a page, it tries to balance the rest of the
// group over that page.
func (b *flowBalance) entered(cb *CSSBuilder, rg Region, cur region, pageStart bool) region {
	switch {
	case b.capped:
		if len(b.expect) == 0 || rg != b.expect[0] {
			// Next goes elsewhere than Remaining said: fill on.
			b.capped, b.expect = false, nil
			return cur
		}
		b.expect = b.expect[1:]
		cur.height = capHeight(cur, b.line)
	case b.vl != nil:
		if pageStart {
			if line, rem, ok := cb.balanceLine(b, cur, true); ok {
				return b.capAt(line, rem, cur)
			}
		}
		b.handed = append(b.handed, cur)
	}
	return cur
}

// capAt starts the cap at line, cur being the first region it applies to
// and rem the regions Remaining named after it.
func (b *flowBalance) capAt(line bag.ScaledPoint, rem []Region, cur region) region {
	b.capped, b.line, b.expect = true, line, rem
	b.vl, b.handed, b.page, b.sink = nil, nil, pageState{}, nil
	cur.height = capHeight(cur, line)
	return cur
}

// capHeight is r's height down to line, or to its foot above it.
func capHeight(r region, line bag.ScaledPoint) bag.ScaledPoint {
	// A region that starts below the line keeps a sliver, which takes
	// nothing a trial would have let fit.
	return max(min(r.height, r.top-line), 1)
}

// balanceLine returns the highest line at which the rest of the group ends
// in cur and the regions after it on its page, and those regions as
// Remaining named them; false when it does not end there or no trial can
// tell. entering is set for a region Next has just handed out, which the
// trial reaches after the regions handed out before; otherwise cur is the
// region the group was built for.
func (cb *CSSBuilder) balanceLine(b *flowBalance, cur region, entering bool) (bag.ScaledPoint, []Region, bool) {
	rem := b.rr.Remaining()
	if len(rem) == 0 {
		return 0, nil, false
	}
	page := []region{cur}
	for _, rg := range rem {
		page = append(page, callerRegion(rg))
	}
	var room bag.ScaledPoint
	top, bottom := cur.top, cur.bottom()
	for _, r := range page {
		room += r.height
		top, bottom = max(top, r.top), min(bottom, r.bottom())
	}
	// The group's height less what the regions since its build hold: more
	// than the page holds cannot end on it.
	if b.height-b.consumed > room {
		return 0, nil, false
	}
	fits := func(h bag.ScaledPoint) bool { return cb.flowTrial(b, entering, page, top-h) }
	if !fits(top - bottom) {
		return 0, nil, false
	}
	return top - bisectTrial(top-bottom, fits), rem, true
}

// errFlowTrialOverflow stops a trial whose content goes on past the page it
// balances.
var errFlowTrialOverflow = errors.New("htmlbag: the content does not end on the balanced page")

// flowTrialRegions hands a trial the regions of the real run again, then
// those of the page being balanced, capped at a line.
type flowTrialRegions struct {
	cb   *CSSBuilder
	regs []region
	// at is the index of the current region in regs, capFrom that of the
	// first capped one.
	at, capFrom int
	tooTall     bool
}

func (t *flowTrialRegions) next(brk string) (region, error) {
	k := t.at + 1
	// A forced break inside the balanced page would leave it in the real
	// run.
	if k >= len(t.regs) || brk != "" && k > t.capFrom {
		return region{}, errFlowTrialOverflow
	}
	t.at = k
	return t.regs[k], nil
}

// filled notes whether a capped region holds more than its height, then
// drops what it holds.
func (t *flowTrialRegions) filled(filled) error {
	cur := t.regs[t.at]
	if t.at >= t.capFrom && t.cb.trialUsed(cur.sink) > cur.height {
		t.tooTall = true
	}
	return t.cb.flushInsertsIn(cur)
}

// trialUsed is how far the content of the region the sink belongs to
// reaches, margins below it left out, as regionSink.filled reports it.
func (cb *CSSBuilder) trialUsed(s *regionSink) bag.ScaledPoint {
	var used bag.ScaledPoint
	for _, e := range s.entries {
		if !e.margin {
			used = max(used, e.off+max(e.height, e.floats))
		}
	}
	var y bag.ScaledPoint
	for _, e := range cb.pageBuf {
		if _, m := marginKern(e.box.List); !m || e.box.List.Next() != nil {
			used = max(used, y+max(e.height, floatsBottom(e.box)))
		}
		y += e.height
	}
	return used
}

// trialWarned marks every page-level insert as warned about, so that a
// trial logs nothing and leaves the warnings to the real run.
var trialWarned = map[InsertClass]bool{InsertFootnote: true, InsertFloatTop: true, InsertFloatBottom: true}

// flowTrial runs a fresh copy of the group b holds from the state of its
// build, through the regions handed out since when entering, then through
// page capped at line. It reports whether the group ends there without a
// capped region holding more than its height. Neither the real cursor nor the builder's page change:
// the trial runs on a copy of the cursor of the build, and its regions
// paint nothing.
func (cb *CSSBuilder) flowTrial(b *flowBalance, entering bool, page []region, line bag.ScaledPoint) bool {
	// Without entering, page starts with the region the group was built
	// for.
	var regs []region
	if entering {
		regs = append([]region{b.fc.cur}, b.handed...)
	}
	capFrom := len(regs)
	for _, r := range page {
		r.height = capHeight(r, line)
		regs = append(regs, r)
	}
	for i := range regs {
		regs[i].sink, regs[i].trial, regs[i].collects = &regionSink{}, true, true
	}
	regs[0].sink.entries = slices.Clone(b.sink)
	t := &flowTrialRegions{cb: cb, regs: regs, capFrom: capFrom}
	tfc := b.fc
	tfc.regions, tfc.cur, tfc.warned = t, regs[0], trialWarned

	fits := false
	cb.onTrialPage(b.page, func() {
		// As in floatFitWidth, a build inside the trial (a rebuild at
		// another width, a repeated table header) registers nothing.
		flow, rebuild, tagging, callback := cb.callerFlow, cb.reflowRebuild, cb.enableTagging, cb.ElementCallback
		cb.callerFlow, cb.reflowRebuild, cb.enableTagging, cb.ElementCallback = &tfc, true, false, nil
		defer func() {
			cb.callerFlow, cb.reflowRebuild, cb.enableTagging, cb.ElementCallback = flow, rebuild, tagging, callback
		}()
		restart, _, err := cb.outputGroupNodes(trialCopy(b.vl), &tfc)
		// A restart is a region of another width: the trial gives up.
		if err != nil || restart >= 0 || t.at < capFrom {
			return
		}
		if err := t.filled(filled{}); err != nil {
			return
		}
		fits = !t.tooTall
	})
	return fits
}

// balanceGroup keeps vl, the last group just built or rebuilt for fc's
// region, for the trials, and balances it from that region on when it ends
// on its page.
func (cb *CSSBuilder) balanceGroup(b *flowBalance, vl *node.VList, fc *flowCursor) {
	if b.capped {
		return
	}
	b.take(cb, vl, fc)
	line, rem, ok := cb.balanceLine(b, fc.cur, false)
	if !ok {
		return
	}
	cr := fc.regions.(*callerRegions)
	cr.cur = b.capAt(line, rem, cr.cur)
	fc.cur.height = cr.cur.height
}
